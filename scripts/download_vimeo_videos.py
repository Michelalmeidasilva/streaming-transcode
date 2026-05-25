#!/usr/bin/env python3

import argparse
import csv
import subprocess
import sys
import time
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Baixa os videos listados em um CSV gerado pelo vimeo-analyzer."
    )
    parser.add_argument(
        "--csv",
        default="vimeo-bitrates.csv",
        help="caminho do CSV de entrada",
    )
    parser.add_argument(
        "--output-dir",
        default="downloads/vimeo",
        help="diretorio onde os videos serao salvos",
    )
    parser.add_argument(
        "--yt-dlp-path",
        default="yt-dlp",
        help="caminho do executavel yt-dlp",
    )
    parser.add_argument(
        "--cookies-from-browser",
        default="",
        help="browser para ler cookies, ex: chrome, firefox, safari",
    )
    parser.add_argument(
        "--download-archive",
        default="downloads/vimeo/.downloaded.txt",
        help="arquivo de controle do yt-dlp para pular downloads ja feitos",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=0,
        help="limita quantos videos baixar; 0 baixa todos",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="mostra os comandos sem baixar",
    )
    parser.add_argument(
        "--sleep-seconds",
        type=float,
        default=8.0,
        help="espera entre downloads para reduzir rate limit",
    )
    parser.add_argument(
        "--retries",
        type=int,
        default=4,
        help="quantidade de tentativas por video",
    )
    parser.add_argument(
        "--retry-backoff",
        type=float,
        default=20.0,
        help="segundos base para backoff exponencial apos falha",
    )
    parser.add_argument(
        "--start-at",
        type=int,
        default=1,
        help="indice 1-based para iniciar a lista, util para retomar",
    )
    return parser.parse_args()


def load_rows(csv_path: Path) -> list[dict[str, str]]:
    with csv_path.open(newline="", encoding="utf-8") as handle:
        return list(csv.DictReader(handle))


def unique_videos(rows: list[dict[str, str]]) -> list[dict[str, str]]:
    unique: dict[str, dict[str, str]] = {}
    for row in rows:
        video_id = (row.get("video_id") or "").strip()
        input_url = (row.get("input_url") or "").strip()
        if not video_id or not input_url:
            continue
        if video_id not in unique:
            unique[video_id] = row
    return list(unique.values())


def build_command(
    yt_dlp_path: str,
    row: dict[str, str],
    output_dir: Path,
    archive_path: Path,
    cookies_from_browser: str,
) -> list[str]:
    input_url = row["input_url"].strip()

    output_template = str(output_dir / "%(id)s - %(title)s.%(ext)s")
    command = [
        yt_dlp_path,
        "--no-progress",
        "--continue",
        "--no-overwrites",
        "--download-archive",
        str(archive_path),
        "-o",
        output_template,
        "-f",
        "bv*+ba/b",
        "--merge-output-format",
        "mp4",
    ]
    if cookies_from_browser:
        command.extend(["--cookies-from-browser", cookies_from_browser])
    command.append(input_url)
    return command


def main() -> int:
    args = parse_args()
    csv_path = Path(args.csv)
    output_dir = Path(args.output_dir)
    archive_path = Path(args.download_archive)

    if not csv_path.exists():
        print(f"CSV nao encontrado: {csv_path}", file=sys.stderr)
        return 1

    rows = unique_videos(load_rows(csv_path))
    if args.start_at > 1:
        rows = rows[args.start_at - 1 :]
    if args.limit > 0:
        rows = rows[: args.limit]

    output_dir.mkdir(parents=True, exist_ok=True)
    archive_path.parent.mkdir(parents=True, exist_ok=True)

    failures = 0
    for index, row in enumerate(rows, start=1):
        video_id = row["video_id"].strip()
        title = (row.get("title") or "").strip() or "<sem titulo>"
        command = build_command(
            args.yt_dlp_path,
            row,
            output_dir,
            archive_path,
            args.cookies_from_browser,
        )

        print(f"[{index}/{len(rows)}] {video_id} | {title}")
        print(" ".join(command))
        if args.dry_run:
            continue

        completed = None
        for attempt in range(1, args.retries + 1):
            completed = subprocess.run(command)
            if completed.returncode == 0:
                break
            if attempt == args.retries:
                failures += 1
                print(f"falha ao baixar {video_id}", file=sys.stderr)
                break

            wait_seconds = args.retry_backoff * attempt
            print(
                f"tentativa {attempt} falhou para {video_id}; aguardando {wait_seconds:.0f}s antes de tentar de novo",
                file=sys.stderr,
            )
            time.sleep(wait_seconds)

        if index < len(rows) and args.sleep_seconds > 0:
            time.sleep(args.sleep_seconds)

    if failures:
        print(f"downloads com falha: {failures}", file=sys.stderr)
        return 1

    print(f"downloads concluidos: {len(rows)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
