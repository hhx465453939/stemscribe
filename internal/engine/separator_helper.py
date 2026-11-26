#!/usr/bin/env python
"""Stemscribe vocal-separation helper.

Runs audio-separator (the UVR5 inference stack) in a separate process so that no
tensor code lives in Go (ADR-002). The Go adapter writes this file to
<workdir>/runtimes/separator/helper.py on first use and calls it with explicit
paths; nothing here reads the repository layout.

Protocol (stdout, one line, machine-readable):
    {"vocals": "...", "instrumental": "...", "backend": "cuda12", "providers": [...]}
Failures exit non-zero and print the reason to stderr.

Why the metadata seeding below exists: audio-separator downloads its model lists
from raw.githubusercontent.com when they are missing, and GitHub is unreachable
from the target network (hf-mirror is the only model source). The exact upstream
files ship with Stemscribe and are copied next to the weights, which turns
"separate" into a fully offline operation after the model install.

Why the ONNX provider is set by hand: audio-separator only enables CUDA when
torch.cuda.is_available() is true, and the Windows torch wheel is CPU-only. MDX
inference itself is pure ONNX Runtime, so the provider is chosen here and then
verified against the sessions audio-separator actually created -- a run that
quietly computes on the CPU is the one failure the user cannot see.
"""

import argparse
import glob
import json
import logging
import os
import shutil
import sys
import sysconfig

META_FILES = ("download_checks.json", "mdx_model_data.json", "vr_model_data.json")


def seed_metadata(meta_dir: str, model_dir: str) -> None:
    """Copy the pinned UVR metadata next to the weights if it is absent."""
    for name in META_FILES:
        src = os.path.join(meta_dir, name)
        dst = os.path.join(model_dir, name)
        if os.path.isfile(dst) or not os.path.isfile(src):
            continue
        shutil.copyfile(src, dst)


def nvidia_dll_dirs() -> list:
    """DLL directories installed by the nvidia-*-cu12 pip packages.

    onnxruntime-gpu links cublas, cufft and cudnn at load time. Windows resolves
    those through PATH, and pip puts them under site-packages/nvidia/<lib>/bin.
    """
    dirs = []
    for key in ("purelib", "platlib"):
        root = sysconfig.get_paths().get(key)
        if root:
            dirs.extend(sorted(glob.glob(os.path.join(root, "nvidia", "*", "bin"))))
    return dirs


def extend_path(dirs) -> None:
    if not dirs:
        return
    current = os.environ.get("PATH", "")
    os.environ["PATH"] = os.pathsep.join(list(dirs) + ([current] if current else []))


def configure_onnx(accel: str):
    """Pick ONNX providers, refusing to pretend a missing GPU is present.

    Returns (providers, available, reason). A non-empty reason means CUDA was
    requested but cannot be used; the caller still runs, on the CPU, but reports
    the downgrade in its result instead of hiding it.
    """
    import onnxruntime as ort

    available = list(ort.get_available_providers())
    if accel == "cuda12":
        if "CUDAExecutionProvider" in available:
            return ["CUDAExecutionProvider", "CPUExecutionProvider"], available, ""
        reason = (
            "CUDAExecutionProvider is not available (onnxruntime reports "
            f"{available}); the separation runtime was installed without its CUDA libraries"
        )
        return ["CPUExecutionProvider"], available, reason
    return ["CPUExecutionProvider"], available, ""


def record_sessions(ort):
    """Record the providers every ONNX session actually activated."""
    created = []
    original = ort.InferenceSession

    class _RecordingSession(original):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, **kwargs)
            created.append(list(self.get_providers()))

    ort.InferenceSession = _RecordingSession
    return created


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
    ap.add_argument("--accel", default="auto", choices=("auto", "cuda12", "cpu"),
                    help="requested execution device; auto uses CUDA when onnxruntime can")
    ap.add_argument("--sample-rate", type=int, default=44100)
    args = ap.parse_args()

    for label, path in (("input", args.input), ("model", os.path.join(args.model_dir, args.model))):
        if not os.path.isfile(path):
            print(f"{label} not found: {path}", file=sys.stderr)
            return 2

    seed_metadata(args.meta_dir, args.model_dir)

    out_dir = os.path.dirname(os.path.abspath(args.vocals_out))
    os.makedirs(out_dir, exist_ok=True)

    extend_path(nvidia_dll_dirs())
    import onnxruntime as ort

    accel = args.accel
    if accel == "auto":
        accel = "cuda12" if "CUDAExecutionProvider" in ort.get_available_providers() else "cpu"
    providers, available, accel_reason = configure_onnx(accel)
    if accel_reason:
        print("WARNING " + accel_reason, file=sys.stderr)

    from audio_separator.separator import Separator

    sessions = record_sessions(ort)

    sep = Separator(
        log_level=logging.WARNING,
        model_file_dir=args.model_dir,
        output_dir=out_dir,
        output_format="WAV",
        sample_rate=args.sample_rate,
    )
    # The Windows torch wheel is CPU-only, so audio-separator's own CUDA gate is
    # closed even when onnxruntime-gpu is installed. The gate guards torch usage
    # that this MDX path never exercises; the provider is what decides the device.
    sep.onnx_execution_provider = providers
    sep.load_model(model_filename=args.model)
    produced = sep.separate(args.input) or []

    active = sorted({p for session in sessions for p in session})
    backend = "cuda12" if "CUDAExecutionProvider" in active else "cpu"
    reason = accel_reason
    if accel == "cuda12" and backend == "cpu" and not reason:
        reason = "onnxruntime created its session without CUDAExecutionProvider"
    if accel == "cuda12" and backend == "cpu":
        print(f"WARNING separation fell back to CPU: {reason}", file=sys.stderr)

    vocals, instrumental = pick_outputs(produced, out_dir)
    if vocals is None or instrumental is None:
        print(f"unexpected separator output: {produced}", file=sys.stderr)
        return 3

    # Move rather than copy: a 30-minute episode is ~300 MB per stem.
    if os.path.abspath(vocals) != os.path.abspath(args.vocals_out):
        os.replace(vocals, args.vocals_out)
    if os.path.abspath(instrumental) != os.path.abspath(args.instrumental_out):
        os.replace(instrumental, args.instrumental_out)

    print(json.dumps({
        "vocals": args.vocals_out,
        "instrumental": args.instrumental_out,
        "backend": backend,
        "providers": active,
        "requested": accel,
        "available": available,
        "reason": reason,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())
