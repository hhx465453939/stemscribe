#!/usr/bin/env python
"""Stemscribe vocal-separation helper.

Runs audio-separator (the UVR5 inference stack) in a separate process so that no
tensor code lives in Go (ADR-002). The Go adapter writes this file to
<workdir>/runtimes/separator/helper.py on first use and calls it with explicit
paths; nothing here reads the repository layout.

Protocol (stdout, one line, machine-readable):
    {"vocals": "<abs path>", "instrumental": "<abs path>"}
Failures exit non-zero and print the reason to stderr.

Why the metadata seeding below exists: audio-separator downloads its model lists
from raw.githubusercontent.com when they are missing, and GitHub is unreachable
from the target network (hf-mirror is the only model source). The exact upstream
files ship with Stemscribe and are copied next to the weights, which turns
"separate" into a fully offline operation after the model install.
"""

import argparse
import json
import logging
import os
import shutil
import sys

META_FILES = ("download_checks.json", "mdx_model_data.json", "vr_model_data.json")


def seed_metadata(meta_dir: str, model_dir: str) -> None:
    """Copy the pinned UVR metadata next to the weights if it is absent."""
    for name in META_FILES:
        src = os.path.join(meta_dir, name)
        dst = os.path.join(model_dir, name)
        if os.path.isfile(dst) or not os.path.isfile(src):
            continue
        shutil.copyfile(src, dst)


def pick_outputs(produced, out_dir):
    """Map audio-separator's <input>_(Vocals)_<model>.wav names onto our names.

    The stem marker is matched with its parentheses: matching a bare "vocal"
    also hits the model name (Kim_Vocal_2) and swapped the two files. The API
    returns bare filenames, so they are resolved against the output directory.
    """
    vocals = instrumental = None
    for path in produced:
        if not os.path.isabs(path):
            path = os.path.join(out_dir, path)
        name = os.path.basename(path).lower()
        if "(vocals)" in name:
            vocals = path
        elif "(instrumental)" in name:
            instrumental = path
    return vocals, instrumental


def main() -> int:
    ap = argparse.ArgumentParser(description="Stemscribe separation helper")
    ap.add_argument("--input", required=True)
    ap.add_argument("--vocals-out", required=True)
    ap.add_argument("--instrumental-out", required=True)
    ap.add_argument("--model", required=True, help="model filename, e.g. Kim_Vocal_2.onnx")
    ap.add_argument("--model-dir", required=True, help="directory holding the weights")
    ap.add_argument("--meta-dir", required=True, help="directory holding the pinned UVR metadata")
    ap.add_argument("--sample-rate", type=int, default=44100)
    args = ap.parse_args()

    for label, path in (("input", args.input), ("model", os.path.join(args.model_dir, args.model))):
        if not os.path.isfile(path):
            print(f"{label} not found: {path}", file=sys.stderr)
            return 2

    seed_metadata(args.meta_dir, args.model_dir)

    out_dir = os.path.dirname(os.path.abspath(args.vocals_out))
    os.makedirs(out_dir, exist_ok=True)

    from audio_separator.separator import Separator

    sep = Separator(
        log_level=logging.WARNING,
        model_file_dir=args.model_dir,
        output_dir=out_dir,
        output_format="WAV",
        sample_rate=args.sample_rate,
    )
    sep.load_model(model_filename=args.model)
    produced = sep.separate(args.input) or []

    vocals, instrumental = pick_outputs(produced, out_dir)
    if vocals is None or instrumental is None:
        print(f"unexpected separator output: {produced}", file=sys.stderr)
        return 3

    # Move rather than copy: a 30-minute episode is ~300 MB per stem.
    if os.path.abspath(vocals) != os.path.abspath(args.vocals_out):
        os.replace(vocals, args.vocals_out)
    if os.path.abspath(instrumental) != os.path.abspath(args.instrumental_out):
        os.replace(instrumental, args.instrumental_out)

    print(json.dumps({"vocals": args.vocals_out, "instrumental": args.instrumental_out}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
