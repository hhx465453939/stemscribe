<h1 align="center">Stemscribe</h1>

<p align="center">
  <b>Turn any video into an archival-grade transcript — entirely on your own GPU.</b><br>
  <sub>Local-first video → audio → text → polish pipeline for Windows. No cloud transcription.</sub>
</p>

<p align="center">
  <img src="docs/images/app-main-dark.png" alt="Stemscribe — dark theme" width="49%">
  <img src="docs/images/app-main-light.png" alt="Stemscribe — light theme" width="49%">
</p>

<p align="center">
  <sub><b>Two themes, one design language.</b> Dark for a dim room, light for a projector. Both Chinese and English, switchable in the sidebar.</sub>
</p>

<p align="center">
  <a href=".github/workflows/ci.yml"><img alt="ci" src="https://github.com/hhx465453939/stemscribe/actions/workflows/ci.yml/badge.svg"></a>
  <img alt="license" src="https://img.shields.io/badge/license-MIT-c9a227">
  <img alt="go" src="https://img.shields.io/badge/go-1.23-00ADD8">
  <img alt="next" src="https://img.shields.io/badge/next-15-000000">
  <img alt="i18n" src="https://img.shields.io/badge/i18n-中文%20%7C%20EN-6b7076">
</p>

<p align="center"><a href="README.zh-CN.md">中文说明 →</a></p>

---

## Why this exists

The parts already exist: a Bilibili downloader, UVR for vocal separation, Whisper for
transcription, an LLM for cleanup. What does not exist is the **seam** — and the seam is
where all the work is.

| Before | Stemscribe |
|---|---|
| Four apps, files dragged by hand | One click, staged pipeline |
| Models scattered across drives | One portable work directory, copy it anywhere |
| "Which model did I use last time?" | Provenance recorded per artifact |
| 200 episodes = 200 manual rounds | Batch queue, resumable, retry just the failures |
| Transcript stuck in a chat window | Structured bundle you can grep, back up, hand to an agent |

## What it does

```
source ──► download ──► demux ──► separate ──► transcribe ──► align ──► polish ──► export
 (URL /                        (vocals /      (whisper.cpp,   (sentence   (optional   (bundle +
  file)                         instrumental)  GGML, on GPU)    repair)     LLM)        subtitles)
```

- **Local by default.** Audio and video never leave the machine. Only *text* may reach an LLM, and only when you switch polishing on.
- **You control the VRAM.** A budget scheduler reads real VRAM, keeps models resident (LRU), streams audio in segments, and downgrades explicitly: `CUDA → Vulkan → DirectML → CPU`. Downgrades are always surfaced with a reason.
- **Bring your own model.** whisper.cpp builds for CUDA / Vulkan / DirectML / CPU, installed on demand into your work directory with sha256 verification.
- **Bring your own LLM.** Any OpenAI-compatible endpoint (OpenAI, DeepSeek, Kimi, OpenRouter, Ollama, vLLM, LM Studio, one-api…) or Anthropic. Paste a URL and key, hit **Refresh**, pick a model.
- **Every capability is visible.** Context window, max output, reasoning depth, vision, tool calling, sampling — inspectable and editable per model, with unsupported fields **greyed out and explained**.
- **Reproducible artifacts.** Each episode is a self-contained directory; changing a parameter invalidates only the stages that depend on it.

## Themes and languages

Both are first-class, not an afterthought bolted onto a dark-only UI.

| | |
|---|---|
| **Light** | The Swiss grid in daylight: paper white, hairline rules, one deep amber accent. For projectors, printouts and screenshots in docs. |
| **Dark** | The same grid at lights-out: a dark field where a single warm pool of light marks the one task that is running. |
| **中文 / English** | Every visible string comes from one typed dictionary. The type checker fails the build if a key is missing from either locale. |
| **Defaults** | Theme follows the OS, language follows the browser. Both are remembered once you touch the switch. |
| **Deep links** | `?theme=light&lang=en` — deterministic for screenshots and documentation, and it never overwrites your saved preference. |

<p align="center">
  <img src="docs/images/app-main-en.png" alt="English UI, dark theme" width="70%"><br>
  <sub>The English build, dark theme. Language names (<code>中文</code> / <code>EN</code>) deliberately stay untranslated.</sub>
</p>

## Design

The interface is built from two art genes, and deliberately only two.

| | |
|---|---|
| **Swiss grid** (structure) | 12-column grid, one sans-serif family, a four-step type scale (32 / 20 / 14 / 11), 1px hairlines, **one radius (2px)**, flat surfaces — hierarchy from surface tint and rules, never from drop shadows. |
| **Single light source** (focus) | Exactly one thing is lit: the task currently running gets a warm surface and a 2px amber rule; finished work recedes to hairline grey. |
| **Palette** | Four colours per theme. Light: paper `#f7f7f5` / white / ink `#14161a` / amber `#8a5f0e`. Dark: `#0a0b0d` / `#14171b` / bone `#e9e6e0` / amber `#c9a227`. |
| **Type** | Inter (UI), JetBrains Mono (timestamps, VRAM, token counts), Noto Sans SC. |

The light theme is **not** an inverted dark theme — an inversion gives you greyish whites and
dirty blacks and loses the "one light source" meaning entirely. Both themes are derived from
the same gene under different lighting, and both are audited:

```
$ python scripts/contrast.py
light  text-3   #6b7076  #f7f7f5   4.66  PASS
dark   text-3   #7d848c  #0a0b0d   5.20  PASS
all pairs >= 4.5:1 in both themes
```

Why so austere: this is an instrument, not a dashboard. A finished progress bar that competes
with a running one for attention is a bug. Blue/purple gradients, glassmorphism, emoji status
icons and celebratory confetti are explicitly rejected — see [`docs/UI-DESIGN.md`](docs/UI-DESIGN.md)
for the token table (§14) and the anti-pattern list (§11).

## Screenshots

<table>
<tr>
<td width="50%"><img src="docs/images/app-settings-dark.png" alt="Provider settings, dark"><br>
<sub><b>Provider settings (dark).</b> Capability matrix per model; unsupported fields are greyed out with a reason.</sub></td>
<td width="50%"><img src="docs/images/app-settings-light.png" alt="Provider settings, light"><br>
<sub><b>Provider settings (light).</b> Same matrix in daylight. Nothing is hidden, nothing silently ignored.</sub></td>
</tr>
<tr>
<td width="50%"><img src="docs/images/app-models-dark.png" alt="Model centre"><br>
<sub><b>Model centre.</b> Size, VRAM estimate and licence shown before you download.</sub></td>
<td width="50%">&nbsp;</td>
</tr>
</table>

## Install

> Status: **v0.0.x — scaffold**. Orchestration, artifacts, theming and the UI shell work; real
> inference lands in CHG-0002. See [`docs/ROADMAP.md`](docs/ROADMAP.md).

Grab a release, unzip, run `stemscribe.exe`. It serves the embedded UI on loopback and opens
your browser (`127.0.0.1` only — nothing is exposed to the network).

- WebView2 runtime is already present on Windows 10 1803+ / Windows 11
- NVIDIA GPU optional — the CPU path always works
- **Unsigned build**: SmartScreen will warn. Verify against the release's `SHA256SUMS.txt`

## Build from source

```powershell
# Prerequisites: Go >= 1.23, Node >= 22, pnpm >= 11 (pnpm 11 requires Node >= 22.13)
pnpm --dir frontend install
.\scripts\build.ps1              # -> dist/stemscribe.exe (UI embedded, single file)
.\scripts\smoke-test.ps1         # 20 assertions over the bundle contract
bash scripts/verify-contract.sh  # CHG + docs + schemas
bash scripts/i18n-check.sh       # theme tokens + locale parity + no hardcoded CJK
python scripts/contrast.py       # WCAG AA audit for both themes
```

`stemscribe.exe` with no arguments opens the UI. The CLI stays available for automation:

```powershell
.\dist\stemscribe.exe run --input .\episode.mp4 --duration-ms 30000
.\dist\stemscribe.exe probe
```

## Architecture at a glance

| Layer | Choice |
|---|---|
| Shell | single Go binary; embedded static UI served on loopback (`Wails/WebView2` is the planned native shell — ADR-001) |
| UI | Next.js 15 static export, design tokens in plain CSS, no UI framework |
| i18n | typed flat dictionary, locale parity enforced at compile time (ADR-010) |
| Inference | out-of-process engines over a JSONL protocol (`whisper.cpp`, `audio-separator`, `ffmpeg`) |
| Models | registry-declared, on-demand download, sha256 verified, atomically installed |
| Scheduling | VRAM budget → LRU residency → segmented streaming → four-level backend fallback |
| LLM | OpenAI-compatible + Anthropic adapters, three-tier capability resolution |

Details: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) · decisions: [`docs/DECISIONS.md`](docs/DECISIONS.md)

## Documentation

| Doc | Contents |
|---|---|
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Layers, process model, IPC contract, error model |
| [`docs/PIPELINE.md`](docs/PIPELINE.md) | Stage DAG, idempotency, alignment repair, failure matrix |
| [`docs/MODEL-REGISTRY.md`](docs/MODEL-REGISTRY.md) | Registry schema, download/verify flow, licensing |
| [`docs/RESOURCE-SCHEDULER.md`](docs/RESOURCE-SCHEDULER.md) | VRAM budget, LRU, segmentation, RAM/IO strategy |
| [`docs/SEPARATION-BENCH.md`](docs/SEPARATION-BENCH.md) | Measured separation throughput, VRAM peaks, GPU-vs-CPU fidelity |
| [`docs/LLM-PROVIDERS.md`](docs/LLM-PROVIDERS.md) | Provider abstraction, capability resolution, parameter projection |
| [`docs/UI-DESIGN.md`](docs/UI-DESIGN.md) | Design genes, dual-theme token table, anti-patterns |
| [`docs/UPSTREAM-RESEARCH.md`](docs/UPSTREAM-RESEARCH.md) | What we borrowed from UVR / Buzz / WhisperDesktop |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Milestones and change contracts |

## Credits

Stemscribe orchestrates other people's excellent work:
[whisper.cpp](https://github.com/ggml-org/whisper.cpp) ·
[Ultimate Vocal Remover](https://github.com/Anjok07/ultimatevocalremovergui) ·
[audio-separator](https://github.com/nomadkaraoke/python-audio-separator) ·
[Buzz](https://github.com/chidiwilliams/buzz) ·
[Const-me/Whisper](https://github.com/Const-me/Whisper) ·
[bilibili-video-downloader](https://github.com/lanyeeee/bilibili-video-downloader) ·
[FFmpeg](https://ffmpeg.org/)

**We do not redistribute third-party model weights.** The registry only records where to get
them and how to verify them.

## License

MIT — see [`LICENSE`](LICENSE). Please respect platform terms and copyright law: only process
material you have the right to process.
