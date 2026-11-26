# 模型与运行时注册表

> 对应 F-08 / F-09，决策见 ADR-003。Schema 文件：`docs/schema/model-registry.schema.json`。

## 1. 两类可下载物

| 类型 | 说明 | 安装位置 | 校验 |
|---|---|---|---|
| `runtime` | 可执行的引擎与依赖（whisper.cpp 二进制、python-embed、ffmpeg） | `<workdir>/runtimes/<engine>/<version>/` | sha256 + 解压后目录指纹 |
| `model` | 神经网络权重（gguf/bin/onnx） | `<workdir>/models/<family>/<id>/` | sha256（强校验，不可跳过） |

> 运行时**不含**任何模型；模型**不含**任何可执行代码。二者解耦，便于独立升级与许可隔离。

## 2. registry.json 结构

```jsonc
{
  "schema": 1,
  "generated_at": "2026-11-17T00:00:00Z",
  "mirrors": [
    { "id": "official", "base_url": "https://example.invalid/stemscribe", "priority": 0 },
    { "id": "cn",       "base_url": "https://mirror.example.invalid/stemscribe", "priority": 10 }
  ],
  "runtimes": [
    {
      "id": "whispercpp-cuda12-win-x64",
      "engine": "whisper",
      "version": "1.7.4",
      "backend": "cuda12",
      "platform": "windows-x64",
      "archive": { "url": "whispercpp/1.7.4/win-x64-cuda12.zip",
                   "size": 48234496,
                   "sha256": null,
                   "format": "zip" },
      "entrypoint": "whisper-cli.exe",
      "requires": { "cuda": ">=12.0", "driver": ">=525.0" },
      "vram_overhead_mb": 320
    }
  ],
  "models": [
    {
      "id": "whisper-medium-q5_0",
      "family": "whisper",
      "engine": "whisper",
      "display_name": "Whisper Medium (q5_0)",
      "langs": ["multi"],
      "files": [
        { "url": "models/whisper/ggml-medium-q5_0.bin",
          "size": 539000000, "sha256": null, "dest": "ggml-medium-q5_0.bin" }
      ],
      "vram_estimate_mb": 2600,
      "ram_estimate_mb": 1800,
      "disk_after_install_mb": 540,
      "quality_tier": "balanced",
      "license": { "spdx": "MIT", "url": "https://github.com/openai/whisper/blob/main/LICENSE" },
      "source": "https://huggingface.co/ggerganov/whisper.cpp"
    },
    {
      "id": "uvr-mdx-kim-vocal-2",
      "family": "separator",
      "engine": "separator",
      "display_name": "UVR5 · Kim Vocal 2 (MDX-NET)",
      "architecture": "mdx",
      "params": { "segment_size": true, "overlap": true, "batch_size": true },
      "stems": ["vocals", "instrumental"],
      "files": [
        { "url": "https://hf-mirror.com/lainlives/audio-separator-models/resolve/main/Kim_Vocal_2.onnx",
          "size": 66759214,
          "sha256": "ce74ef3b6a6024ce44211a07be9cf8bc6d87728cc852a68ab34eb8e58cde9c8b",
          "dest": "Kim_Vocal_2.onnx" }
      ],
      "vram_estimate_mb": 2300,
      "ram_estimate_mb": 3200,
      "disk_after_install_mb": 64,
      "quality_tier": "max",
      "segment_default_s": 15,
      "license": { "spdx": "MIT", "url": "https://github.com/Anjok07/ultimatevocalremovergui" },
      "source": "https://huggingface.co/lainlives/audio-separator-models"
    },
    {
      "id": "demucs-htdemucs",
      "family": "separator",
      "engine": "separator",
      "display_name": "Demucs v4 · htdemucs (4 stems)",
      "architecture": "demucs",
      "params": { "segment_size": true, "overlap": true, "shifts": true },
      "stems": ["vocals", "drums", "bass", "other"],
      "files": [
        { "url": "https://hf-mirror.com/lainlives/audio-separator-models/resolve/main/htdemucs.yaml",
          "size": 21, "sha256": "239c445d0b14454d541ad8bd9bb271c9e536d267e8a4625208744cbb2e7bb66c", "dest": "htdemucs.yaml" },
        { "url": "https://hf-mirror.com/lainlives/audio-separator-models/resolve/main/955717e8-8726e21a.th",
          "size": 84141911, "sha256": "8726e21a993978c7ba086d3872e7608d7d5bfca646ca4aca459ffda844faa8b4", "dest": "955717e8-8726e21a.th" }
      ],
      "vram_estimate_mb": 3800,
      "disk_after_install_mb": 81,
      "quality_tier": "balanced",
      "license": { "spdx": "CC-BY-NC-4.0", "nonCommercial": true,
                   "url": "https://github.com/facebookresearch/demucs/blob/main/LICENSE" },
      "source": "https://huggingface.co/lainlives/audio-separator-models"
    }
  ]
}
```

### 2.1 分离模型族（`architecture` 决定运行时与参数面）

| `architecture` | 推理栈 | 参数面（`params`） | 加载方式 |
|---|---|---|---|
| `mdx` | ONNX Runtime | `segment_size`（STFT 帧）、`overlap`、`batch_size` | 按文件尾部 MD5 查 `mdx_model_data.json` |
| `vr` | PyTorch | `batch_size` | 按文件尾部 MD5 查 `vr_model_data.json` |
| `demucs` | PyTorch | `segment_size`（**秒**）、`overlap`、`shifts` | YAML 点名同目录 `.th` |
| `mdxc` | PyTorch | `segment_size`、`overlap`、`batch_size` | `.ckpt` + YAML 配置 |
| `roformer` | PyTorch | 无（audio-separator 0.48 不开放 roformer 参数） | YAML + `.ckpt` |

MDX/VR 的「尾部 MD5」是 audio-separator `get_model_hash()` 的实现：文件 ≥ 10 240 000 字节时
只哈希最后 10 240 000 字节。参数的 MD5 不在 `model_data` 里就加载失败（`Unsupported Model File`），
所以**选模型前必须先验证 MD5 在 `internal/engine/separator_meta/*.json` 内**——体积和 sha256
都对也救不了。CHG-0018 因此移除了 `uvr-vr-hp2-vocals`（MD5 不在上游数据里）。

Demucs 的 YAML 决定入口文件：`{ "models": ["955717e8"] }` 经前缀匹配找到同目录的
`955717e8-8726e21a.th`。因此 `FindModel` 要求**清单全在**才判定「已安装」，
并把 YAML 交给 audio-separator（不是第一个文件）。

多文件模型的 `VERIFIED.json` 额外记录 `files[]`（dest/sha256/size），单文件模型保持原格式。

### 2.2 字段语义（易误解的才列）

| 字段 | 语义 | 陷阱 |
|---|---|---|
| `mirrors[].priority` | 越小越先试 | 失败才降级到下一镜像，不是并发 |
| `archive.sha256` | **整包**校验 | 与解压后逐文件指纹分开：前者防传输损坏，后者防运行时目录被污染 |
| `vram_estimate_mb` | 稳态显存占用**经验值** | 仅用于装前预检；真实值由首次运行后校准并写 `config/calibration.json`。分离模型装 CUDA 运行时实测 **2300 MB**（120 s 素材，RTX 3060，nvidia-smi 2372 → 4670 MiB）；只装 CPU 运行时不占显存。whisper 族必须 >0 |
| `disk_after_install_mb` | 安装后占用（含解压） | 预检用这个，不是 `size` |
| `quality_tier` | `light` / `balanced` / `max` | 对应首次向导三档，不是质量评分 |
| `license.spdx` | 权重许可 | UVR 系权重许可各异，registry 不做镜像分发，只指官方源 |
| `stems` | 该模型**实际能输出**的分轨 | 能力在**模型上**，不是全局开关（ADR-019）：MDX-Net / VR 是 2 轨（`vocals`+`instrumental`），Demucs 是 4 轨（`drums`/`bass`/`other`/`vocals`）。校验强制 separator 族必须声明 ≥2 轨且含 `vocals`——否则 UI 无法判断该提供几路输出，猜错会静默丢掉模型已经产出的音轨 |

## 3. 下载与安装流程

```
选目标 → 磁盘预算预检 → 逐文件：
   ① HEAD 取 content-length（不可用则信 registry.size）
   ② 检查本地 .part 的 size，从断点续传（Range: bytes=N-）
   ③ 分片并发（默认 4 片，512 KB/片），限速可配
   ④ 落 .part.partial → fsync
   ⑤ sha256 全量校验（流式计算，不整文件入内存）
   ⑥ 通过则 rename 为最终名（同一卷内原子）
   ⑦ 失败则保留 .part 供续传，记录错误
→ 解压（runtime 类）到临时目录 → 逐文件指纹比对 → 原子 rename 到正式目录
→ 写 models.lock.json（id → sha256 + install_time + source_url）
```

**分离运行时是一个例外**：Python 环境带绝对路径和平台解释器，无法作为 registry archive 分发。
`stemscribe runtime install separator-python` 用 uv（优先）或 `python -m venv` 建
`runtimes/separator/venv`，再按硬件从 PyPI 安装两种变体之一：

| 变体 | pip 包 | 何时选 |
|---|---|---|
| CUDA | `audio-separator` + `onnxruntime-gpu==1.21.1` + `nvidia-cublas-cu12` + `nvidia-cudnn-cu12` + `nvidia-cufft-cu12` + `nvidia-curand-cu12` + `nvidia-cuda-runtime-cu12` + `audioread` | `nvidia-smi` 可用且驱动 ≥ 最低版本 |
| CUDA + torch | 上述，另把 `torch`/`torchvision` 换成 `torch==2.14.1+cu126` + `torchvision==0.29.1+cu126`（SJTU 镜像 → 官方 → 阿里云，`STEMSCRIBE_TORCH_INDEX` 可覆盖） | 装了 `architecture` 为 `vr`/`demucs`/`mdxc`/`roformer` 的模型 |
| CPU | `audio-separator` + `onnxruntime` + `audioread` | 其他情况 |

两个容易踩的点（都实测过）：
- `onnxruntime-gpu` 必须钉 1.21.1。1.24+ 是 CUDA 13 构建，provider DLL 依赖
  `cublasLt64_13.dll`，566 系驱动直接加载失败。
- `audioread` 必须显式列出。`audio-separator` 的 `uvr_lib_v5` 在 import 时就需要它，
  但其包元数据已不再声明（librosa 1.x 去掉了这个依赖），干净 venv 会在 import 阶段
  `ModuleNotFoundError`。
- 普通 PyPI 包按阿里云镜像 → 官方索引的顺序尝试（`STEMSCRIBE_PYPI_INDEX` 可覆盖，与
  `STEMSCRIBE_TORCH_INDEX` 同理）。实测（2026-11-27，本机）：`pypi.org` 索引 0.8 s 响应，
  但 `files.pythonhosted.org` 下发 203 MB 的 `onnxruntime-gpu` 只有约 40 KB/s（单个 wheel
  要 85 分钟）；阿里云镜像同一文件约 3 MB/s。每个索引的失败原因都会写进安装日志，不静默跳过。

两种 ONNX Runtime 发行包互斥，安装前会卸载另一支。装完用 **Identity 小模型真跑一次
`CUDAExecutionProvider` 会话**，只有 session 实际激活 CUDA 才写 `accel=cuda12`；
否则写 `accel=cpu` + `accel_reason`，并在模型中心与能力页显性标注——CUDA 依赖
（cublas/cufft/cudnn）由 nvidia-* wheel 自带，不要求用户预装 CUDA toolkit。

torch 档同样要自证：`INSTALLED.json` 记录 `torch_backend`（`cuda`/`cpu`/`absent`）与
`torch_version`，探针用真实 CUDA 张量运算（`torch.zeros(1, device="cuda").sum()`）。
**ONNX 会话与 torch 必须分进程探测**：同一个进程先建 ONNX CUDA session 再 import torch，
Windows 上 torch 的 `cudnn_cnn64_9.dll` 会加载失败（实测 `[WinError 127]`），
探针会误报 `torch_backend=absent`。

装完 7.2 GB 的 CUDA+torch 档后，运行期仍按模型 `architecture` 选栈：MDX 走 ONNX
（`engine.log` 记 `runtime=onnx`），VR/Demucs 走 torch（`runtime=torch`），
两者共用同一个 venv，互不降级。
模型本身仍走 registry + hf-mirror + 实测 sha256，不例外。

**必须守住的四条**
1. **sha256 不可跳过**（`--no-verify` 仅在开发者模式且 UI 显红字警告时允许）。
2. **原子落盘**：禁止「直接下载到最终路径」——断电会留下半截模型被误认为可用。
3. **路径白名单**：`dest` 只能是相对路径，禁止 `..`、绝对路径、盘符；解析后必须仍在目标子目录内。
4. **失败可诊断**：错误码细分 `DL_HTTP_403` / `DL_SIZE_MISMATCH` / `DL_SHA_MISMATCH` / `DL_DISK_FULL` / `DL_TIMEOUT`。

## 4. 目录布局（安装后）

```
<workdir>/runtimes/
├── whisper/
│   ├── 1.7.4-cuda12/  whisper-cli.exe  *.dll  VERIFIED.json
│   ├── 1.7.4-cpu/     whisper-cli.exe  VERIFIED.json
│   └── ACTIVE.json                  # {"engine":"whisper","version":"1.7.4","backend":"cuda12"}
├── separator/
│   ├── venv/          Scripts/python.exe  Lib/site-packages/audio_separator/...
│   ├── meta/          download_checks.json  mdx_model_data.json  vr_model_data.json
│   ├── helper.py      # 内嵌脚本，首次运行落盘
│   └── INSTALLED.json # {"version":"0.48.0","accel":"cuda12","providers":["CUDAExecutionProvider","CPUExecutionProvider"],"source_url":"https://pypi.org/project/audio-separator/"}
└── ffmpeg/
    └── 7.1/           ffmpeg.exe  ffprobe.exe  VERIFIED.json

<workdir>/models/
├── whisper/whisper-large-v3/ggml-large-v3.bin
├── whisper/whisper-medium/ggml-medium.bin
└── separator/uvr-mdx-kim-vocal-2/Kim_Vocal_2.onnx

<workdir>/config/
├── calibration.json   # 实测显存档位（自动回写）
├── models.lock.json   # 已装清单 + sha256
└── settings.json
```

`ACTIVE.json` 让「切后端」只是改一个指针，不需要重装。

## 5. 后端变体选择算法

```
if no nvidia gpu                  -> cpu
elif driver < 525                 -> vulkan (若可用) else cpu
elif cuda runtime missing         -> vulkan (若可用) else directml (若可用) else cpu
else                              -> cuda12
```

- NVIDIA：优先 `cuda12`（性能最好），退化 `vulkan`（跨厂商，性能中），再 `directml`（Windows 原生，兼容好），最后 `cpu`。
- AMD/Intel：`vulkan` → `directml` → `cpu`。
- **已安装集合同样是输入**：`probe`、`/api/capabilities` 与任务 meta 共用 `runctl.BackendReport`；
  硬件偏好 cuda12 但只装了 vulkan 时，三处都报告 `vulkan`，reason 写明「cuda12 未安装」。
  只看硬件会让 probe 说 cuda12、实际跑 vulkan，这是 CHG-0016 修掉的谎报。
- 选择结果**必须显示在 UI**（侧栏 `CUDA 12 · RTX 3060` / `VULKAN · RTX 3060`，悬停给原因），且允许手动锁定（锁定后不自动降级，OOM 直接报错——给高级用户确定性）。

### 5.1 CUDA 运行时的构建与分发（CHG-0016）

- CI 的 `build-cuda` 需要 CUDA runner；GitHub 托管 runner 没有工具链，因此本机脚本是主路径：
  `scripts/build-runtime-cuda.ps1`（VS2019/2022 + CUDA 12.4 + CMake）。
- 归档必须自带 `cudart64_12.dll`、`cublas64_12.dll`、`cublasLt64_12.dll`。
  `stage-runtime.ps1` 对三件 DLL 做硬断言，并在打包前执行 `--help` 冒烟：缺 DLL 的 exe
  只会在开发机（工具包在 PATH）上正常，用户的机器会直接起不来。
- **三件缺一不可（实测，不是猜的）**：`dumpbin /dependents` 显示 exe 只导入 `cudart64_12.dll`
  与 `cublas64_12.dll`，但后者自身导入 `cublasLt64_12.dll`；只放两件的干净目录里启动
  `whisper-cli.exe` 直接 `0xC0000135`（DLL 未找到）。所以 472 MB 的 `cublasLt64_12.dll`
  不能省。
- 体积实测（1.9.5，archs 52/61/75/86，`-Xfatbin=-compress-all`）：exe 303 MiB + CUDA DLL 547 MiB
  → zip **574 MiB**。fatbin 压缩只把 exe 从 539 MiB 压到 303 MiB，zip 几乎不变（Deflate 对已压缩
  cubin 无能为力），它的价值在解压后占盘更小。
- 只有归档成功上传后才能把 registry 的 `size` / `sha256` 写成实测值并删除 `tbd`；
  上传失败就保持 `tbd:true`，UI 明示「CUDA 运行时未分发，当前用 Vulkan（也是 GPU）」。
  **禁止**用估算值填空。
- 构建失败的回退路线：上游官方 1.8.2 `whisper-cublas-12.4.0-bin-x64.zip`（实测可达），
  作为独立版本条目登记，并在 CHG 中记录版本不一致这一事实。

## 6. 许可与合规（开源发布的硬约束）

| 项 | 规则 |
|---|---|
| 不镜像第三方权重 | registry 只存官方直链 + `license` 字段；不把权重塞进 GitHub Release |
| whisper.cpp 二进制 | MIT，允许再分发；release 里可附带，但需附 LICENSE 与来源说明 |
| UVR 权重 | 各模型许可不同（部分仅限非商用）；UI 在安装前展示 `license.spdx` 与链接，用户自行确认 |
| Python embed + audio-separator | audio-separator 为 MIT；其依赖（onnxruntime 等）许可需在 `THIRD-PARTY-NOTICES.md` 列出 |
| 本项目自身 | MIT（见 `LICENSE`） |

> 原则：**分发代码与分发权重是两件事**。本项目只分发代码与"去哪下、怎么校验"的元数据。

## 7. 体积预算（PRD N-02 的可行性核算）

| 项 | 是否随包 | 体积 |
|---|---|---|
| Stemscribe 主程序（Wails + 前端静态产物） | ✅ | ~22 MB |
| ffmpeg（静态精简版） | ❌ 运行时下载 | ~68 MB |
| whisper.cpp cuda12 二进制 | ❌ | ~574 MB（exe 303 MiB + CUDA DLL 547 MiB，实测） |
| Python venv + audio-separator + onnxruntime(-gpu) | ❌ | CPU **1.0 GB** / CUDA **3.7 GB**（实测 `du -sm`，含 nvidia cu12 轮子） |
| 同上 + CUDA torch 档（`torch==2.14.1+cu126` + `torchvision`） | ❌ | **7,243 MB**（实测 `du -sm`，2026-11-26） |
| whisper medium-q5_0 权重 | ❌ | ~540 MB |
| MDX-Net ONNX 权重 | ❌ | 29–67 MB（实测：Kim 64 MB、Karaoke 29 MB） |
| Demucs 4 轨权重（htdemucs / htdemucs_ft） | ❌ | 81 MB / 322 MB（实测，含 YAML 与 1–4 个 `.th`） |
| Demucs 6 轨权重（htdemucs_6s） | ❌ | 53 MB |
| **portable zip 合计** | — | **目标 ≤ 40 MB**（仅主程序 + 图标 + 文档） |

**首次安装最小可用集合**（均衡档，GPU 走 CUDA）：ffmpeg 68 MB + whisper cuda 574 MB + large-v3 2.9 GB + 一个 MDX 模型 64 MB ≈ **3.6 GB**；GPU 分离运行时另加 **3.7 GB**（实测；CPU 变体 1.0 GB）。装上任意 torch 架构模型（VR/Demucs）后，分离运行时再涨到 **7.2 GB**。
选 cpu/vulkan 运行时则 whisper 部分降到 1 MB / 15 MB，这是首次安装体积与速度的取舍。

> 若最终实测超预算，优先砍的是「Python 分离运行时」——改为纯 ONNX Runtime 的 C++ 分离器（打包 ~25 MB）。这是 ADR-002 的备选路径，登记为 CHG-0002 的决策点。
