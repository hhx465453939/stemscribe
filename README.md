# Stemscribe

**Turn any video into an archival-grade transcript — entirely on your own GPU.**

Local-first video → audio → transcript → polished notes pipeline for Windows.
No cloud transcription. No uploading your media. One window, one work directory, reproducible output.

[中文说明 →](README.zh-CN.md)

---

## Why

The pieces already exist: a Bilibili downloader, UVR for vocal separation, Whisper for transcription,
an LLM for cleanup. What doesn't exist is the **seam** — and the seam is where all the work is.

Stemscribe is that seam:

| Before | After |
|---|---|
| 4 apps, files dragged by hand | 1 click, staged pipeline |
| Models scattered across `F:\` and `E:\` | Everything in one portable work directory |
| "which model did I use last time?" | Provenance recorded per artifact |
| 200 episodes = 200 manual rounds | Batch queue, resumable, retry failed items |
| Transcript stuck in a chat window | Structured bundle you can grep, back up, and hand to an agent |

## What it does

```
source ──► download ──► demux ──► separate ──► transcribe ──► align ──► polish ──► export
 (URL/                          (vocals /     (whisper.cpp,   (sentence   (optional   (bundle +
  file)                          instrumental)  GGML, GPU)      repair)     LLM)        subtitles)
```

- **Local by default.** Audio and video never leave the machine. Only the *text* may be sent to an LLM, and only if you turn polishing on.
- **Bring your own model.** whisper.cpp backends: CUDA / Vulkan / DirectML / CPU, auto-selected, with an explicit downgrade chain you can see.
- **Bring your own LLM.** Any OpenAI-compatible endpoint (OpenAI, DeepSeek, Kimi, OpenRouter, Ollama, vLLM, LM Studio, one-api…) or Anthropic. Point at a URL, paste a key, hit **Refresh**, pick a model.
- **Every capability is visible.** Context window, max output tokens, reasoning depth, vision, tool calling, sampling knobs — all inspectable and editable per model, with unsupported fields greyed out *and explained*.
- **Reproducible artifacts.** Each episode is a self-contained directory; rerunning with different params invalidates only the stages that depend on them.

## Install

> Status: **v0.0.x — scaffold**. Functional pipeline lands in v0.1.0. See [`docs/ROADMAP.md`](docs/ROADMAP.md).

- Portable zip, no installer, no admin rights
- WebView2 runtime (already present on Windows 10 1803+ / Windows 11)
- NVIDIA GPU optional — CPU path always works
- **Unsigned build**: SmartScreen will warn. Verify downloads against the release's `SHA256SUMS.txt`.

## Build from source

```powershell
# Prerequisites: Go ≥1.23, Node ≥20, pnpm ≥9, MSYS2 (GCC for Wails), WebView2 runtime
pnpm install
pnpm --dir frontend install
.\scripts\dev.ps1           # dev: wails dev + next dev
.\scripts\build.ps1         # produce build/bin/Stemscribe.exe
.\scripts\smoke-test.ps1    # end-to-end smoke (synthetic audio, no network)
```

CI runs on `windows-latest`; releases are produced by pushing a tag (`v0.1.0-rc1`).

## Design

Dark Swiss grid with a single light source — a precision instrument, not a dashboard.
Rationale, tokens and anti-patterns: [`docs/UI-DESIGN.md`](docs/UI-DESIGN.md).

## Architecture at a glance

| Layer | Choice |
|---|---|
| Shell | Wails v2 (Go + WebView2), single ~25 MB binary |
| UI | Next.js 15 static export |
| Inference | Out-of-process engines driven by a JSONL protocol (`whisper.cpp`, `audio-separator`, `ffmpeg`) |
| Models | Registry-declared, on-demand download, sha256-verified, atomically installed into your work dir |
| Scheduling | NVML VRAM budget → LRU residency → segmented streaming → four-level backend fallback |
| LLM | OpenAI-compatible + Anthropic adapters, three-tier capability resolution |
| Agent | Local MCP server (search / get / context-pack) |

Details: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) · decisions: [`docs/DECISIONS.md`](docs/DECISIONS.md)

## Documentation

| Doc | Contents |
|---|---|
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Layers, process model, IPC contract, error model |
| [`docs/PIPELINE.md`](docs/PIPELINE.md) | Stage DAG, idempotency, alignment repair, failure matrix |
| [`docs/MODEL-REGISTRY.md`](docs/MODEL-REGISTRY.md) | Registry schema, download/verify flow, licensing |
| [`docs/RESOURCE-SCHEDULER.md`](docs/RESOURCE-SCHEDULER.md) | VRAM budget, LRU, segmentation, RAM/IO strategy |
| [`docs/LLM-PROVIDERS.md`](docs/LLM-PROVIDERS.md) | Provider abstraction, capability resolution, parameter projection |
| [`docs/UI-DESIGN.md`](docs/UI-DESIGN.md) | Design genes, tokens, components, anti-patterns |
| [`docs/UPSTREAM-RESEARCH.md`](docs/UPSTREAM-RESEARCH.md) | What we borrowed from UVR / Buzz / WhisperDesktop / the Bilibili downloader |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Milestones and change contracts |

## Credits

Stemscribe orchestrates other people's excellent work:

- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) — MIT
- [Ultimate Vocal Remover](https://github.com/Anjok07/ultimatevocalremovergui) — model families and segmentation practice
- [audio-separator](https://github.com/nomadkaraoke/python-audio-separator) — ONNX separation runtime
- [Buzz](https://github.com/chidiwilliams/buzz) — 16 kHz pipeline and compute-type practice
- [Const-me/Whisper](https://github.com/Const-me/Whisper) — single-binary distribution and model residency
- [bilibili-video-downloader](https://github.com/lanyeeee/bilibili-video-downloader) — desktop packaging reference
- [FFmpeg](https://ffmpeg.org/) — LGPL/GPL, downloaded at runtime

**We do not redistribute third-party model weights.** The registry only records where to get them and how to verify them.

## License

MIT — see [`LICENSE`](LICENSE). Third-party notices: `THIRD-PARTY-NOTICES.md` (added with the first runtime integration).
