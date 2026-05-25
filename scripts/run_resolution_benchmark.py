#!/usr/bin/env python3

import argparse
import csv
import hashlib
import json
import os
import re
import subprocess
from datetime import datetime
from pathlib import Path


OBSERVABILITY_RE = re.compile(
    r"observability supported=(?P<supported>\w+) "
    r"samples=(?P<samples>\d+) "
    r"elapsed=(?P<elapsed>[0-9.]+)s "
    r"rtf=(?P<rtf>[0-9.]+) "
    r"avgCpu=(?P<avg_cpu>[0-9.]+)% "
    r"maxCpu=(?P<max_cpu>[0-9.]+)% "
    r"outputSize=(?P<output_size>\d+) "
    r"outputBitrate=(?P<output_bitrate>\d+)kbps "
    r'error="(?P<error>.*)"'
)

RESOLUTIONS = [
    ("4k", 2160, 3840, 12000),
    ("2k", 1440, 2560, 8000),
    ("1080p", 1080, 1920, 6000),
    ("720p", 720, 1280, 3000),
]


def run(cmd, cwd, env=None):
    completed = subprocess.run(
        cmd,
        cwd=cwd,
        env=env,
        text=True,
        capture_output=True,
        check=True,
    )
    return completed.stdout.strip(), completed.stderr.strip()


def run_maybe(cmd, cwd, env=None):
    completed = subprocess.run(
        cmd,
        cwd=cwd,
        env=env,
        text=True,
        capture_output=True,
    )
    return completed.returncode, completed.stdout.strip(), completed.stderr.strip()


def ffprobe_json(path, cwd):
    stdout, _ = run(
        [
            "ffprobe",
            "-v",
            "error",
            "-show_entries",
            "stream=codec_name,width,height,avg_frame_rate,pix_fmt",
            "-show_entries",
            "format=duration,size,bit_rate",
            "-of",
            "json",
            str(path),
        ],
        cwd=cwd,
    )
    return json.loads(stdout)


def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def parse_observability(output):
    match = OBSERVABILITY_RE.search(output)
    if not match:
        raise RuntimeError("linha de observabilidade nao encontrada no output")
    return match.groupdict()


def build_linux_binary(repo_root):
    dist = repo_root / "dist"
    dist.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env.update(
        {
            "GOOS": "linux",
            "GOARCH": "amd64",
            "CGO_ENABLED": "0",
            "GOCACHE": "/private/tmp/go-build",
        }
    )
    run(
        [
            "go",
            "build",
            "-o",
            str(dist / "transcode-local-linux-amd64"),
            "./cmd/transcode-local",
        ],
        cwd=repo_root,
        env=env,
    )


def build_host_binary(repo_root):
    dist = repo_root / "dist"
    dist.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env.update({"GOCACHE": "/private/tmp/go-build"})
    run(
        [
            "go",
            "build",
            "-o",
            str(dist / "transcode-local-host"),
            "./cmd/transcode-local",
        ],
        cwd=repo_root,
        env=env,
    )


def build_image(repo_root):
    run(["docker", "compose", "-f", "compose.yaml", "build", "transcode-local"], cwd=repo_root)


def docker_available(repo_root):
    code, _, stderr = run_maybe(["docker", "info"], cwd=repo_root)
    return code == 0, stderr


def benchmark_one_docker(repo_root, input_path, codec, label, width, height, bitrate_kbps, output_dir):
    output_dir.mkdir(parents=True, exist_ok=True)
    output_path = output_dir / f"{input_path.parent.name}-{codec}-{label}.mp4"
    if output_path.exists():
        output_path.unlink()

    env = os.environ.copy()
    env.update(
        {
            "INPUT": f"/workspace/{input_path.relative_to(repo_root)}",
            "OUTPUT": f"/workspace/{output_path.relative_to(repo_root)}",
            "WIDTH": str(width),
            "HEIGHT": str(height),
            "BITRATE_KBPS": str(bitrate_kbps),
            "TRANSCODE_CODEC": codec,
        }
    )

    stdout, stderr = run(
        ["docker", "compose", "-f", "compose.yaml", "run", "--rm", "transcode-local"],
        cwd=repo_root,
        env=env,
    )
    return output_path, "\n".join(part for part in [stdout, stderr] if part)


def benchmark_one_local(repo_root, input_path, codec, label, width, height, bitrate_kbps, output_dir):
    output_dir.mkdir(parents=True, exist_ok=True)
    output_path = output_dir / f"{input_path.parent.name}-{codec}-{label}.mp4"
    if output_path.exists():
        output_path.unlink()

    stdout, stderr = run(
        [
            str(repo_root / "dist" / "transcode-local-host"),
            "--input",
            str(input_path),
            "--output",
            str(output_path),
            "--codec",
            codec,
            "--width",
            str(width),
            "--height",
            str(height),
            "--bitrate-kbps",
            str(bitrate_kbps),
        ],
        cwd=repo_root,
        env=os.environ.copy(),
    )
    return output_path, "\n".join(part for part in [stdout, stderr] if part)


def benchmark_one(repo_root, runtime_name, input_path, codec, label, width, height, bitrate_kbps, output_dir):
    if runtime_name == "docker":
        output_path, combined = benchmark_one_docker(
            repo_root, input_path, codec, label, width, height, bitrate_kbps, output_dir
        )
    else:
        output_path, combined = benchmark_one_local(
            repo_root, input_path, codec, label, width, height, bitrate_kbps, output_dir
        )

    observability = parse_observability(combined)
    probe = ffprobe_json(output_path, repo_root)
    sha256 = sha256_file(output_path)
    stream = probe["streams"][0]
    fmt = probe["format"]

    return {
        "resolution_label": label,
        "codec": codec,
        "runtime": runtime_name,
        "target_width": width,
        "target_height": height,
        "target_bitrate_kbps": bitrate_kbps,
        "supported": observability["supported"],
        "samples": int(observability["samples"]),
        "elapsed_seconds": float(observability["elapsed"]),
        "rtf": float(observability["rtf"]),
        "avg_cpu_percent": float(observability["avg_cpu"]),
        "max_cpu_percent": float(observability["max_cpu"]),
        "reported_output_size_bytes": int(observability["output_size"]),
        "reported_output_bitrate_kbps": int(observability["output_bitrate"]),
        "output_codec": stream["codec_name"],
        "output_width": int(stream["width"]),
        "output_height": int(stream["height"]),
        "output_pix_fmt": stream["pix_fmt"],
        "output_avg_frame_rate": stream["avg_frame_rate"],
        "output_duration_seconds": float(fmt["duration"]),
        "output_size_bytes": int(fmt["size"]),
        "output_bitrate_bps": int(fmt["bit_rate"]),
        "sha256": sha256,
        "output_path": str(output_path.relative_to(repo_root)),
        "observability_error": observability["error"],
        "docker_output_log": combined,
    }


def write_reports(rows, csv_path, md_path, input_path, codec):
    fieldnames = [
        "resolution_label",
        "codec",
        "runtime",
        "target_width",
        "target_height",
        "target_bitrate_kbps",
        "supported",
        "samples",
        "elapsed_seconds",
        "rtf",
        "avg_cpu_percent",
        "max_cpu_percent",
        "reported_output_size_bytes",
        "reported_output_bitrate_kbps",
        "output_codec",
        "output_width",
        "output_height",
        "output_pix_fmt",
        "output_avg_frame_rate",
        "output_duration_seconds",
        "output_size_bytes",
        "output_bitrate_bps",
        "sha256",
        "output_path",
        "observability_error",
    ]
    with open(csv_path, "w", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow({key: row[key] for key in fieldnames})

    with open(md_path, "w") as handle:
        handle.write("# Benchmark por Resolucao\n\n")
        handle.write(f"Entrada: `{input_path}`\n\n")
        handle.write(f"Codec: `{codec}`\n\n")
        handle.write(f"Planilha: `{csv_path.name}`\n\n")
        handle.write("| Resolucao | Elapsed (s) | RTF | Avg CPU % | Max CPU % | Bitrate final kbps | Tamanho final |\n")
        handle.write("| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
        for row in rows:
            handle.write(
                f"| {row['resolution_label']} | {row['elapsed_seconds']:.3f} | {row['rtf']:.3f} | "
                f"{row['avg_cpu_percent']:.2f} | {row['max_cpu_percent']:.2f} | "
                f"{row['reported_output_bitrate_kbps']} | {row['output_size_bytes']} |\n"
            )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--codec", default="av1")
    parser.add_argument("--report-dir", default="relatorios")
    parser.add_argument("--output-dir", default="outputs/docker-validation")
    parser.add_argument("--runtime", choices=["auto", "docker", "local"], default="auto")
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parent.parent
    input_path = (repo_root / args.input).resolve()
    report_dir = repo_root / args.report_dir
    output_dir = repo_root / args.output_dir
    report_dir.mkdir(parents=True, exist_ok=True)

    runtime_name = args.runtime
    if runtime_name == "auto":
        available, _ = docker_available(repo_root)
        runtime_name = "docker" if available else "local"

    if runtime_name == "docker":
        build_linux_binary(repo_root)
        build_image(repo_root)
    else:
        build_host_binary(repo_root)

    rows = []
    for label, width, height, bitrate_kbps in RESOLUTIONS:
        rows.append(
            benchmark_one(
                repo_root=repo_root,
                runtime_name=runtime_name,
                input_path=input_path,
                codec=args.codec,
                label=label,
                width=width,
                height=height,
                bitrate_kbps=bitrate_kbps,
                output_dir=output_dir,
            )
        )

    timestamp = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
    stem = f"benchmark-{input_path.parent.name}-{args.codec}-{timestamp}"
    csv_path = report_dir / f"{stem}.csv"
    md_path = report_dir / f"{stem}.md"
    write_reports(rows, csv_path, md_path, args.input, args.codec)

    print(csv_path)
    print(md_path)


if __name__ == "__main__":
    main()
