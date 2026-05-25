#!/usr/bin/env python3

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass, asdict
from datetime import datetime, timezone
from fractions import Fraction
from pathlib import Path


VIDEO_EXTS = {".mp4", ".mov", ".mkv", ".webm"}

GROUP_ALIASES = {
    "comercial-cortes": "comercial-cortes",
    "comercial-falas": "comercial-educativo-produtos",
    "comercial-estatico - poucos cortes": "comercial-estatico - poucos cortes",
    "comercial-educativo - com fala - institucional": "institucional-eventos-treinamento",
}


@dataclass
class VideoSpec:
    source_path: str
    source_group: str
    target_group: str
    relative_name: str
    video_id: str
    title_slug: str
    width: int
    height: int
    fps: float
    duration_seconds: float
    source_pix_fmt: str
    target_pix_fmt: str
    estimated_y4m_bytes: int


@dataclass
class VideoResult:
    source_path: str
    target_group: str
    video_id: str
    status: str
    reason: str
    output_dir: str
    master_path: str | None = None
    y4m_path: str | None = None
    checksum_path: str | None = None
    log_path: str | None = None
    duration_seconds: float | None = None
    estimated_y4m_bytes: int | None = None


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Gera masters FFV1 e Y4M para benchmark de codecs."
    )
    parser.add_argument("--input-dir", default="dataset", help="pasta com os videos")
    parser.add_argument(
        "--output-dir",
        default="outputs/reference-pipeline",
        help="pasta raiz dos artefatos gerados",
    )
    parser.add_argument(
        "--report-dir",
        default="relatorios",
        help="pasta onde os relatorios serao gravados",
    )
    parser.add_argument(
        "--overwrite",
        action="store_true",
        help="regera arquivos mesmo se eles ja existirem",
    )
    parser.add_argument(
        "--sha256",
        action="store_true",
        help="gera checksums SHA256 para master e y4m",
    )
    parser.add_argument(
        "--skip-master",
        action="store_true",
        help="nao gera o master FFV1; gera apenas o arquivo Y4M final",
    )
    parser.add_argument(
        "--ignore-space-check",
        action="store_true",
        help="executa mesmo se a estimativa de espaco exceder o espaco livre",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=0,
        help="processa apenas os primeiros N videos encontrados",
    )
    return parser.parse_args()


def slugify(value: str) -> str:
    value = value.strip()
    value = re.sub(r"\s+", "-", value)
    value = re.sub(r"[^A-Za-z0-9._-]", "-", value)
    value = re.sub(r"-{2,}", "-", value)
    return value.strip("-") or "video"


def run_command(command: list[str], log_file: Path) -> None:
    with log_file.open("a", encoding="utf-8") as log:
        log.write(f"$ {' '.join(command)}\n")
        process = subprocess.run(
            command,
            stdout=log,
            stderr=subprocess.STDOUT,
            text=True,
            check=False,
        )
        log.write(f"[exit_code={process.returncode}]\n\n")
    if process.returncode != 0:
        raise RuntimeError(f"command failed with exit code {process.returncode}")


def ffprobe_json(path: Path) -> dict:
    command = [
        "ffprobe",
        "-v",
        "error",
        "-show_format",
        "-show_streams",
        "-of",
        "json",
        str(path),
    ]
    return json.loads(subprocess.check_output(command, text=True))


def detect_target_pix_fmt(source_pix_fmt: str) -> str:
    lower = source_pix_fmt.lower()
    if "10" in lower or "12" in lower or "16" in lower:
        return "yuv420p10le"
    return "yuv420p"


def estimate_y4m_bytes(width: int, height: int, fps: float, duration: float, pix_fmt: str) -> int:
    bytes_per_pixel = 3.0 if pix_fmt == "yuv420p10le" else 1.5
    return int(width * height * bytes_per_pixel * fps * duration)


def discover_videos(input_dir: Path) -> list[VideoSpec]:
    specs: list[VideoSpec] = []
    for path in sorted(input_dir.rglob("*")):
        if path.suffix.lower() not in VIDEO_EXTS or not path.is_file():
            continue
        meta = ffprobe_json(path)
        video_stream = next((s for s in meta.get("streams", []) if s.get("codec_type") == "video"), None)
        if not video_stream:
            continue
        width = int(video_stream["width"])
        height = int(video_stream["height"])
        fps = float(Fraction(video_stream.get("r_frame_rate", "0/1")))
        duration = float(meta["format"]["duration"])
        source_pix_fmt = video_stream.get("pix_fmt", "yuv420p")
        target_pix_fmt = detect_target_pix_fmt(source_pix_fmt)
        source_group = path.parent.name
        target_group = GROUP_ALIASES.get(source_group, source_group)
        video_id = path.name.split(" ", 1)[0]
        title_part = path.stem.split(" - ", 1)[1] if " - " in path.stem else path.stem
        title_slug = slugify(title_part)
        specs.append(
            VideoSpec(
                source_path=str(path),
                source_group=source_group,
                target_group=target_group,
                relative_name=path.name,
                video_id=video_id,
                title_slug=title_slug,
                width=width,
                height=height,
                fps=fps,
                duration_seconds=duration,
                source_pix_fmt=source_pix_fmt,
                target_pix_fmt=target_pix_fmt,
                estimated_y4m_bytes=estimate_y4m_bytes(width, height, fps, duration, target_pix_fmt),
            )
        )
    return specs


def validate_master(master_path: Path, spec: VideoSpec) -> None:
    data = ffprobe_json(master_path)
    video_stream = next((s for s in data.get("streams", []) if s.get("codec_type") == "video"), None)
    if not video_stream:
        raise RuntimeError("master sem stream de video")
    if video_stream.get("codec_name") != "ffv1":
        raise RuntimeError(f"codec master invalido: {video_stream.get('codec_name')}")
    if int(video_stream.get("width", 0)) != spec.width or int(video_stream.get("height", 0)) != spec.height:
        raise RuntimeError("resolucao master divergente")
    if video_stream.get("pix_fmt") != spec.target_pix_fmt:
        raise RuntimeError(f"pix_fmt master divergente: {video_stream.get('pix_fmt')}")


def validate_y4m(y4m_path: Path, spec: VideoSpec) -> None:
    data = ffprobe_json(y4m_path)
    video_stream = next((s for s in data.get("streams", []) if s.get("codec_type") == "video"), None)
    if not video_stream:
        raise RuntimeError("y4m sem stream de video")
    if int(video_stream.get("width", 0)) != spec.width or int(video_stream.get("height", 0)) != spec.height:
        raise RuntimeError("resolucao y4m divergente")
    if video_stream.get("pix_fmt") != spec.target_pix_fmt:
        raise RuntimeError(f"pix_fmt y4m divergente: {video_stream.get('pix_fmt')}")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        while True:
            chunk = handle.read(1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
    return digest.hexdigest()


def write_checksums(checksum_path: Path, master_path: Path, y4m_path: Path) -> None:
    master_sum = sha256_file(master_path)
    y4m_sum = sha256_file(y4m_path)
    checksum_path.write_text(
        f"{master_sum}  {master_path.name}\n{y4m_sum}  {y4m_path.name}\n",
        encoding="utf-8",
    )


def write_y4m_checksum(checksum_path: Path, y4m_path: Path) -> None:
    y4m_sum = sha256_file(y4m_path)
    checksum_path.write_text(
        f"{y4m_sum}  {y4m_path.name}\n",
        encoding="utf-8",
    )


def is_complete(master_path: Path, y4m_path: Path, spec: VideoSpec) -> bool:
    if not master_path.exists() or not y4m_path.exists():
        return False
    try:
        validate_master(master_path, spec)
        validate_y4m(y4m_path, spec)
    except Exception:
        return False
    return True


def is_y4m_complete(y4m_path: Path, spec: VideoSpec) -> bool:
    if not y4m_path.exists():
        return False
    try:
        validate_y4m(y4m_path, spec)
    except Exception:
        return False
    return True


def process_video(
    spec: VideoSpec,
    output_root: Path,
    overwrite: bool,
    with_sha256: bool,
    skip_master: bool,
) -> VideoResult:
    output_dir = output_root / spec.target_group / f"{spec.video_id}-{spec.title_slug}"
    output_dir.mkdir(parents=True, exist_ok=True)
    master_path = output_dir / "master_ffv1.mkv"
    y4m_path = output_dir / "reference.y4m"
    checksum_path = output_dir / "sha256.txt"
    log_path = output_dir / "conversion.log"
    metadata_path = output_dir / "metadata.json"

    metadata_path.write_text(json.dumps(asdict(spec), indent=2, ensure_ascii=False), encoding="utf-8")

    if not skip_master and not overwrite and is_complete(master_path, y4m_path, spec):
        if with_sha256 and not checksum_path.exists():
            write_checksums(checksum_path, master_path, y4m_path)
        return VideoResult(
            source_path=spec.source_path,
            target_group=spec.target_group,
            video_id=spec.video_id,
            status="skipped",
            reason="artefatos ja existem e foram validados",
            output_dir=str(output_dir),
            master_path=str(master_path),
            y4m_path=str(y4m_path),
            checksum_path=str(checksum_path) if checksum_path.exists() else None,
            log_path=str(log_path),
            duration_seconds=spec.duration_seconds,
            estimated_y4m_bytes=spec.estimated_y4m_bytes,
        )
    if skip_master and not overwrite and is_y4m_complete(y4m_path, spec):
        if with_sha256 and not checksum_path.exists():
            write_y4m_checksum(checksum_path, y4m_path)
        return VideoResult(
            source_path=spec.source_path,
            target_group=spec.target_group,
            video_id=spec.video_id,
            status="skipped",
            reason="y4m ja existe e foi validado",
            output_dir=str(output_dir),
            y4m_path=str(y4m_path),
            checksum_path=str(checksum_path) if checksum_path.exists() else None,
            log_path=str(log_path),
            duration_seconds=spec.duration_seconds,
            estimated_y4m_bytes=spec.estimated_y4m_bytes,
        )

    if overwrite:
        for path in (master_path, y4m_path, checksum_path):
            if path.exists():
                path.unlink()

    log_path.write_text(
        f"source={spec.source_path}\nstarted_at={datetime.now(timezone.utc).isoformat()}\n\n",
        encoding="utf-8",
    )

    if skip_master:
        y4m_cmd = [
            "ffmpeg",
            "-hide_banner",
            "-y",
            "-i",
            spec.source_path,
            "-map",
            "0:v:0",
            "-an",
            "-pix_fmt",
            spec.target_pix_fmt,
            "-f",
            "yuv4mpegpipe",
            str(y4m_path),
        ]
        run_command(y4m_cmd, log_path)
    else:
        master_cmd = [
            "ffmpeg",
            "-hide_banner",
            "-y",
            "-i",
            spec.source_path,
            "-map",
            "0:v:0",
            "-an",
            "-c:v",
            "ffv1",
            "-level",
            "3",
            "-g",
            "1",
            "-pix_fmt",
            spec.target_pix_fmt,
            str(master_path),
        ]
        y4m_cmd = [
            "ffmpeg",
            "-hide_banner",
            "-y",
            "-i",
            str(master_path),
            "-map",
            "0:v:0",
            "-an",
            "-pix_fmt",
            spec.target_pix_fmt,
            "-f",
            "yuv4mpegpipe",
            str(y4m_path),
        ]
        run_command(master_cmd, log_path)
        validate_master(master_path, spec)
        run_command(y4m_cmd, log_path)
    validate_y4m(y4m_path, spec)

    if with_sha256:
        if skip_master:
            write_y4m_checksum(checksum_path, y4m_path)
        else:
            write_checksums(checksum_path, master_path, y4m_path)

    return VideoResult(
        source_path=spec.source_path,
        target_group=spec.target_group,
        video_id=spec.video_id,
        status="converted",
        reason="ok",
        output_dir=str(output_dir),
        master_path=str(master_path) if not skip_master else None,
        y4m_path=str(y4m_path),
        checksum_path=str(checksum_path) if with_sha256 else None,
        log_path=str(log_path),
        duration_seconds=spec.duration_seconds,
        estimated_y4m_bytes=spec.estimated_y4m_bytes,
    )


def write_report(report_dir: Path, payload: dict) -> tuple[Path, Path]:
    report_dir.mkdir(parents=True, exist_ok=True)
    stamp = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
    json_path = report_dir / f"pipeline-conversao-lote-{stamp}.json"
    md_path = report_dir / f"pipeline-conversao-lote-{stamp}.md"
    json_path.write_text(json.dumps(payload, indent=2, ensure_ascii=False), encoding="utf-8")

    lines = [
        "# Relatorio de Conversao em Lote",
        "",
        f"Data: {payload['generated_at']}",
        "",
        "## Resumo",
        "",
        f"- Videos encontrados: `{payload['summary']['found']}`",
        f"- Convertidos: `{payload['summary']['converted']}`",
        f"- Pulados: `{payload['summary']['skipped']}`",
        f"- Falharam: `{payload['summary']['failed']}`",
        f"- Bloqueados no preflight: `{payload['summary'].get('blocked_preflight', 0)}`",
        f"- Espaco livre no inicio: `{payload['preflight']['free_gib']:.2f} GiB`",
        f"- Estimativa total Y4M: `{payload['preflight']['estimated_y4m_gib']:.2f} GiB`",
        "",
        "## Configuracao",
        "",
        f"- Input: `{payload['config']['input_dir']}`",
        f"- Output: `{payload['config']['output_dir']}`",
        f"- Overwrite: `{payload['config']['overwrite']}`",
        f"- SHA256: `{payload['config']['sha256']}`",
        f"- Skip master: `{payload['config']['skip_master']}`",
        "",
        "## Preflight",
        "",
        f"- Status: `{payload['preflight']['status']}`",
        f"- Motivo: `{payload['preflight']['reason']}`",
        "",
        "## Convertidos",
        "",
    ]

    converted = [r for r in payload["results"] if r["status"] == "converted"]
    if converted:
        for item in converted:
            lines.append(
                f"- `{item['video_id']}` | `{item['target_group']}` | `{item['master_path']}` | `{item['y4m_path']}`"
            )
    else:
        lines.append("- Nenhum video convertido.")

    lines.extend(["", "## Nao Convertidos", ""])
    not_converted = [r for r in payload["results"] if r["status"] != "converted"]
    if not_converted:
        for item in not_converted:
            lines.append(
                f"- `{item['video_id']}` | status=`{item['status']}` | motivo=`{item['reason']}` | origem=`{item['source_path']}`"
            )
    else:
        lines.append("- Nenhum.")

    md_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return json_path, md_path


def main() -> int:
    args = parse_args()
    input_dir = Path(args.input_dir)
    output_dir = Path(args.output_dir)
    report_dir = Path(args.report_dir)

    specs = discover_videos(input_dir)
    if args.limit > 0:
        specs = specs[: args.limit]

    disk_usage = shutil.disk_usage(output_dir.parent if output_dir.parent.exists() else Path("."))
    free_gib = disk_usage.free / 1024 / 1024 / 1024
    estimated_y4m_gib = sum(s.estimated_y4m_bytes for s in specs) / 1024 / 1024 / 1024

    preflight_ok = args.ignore_space_check or disk_usage.free >= sum(s.estimated_y4m_bytes for s in specs)
    preflight_reason = (
        "espaco suficiente ou check ignorado"
        if preflight_ok
        else "espaco insuficiente para manter todos os Y4M estimados no disco"
    )

    results: list[VideoResult] = []
    if preflight_ok:
        for spec in specs:
            try:
                results.append(process_video(spec, output_dir, args.overwrite, args.sha256, args.skip_master))
            except Exception as exc:
                results.append(
                    VideoResult(
                        source_path=spec.source_path,
                        target_group=spec.target_group,
                        video_id=spec.video_id,
                        status="failed",
                        reason=str(exc),
                        output_dir=str(output_dir / spec.target_group / f"{spec.video_id}-{spec.title_slug}"),
                        duration_seconds=spec.duration_seconds,
                        estimated_y4m_bytes=spec.estimated_y4m_bytes,
                    )
                )
    else:
        for spec in specs:
            results.append(
                VideoResult(
                    source_path=spec.source_path,
                    target_group=spec.target_group,
                    video_id=spec.video_id,
                    status="blocked-preflight",
                    reason=preflight_reason,
                    output_dir=str(output_dir / spec.target_group / f"{spec.video_id}-{spec.title_slug}"),
                    duration_seconds=spec.duration_seconds,
                    estimated_y4m_bytes=spec.estimated_y4m_bytes,
                )
            )

    payload = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "config": {
            "input_dir": str(input_dir),
            "output_dir": str(output_dir),
            "report_dir": str(report_dir),
            "overwrite": args.overwrite,
            "sha256": args.sha256,
            "skip_master": args.skip_master,
            "ignore_space_check": args.ignore_space_check,
            "limit": args.limit,
        },
        "preflight": {
            "status": "ok" if preflight_ok else "blocked",
            "reason": preflight_reason,
            "free_gib": free_gib,
            "estimated_y4m_gib": estimated_y4m_gib,
        },
        "summary": {
            "found": len(specs),
            "converted": sum(1 for r in results if r.status == "converted"),
            "skipped": sum(1 for r in results if r.status == "skipped"),
            "failed": sum(1 for r in results if r.status == "failed"),
            "blocked_preflight": sum(1 for r in results if r.status == "blocked-preflight"),
        },
        "results": [asdict(r) for r in results],
    }

    json_path, md_path = write_report(report_dir, payload)
    print(f"report_json={json_path}")
    print(f"report_md={md_path}")
    print(json.dumps(payload["summary"], ensure_ascii=False))
    return 0 if preflight_ok and payload["summary"]["failed"] == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
