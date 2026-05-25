#!/usr/bin/env python3

from __future__ import annotations

import csv
import shutil
from pathlib import Path


SOURCE_DIR = Path("dataset")
CSV_PATH = Path("vimeo-bitrates.full.csv")
OUTPUT_DIR = Path("dataset/pre-selecao-top4-bitrate")
TOP_N = 2

GROUP_ALIASES = {
    "comercial-cortes": "comercial-cortes",
    "comercial-falas": "comercial-educativo-produtos",
    "comercial-estatico - poucos cortes": "comercial-estatico - poucos cortes",
    "comercial-educativo - com fala - institucional": "institucional-eventos-treinamento",
}


def build_source_index() -> dict[str, tuple[Path, str]]:
    index: dict[str, tuple[Path, str]] = {}
    for path in SOURCE_DIR.rglob("*"):
        if not path.is_file():
            continue
        if path.suffix.lower() not in {".mp4", ".mov", ".mkv", ".webm"}:
            continue
        if path.parent == OUTPUT_DIR or OUTPUT_DIR in path.parents:
            continue
        video_id = path.name.split(" ", 1)[0]
        index[video_id] = (path, GROUP_ALIASES.get(path.parent.name, path.parent.name))
    return index


def best_rows_by_video() -> dict[str, dict[str, str]]:
    best: dict[str, tuple[tuple[int, int], dict[str, str]]] = {}
    with CSV_PATH.open(newline="", encoding="utf-8-sig") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None:
            return {}
        reader.fieldnames = [name.strip() for name in reader.fieldnames]
        for raw_row in reader:
            row = {
                key.strip(): (value.strip() if isinstance(value, str) else value)
                for key, value in raw_row.items()
                if key is not None
            }
            try:
                height = int(row["height"])
                bitrate = int(row["bitrate_kbps"])
            except (TypeError, ValueError, KeyError):
                continue
            video_id = row["video_id"].strip()
            score = (bitrate, height)
            current = best.get(video_id)
            if current is None or score > current[0]:
                best[video_id] = (score, row)
    return {video_id: item[1] for video_id, item in best.items()}


def main() -> int:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    source_index = build_source_index()
    best_rows = best_rows_by_video()

    grouped: dict[str, list[tuple[int, str, dict[str, str], Path]]] = {}
    for video_id, row in best_rows.items():
        source = source_index.get(video_id)
        if source is None:
            continue
        path, target_group = source
        bitrate = int(row["bitrate_kbps"])
        grouped.setdefault(target_group, []).append((bitrate, video_id, row, path))

    copied = []
    for group, items in grouped.items():
        group_dir = OUTPUT_DIR / group
        group_dir.mkdir(parents=True, exist_ok=True)
        for bitrate, video_id, row, path in sorted(items, key=lambda item: (-item[0], item[1]))[:TOP_N]:
            destination = group_dir / path.name
            shutil.copy2(path, destination)
            copied.append((group, video_id, bitrate, destination))

    report_path = OUTPUT_DIR / "pre-selecao-resumo.csv"
    with report_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["group", "video_id", "bitrate_kbps", "output_path"])
        for group, video_id, bitrate, destination in copied:
            writer.writerow([group, video_id, bitrate, str(destination)])

    print(f"copied={len(copied)}")
    print(f"report={report_path}")
    for group, video_id, bitrate, destination in copied:
        print(f"{group} | {video_id} | {bitrate} | {destination}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
