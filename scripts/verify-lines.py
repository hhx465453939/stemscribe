"""AC-01 / AC-02 verification: sentence splitting vs unlimited line length.

Builds no paths from non-ASCII literals on purpose -- a Chinese path typed through a
shell ends up as GBK bytes in the JSON body and the backend then sees mojibake. The
glob keeps this harness ASCII-only so it tests the app, not the terminal codepage.
"""
import glob
import json
import statistics
import sys
import time
import urllib.request

BASE = sys.argv[1] if len(sys.argv) > 1 else ""
if not BASE:
    raise SystemExit("usage: verify.py http://127.0.0.1:PORT")

videos = sorted(glob.glob("E:/VideoDownload/*40k*/*.mp4"))
if not videos:
    raise SystemExit("no video found under E:/VideoDownload/*40k*/")
VIDEO = videos[0]


def post(path, payload):
    req = urllib.request.Request(
        BASE + path,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json; charset=utf-8"},
        method="POST",
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode("utf-8"))


def get(path):
    with urllib.request.urlopen(BASE + path) as resp:
        return json.loads(resp.read().decode("utf-8"))


def run(out_dir, max_len):
    params = {"input_language": "zh"}
    if max_len is not None:
        params["max_len"] = str(max_len)
        params["split_on_word"] = "true"
    post("/api/queue", {"inputs": [VIDEO], "output_dir": out_dir, "params": params})
    deadline = time.time() + 900
    while time.time() < deadline:
        time.sleep(6)
        items = get("/api/queue")["items"]
        target = [i for i in items if out_dir.replace("\\", "/") in (i.get("artifact_dir") or "") or True]
        last = items[-1]
        if last["state"] in ("done", "failed"):
            return last
    raise SystemExit("timeout waiting for run")


def widths(bundle_dir):
    rows = []
    with open(bundle_dir + "/text/transcript.md", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line.startswith("["):
                continue
            text = line.split("]", 1)[1].strip()
            if text:
                rows.append(len(text))
    return rows


for label, max_len, out in (("max_len=30", 30, "C:/Temp/q30"), ("max_len=0", 0, "C:/Temp/q0")):
    item = run(out, max_len)
    print(f"{label}: state={item['state']} segments={item['segments']} msg={item['message'][:120]}")
    if item["state"] == "done":
        w = widths(item["artifact_dir"].replace("\\", "/"))
        print(
            f"  lines={len(w)} median={statistics.median(w):.0f} chars "
            f"p90={sorted(w)[int(len(w) * 0.9)]} max={max(w)}"
        )
