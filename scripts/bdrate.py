"""BD-rate (Bjontegaard Delta-rate) for codec R-D comparison."""
import numpy as np


def bd_rate(rate_ref, metric_ref, rate_test, metric_test):
    """Average % bitrate difference of `test` vs `ref` at equal quality.
    Negative = test needs less bitrate (more efficient). Inputs are lists of
    (bitrate_kbps, quality) points; >=4 points each recommended.
    """
    lr1 = np.log(np.asarray(rate_ref, dtype=float))
    lr2 = np.log(np.asarray(rate_test, dtype=float))
    m1 = np.asarray(metric_ref, dtype=float)
    m2 = np.asarray(metric_test, dtype=float)

    # Fit log(rate) as a cubic polynomial of quality, integrate over the
    # overlapping quality interval, difference, convert to percent.
    p1 = np.polyfit(m1, lr1, 3)
    p2 = np.polyfit(m2, lr2, 3)
    lo = max(min(m1), min(m2))
    hi = min(max(m1), max(m2))
    if hi <= lo:
        raise ValueError("no overlapping quality range")
    P1 = np.polyint(p1)
    P2 = np.polyint(p2)
    int1 = np.polyval(P1, hi) - np.polyval(P1, lo)
    int2 = np.polyval(P2, hi) - np.polyval(P2, lo)
    avg_diff = (int2 - int1) / (hi - lo)
    return (np.exp(avg_diff) - 1.0) * 100.0


if __name__ == "__main__":
    import argparse, json, os, urllib.request
    from collections import defaultdict
    ap = argparse.ArgumentParser()
    ap.add_argument("--ingest-url", default="https://kg8jhai79k.execute-api.us-east-2.amazonaws.com/api/v1")
    ap.add_argument("--out", default="scripts/out")
    a = ap.parse_args()
    os.makedirs(a.out, exist_ok=True)
    data = json.load(urllib.request.urlopen(a.ingest_url + "/runs?benchmark=true"))
    runs = data if isinstance(data, list) else data.get("runs") or data.get("data") or data.get("items") or []
    pts = defaultdict(list)  # (machine,codec,height,clip) -> [(kbps,vmaf)]
    for r in runs:
        for rd in (r.get("renditions") or []):
            if (rd.get("vmaf") or 0) > 0:
                pts[(r.get("machineLabel"), rd.get("codec"), rd.get("height"), r.get("clip"))].append(
                    (rd.get("outputBitrateKbps"), rd.get("vmaf")))
    # Aggregate BD-rate vs libx264 anchor (machine c7i.xlarge, codec h264), per
    # (machine, codec, height), averaged across clips with >=4 quality points.
    anchor_machine = "c7i.xlarge"
    rows = []
    by_group = defaultdict(list)  # (machine,codec,height) -> list of clip BD-rates
    # index anchor curves by (codec,height,clip)
    anchor = {}
    for (m, c, h, clip), pl in pts.items():
        if m == anchor_machine and c == "h264":
            anchor[(h, clip)] = sorted(pl)
    for (m, c, h, clip), pl in pts.items():
        ref = anchor.get((h, clip))
        if not ref or len(ref) < 4 or len(pl) < 4:
            continue
        pl = sorted(pl)
        try:
            bd = bd_rate([p[0] for p in ref], [p[1] for p in ref],
                         [p[0] for p in pl], [p[1] for p in pl])
        except ValueError:
            continue
        by_group[(m, c, h)].append(bd)
    print("BD-rate vs libx264@%s (negative = more efficient), %% bitrate at equal VMAF:" % anchor_machine)
    print("| machine | codec | height | BD-rate %% | clips |")
    print("|---|---|---|---|---|")
    for (m, c, h), vals in sorted(by_group.items()):
        rows.append((m, c, h, sum(vals)/len(vals), len(vals)))
        print("| %s | %s | %sp | %+.1f | %d |" % (m, c, h, sum(vals)/len(vals), len(vals)))
    # R-D scatter plots per (codec,height) if matplotlib is available.
    try:
        import matplotlib
        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
        from collections import defaultdict as dd
        curves = dd(lambda: dd(list))  # (codec,height) -> machine -> points
        for (m, c, h, clip), pl in pts.items():
            for kbps, v in pl:
                curves[(c, h)][m].append((kbps, v))
        for (c, h), machines in curves.items():
            plt.figure()
            for m, ptsm in sorted(machines.items()):
                ptsm = sorted(ptsm)
                plt.scatter([p[0] for p in ptsm], [p[1] for p in ptsm], s=8, label=m)
            plt.xscale("log"); plt.xlabel("bitrate (kbps)"); plt.ylabel("VMAF")
            plt.title("R-D: %s %sp" % (c, h)); plt.legend(fontsize=6)
            plt.savefig(os.path.join(a.out, "rd-%s-%sp.png" % (c, h)), dpi=120)
            plt.close()
        print("\nWrote R-D figures to", a.out)
    except ImportError:
        print("\n(matplotlib not installed — skipped figures; pip install matplotlib)")
