#!/usr/bin/env python
"""Stemscribe separation helper.

Runs audio-separator (the UVR5 inference stack) in a separate process so that no
tensor code lives in Go (ADR-002). The Go adapter writes this file to
<workdir>/runtimes/separator/helper.py on first use and calls it with explicit
paths; nothing here reads the repository layout.

Protocol (stdout, one line, machine-readable):
    {"stems": {"vocals": "...", "drums": "..."}, "backend": "cuda12",
     "runtime": "torch", "providers": ["torch.cuda"], ...}
Failures exit non-zero and print the reason to stderr.

Why the metadata seeding below exists: audio-separator downloads its model lists
from raw.githubusercontent.com when they are missing, and GitHub is unreachable
from the target network (hf-mirror is the only model source). The exact upstream
files ship with Stemscribe and are copied next to the weights, which turns
"separate" into a fully offline operation after the model install.

Why the ONNX provider is set by hand: audio-separator only enables CUDA when
torch.cuda.is_available() is true, and the Windows torch wheel on PyPI is
CPU-only. MDX inference itself is pure ONNX Runtime, so the provider is chosen
here and then verified against the sessions audio-separator actually created -- a
run that quietly computes on the CPU is the one failure the user cannot see.

Why torch models take a different path: ONNX Runtime and PyTorch each ship their
own cuDNN. Creating an ONNX CUDA session and then importing torch in the same
process fails on Windows (measured: "[WinError 127] Error loading
torch\\lib\\cudnn_cnn64_9.dll"), so a torch model never touches onnxruntime here.
"""

import argparse
import copy
import glob
import json
import logging
import os
import shutil
import sys
import sysconfig

META_FILES = ("download_checks.json", "mdx_model_data.json", "vr_model_data.json")

# Stems audio-separator can emit, in canonical order. The marker is matched with
# its parentheses: matching a bare "vocal" also hits the model name (Kim_Vocal_2)
# and once swapped the two files.
STEMS = ("vocals", "instrumental", "drums", "bass", "other", "guitar", "piano")


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


def stem_of(filename: str):
    """Map "<input>_(Vocals)_<model>.wav" onto the canonical stem name."""
    name = os.path.basename(filename).lower()
    for stem in STEMS:
        if f"({stem})" in name:
            return stem
    return None


# Upstream defaults, copied from Separator.__init__. They are used as the base
# for every parameter dictionary: a partial dict replaces the whole default in
# audio-separator, so passing only the knob the user changed would silently drop
# hop_length or window_size.
ARCH_DEFAULTS = {
    "mdx": {
        "mdx_params": {"hop_length": 1024, "segment_size": 256, "overlap": 0.25,
                       "batch_size": 1, "enable_denoise": False},
    },
    "vr": {
        "vr_params": {"batch_size": 1, "window_size": 512, "aggression": 5,
                      "enable_tta": False, "enable_post_process": False,
                      "post_process_threshold": 0.2, "high_end_process": False},
    },
    "demucs": {
        "demucs_params": {"segment_size": "Default", "shifts": 2,
                          "overlap": 0.25, "segments_enabled": True},
    },
    "mdxc": {
        "mdxc_params": {"segment_size": 256, "override_model_segment_size": False,
                        "batch_size": None, "overlap": None, "pitch_shift": 0},
    },
}

# Which knobs each architecture honours. Anything else is refused rather than
# passed along to be ignored: the run log has to be able to say what was applied.
ARCH_KNOBS = {
    "mdx": {"segment", "overlap", "batch"},
    "vr": {"batch"},
    "demucs": {"segment", "overlap", "shifts"},
    "mdxc": {"segment", "overlap", "batch"},
    "roformer": set(),
}


def arch_parameter_kwargs(arch: str, segment: int, overlap: float,
                          shifts: int, batch: int):
    """Build the architecture parameter dictionaries for Separator().

    Returns (kwargs, applied, ignored): the constructor kwargs, the values that
    were really applied, and the knob names the architecture does not support.
    """
    if not arch:
        return {}, {}, []
    knobs = ARCH_KNOBS.get(arch, set())
    settings = copy.deepcopy(ARCH_DEFAULTS.get(arch, {}))
    if not settings:
        return {}, {}, []
    (params_key,) = settings.keys()
    params = settings[params_key]

    ignored = []
    applied = {}
    values = [
        ("segment", "segment_size", segment, 0),
        ("overlap", "overlap", overlap, 0.0),
        ("shifts", "shifts", shifts, 0),
        ("batch", "batch_size", batch, 0),
    ]
    for knob, key, value, zero in values:
        if value == zero:
            continue
        if knob not in knobs:
            ignored.append(knob)
            continue
        params[key] = value
        if arch == "mdxc" and key == "segment_size":
            # MDXC keeps the checkpoint's own segment size unless this is set.
            params["override_model_segment_size"] = True
        applied[key] = value
    return settings, applied, ignored


def torch_device_report(accel: str, sep):
    """Report what PyTorch will actually compute on, with a real CUDA operation.

    audio-separator sets sep.torch_device from torch.cuda.is_available() and feeds
    that same value to the model's .to(device) call, so the value is meaningful.
    The tensor operation is what separates "the CUDA wheel is installed" from "the
    driver cannot run it".
    """
    device = str(getattr(sep, "torch_device", "") or "")
    if accel != "cuda12":
        return "cpu", [], "", device
    if device != "cuda":
        return "cpu", [], f"PyTorch device is {device or 'unknown'}: the CUDA torch build is not installed", device
    try:
        import torch

        if float(torch.zeros(1, device="cuda").sum().item()) == 0.0:
            return "cuda12", ["torch.cuda"], "", device
        reason = "a CUDA tensor operation returned an unexpected value"
    except Exception as exc:  # noqa: BLE001 - the message is the diagnosis
        reason = f"CUDA tensor operation failed: {str(exc).splitlines()[-1][:200]}"
    return "cpu", [], reason, device


def main() -> int:
    ap = argparse.ArgumentParser(description="Stemscribe separation helper")
    ap.add_argument("--input", required=True)
    ap.add_argument("--out-dir", required=True, help="directory that receives <stem>.wav files")
    ap.add_argument("--model", required=True, help="model filename, e.g. Kim_Vocal_2.onnx or htdemucs_ft.yaml")
    ap.add_argument("--model-dir", required=True, help="directory holding the weights")
    ap.add_argument("--meta-dir", required=True, help="directory holding the pinned UVR metadata")
    ap.add_argument("--accel", default="auto", choices=("auto", "cuda12", "cpu"),
                    help="requested execution device; auto uses CUDA when the stack can")
    ap.add_argument("--arch", default="",
                    choices=("", "mdx", "vr", "demucs", "mdxc", "roformer"),
                    help="registry architecture; picks the parameter dictionary")
    ap.add_argument("--segment", type=int, default=0,
                    help="segment_size in the architecture's own units (MDX frames, Demucs seconds)")
    ap.add_argument("--overlap", type=float, default=0.0,
                    help="window overlap fraction, e.g. 0.25")
    ap.add_argument("--shifts", type=int, default=0, help="Demucs random shifts")
    ap.add_argument("--batch", type=int, default=0, help="batch size (MDX/VR/MDXC)")
    ap.add_argument("--fp16", action="store_true",
                    help="torch autocast fp16 (CUDA only)")
    ap.add_argument("--sample-rate", type=int, default=44100)
    args = ap.parse_args()

    model_path = os.path.join(args.model_dir, args.model)
    for label, path in (("input", args.input), ("model", model_path)):
        if not os.path.isfile(path):
            print(f"{label} not found: {path}", file=sys.stderr)
            return 2

    seed_metadata(args.meta_dir, args.model_dir)

    out_dir = os.path.abspath(args.out_dir)
    os.makedirs(out_dir, exist_ok=True)

    runtime = "onnx" if args.model.lower().endswith(".onnx") else "torch"

    accel = args.accel
    providers, available, accel_reason = [], [], ""
    sessions = []
    if runtime == "onnx":
        extend_path(nvidia_dll_dirs())
        import onnxruntime as ort

        if accel == "auto":
            accel = "cuda12" if "CUDAExecutionProvider" in ort.get_available_providers() else "cpu"
        providers, available, accel_reason = configure_onnx(accel)
        sessions = record_sessions(ort)
        if accel_reason:
            print("WARNING " + accel_reason, file=sys.stderr)
    elif accel == "auto":
        # auto for a torch model asks torch, never onnxruntime: importing both
        # stacks into one process is what breaks torch's cuDNN load on Windows.
        import torch

        accel = "cuda12" if torch.cuda.is_available() else "cpu"

    from audio_separator.separator import Separator

    sep_kwargs, applied_params, ignored_params = arch_parameter_kwargs(
        args.arch, args.segment, args.overlap, args.shifts, args.batch)
    if ignored_params:
        print("WARNING parameters ignored for arch %s: %s"
              % (args.arch, ", ".join(ignored_params)), file=sys.stderr)

    use_fp16 = bool(args.fp16)
    if use_fp16 and runtime != "torch":
        print("WARNING --fp16 ignored: the MDX Runtime path has no autocast", file=sys.stderr)
        use_fp16 = False
    if use_fp16 and accel != "cuda12":
        print("WARNING --fp16 ignored: autocast needs the CUDA torch build", file=sys.stderr)
        use_fp16 = False
    if use_fp16:
        sep_kwargs["use_autocast"] = True

    sep = Separator(
        log_level=logging.WARNING,
        model_file_dir=args.model_dir,
        output_dir=out_dir,
        output_format="WAV",
        sample_rate=args.sample_rate,
        **sep_kwargs,
    )
    if runtime == "onnx":
        # The CPU torch wheel closes audio-separator's own CUDA gate even when
        # onnxruntime-gpu is installed. The provider is what decides the device.
        sep.onnx_execution_provider = providers
    sep.load_model(model_filename=args.model)
    produced = sep.separate(args.input) or []

    if runtime == "onnx":
        active = sorted({p for session in sessions for p in session})
        backend = "cuda12" if "CUDAExecutionProvider" in active else "cpu"
        reason = accel_reason
        if accel == "cuda12" and backend == "cpu" and not reason:
            reason = "onnxruntime created its session without CUDAExecutionProvider"
        device = ""
    else:
        backend, torch_providers, reason, device = torch_device_report(accel, sep)
        active = torch_providers
        available = ["torch.cuda", "torch.cpu"]
    if accel == "cuda12" and backend == "cpu":
        print(f"WARNING separation fell back to CPU: {reason}", file=sys.stderr)

    # Move rather than copy: a 30-minute episode is ~300 MB per stem. The long
    # upstream names are replaced by <stem>.wav so the bundle layout is fixed.
    stems = {}
    for path in produced:
        if not os.path.isabs(path):
            path = os.path.join(out_dir, path)
        stem = stem_of(path)
        if stem is None:
            continue
        target = os.path.join(out_dir, f"{stem}.wav")
        if os.path.abspath(path) != os.path.abspath(target):
            os.replace(path, target)
        stems[stem] = target

    if "vocals" not in stems:
        print(f"unexpected separator output: {produced}", file=sys.stderr)
        return 3

    print(json.dumps({
        "stems": stems,
        "backend": backend,
        "runtime": runtime,
        "device": device,
        "providers": active,
        "requested": accel,
        "available": available,
        "arch": args.arch,
        "params": applied_params,
        "fp16": use_fp16,
        "ignored_params": ignored_params,
        "reason": reason,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())
