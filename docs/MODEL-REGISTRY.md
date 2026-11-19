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
      "id": "uvr-mdx-kuielab-b4",
      "family": "separator",
      "engine": "separator",
      "display_name": "MDX-Net · Kim Vocal 2 (UVR)",
      "files": [
        { "url": "models/separators/uvr/Kim_Vocal_2.onnx",
          "size": 63100000, "sha256": null, "dest": "Kim_Vocal_2.onnx" }
      ],
      "vram_estimate_mb": 1500,
      "segment_default_s": 15,
      "license": { "spdx": "MIT", "url": "https://github.com/Anjok07/ultimatevocalremovergui" },
      "source": "https://github.com/Anjok07/ultimatevocalremovergui"
    }
  ]
}
```

### 2.1 字段语义（易误解的才列）

| 字段 | 语义 | 陷阱 |
|---|---|---|
| `mirrors[].priority` | 越小越先试 | 失败才降级到下一镜像，不是并发 |
| `archive.sha256` | **整包**校验 | 与解压后逐文件指纹分开：前者防传输损坏，后者防运行时目录被污染 |
| `vram_estimate_mb` | 稳态显存占用**经验值** | 仅用于装前预检；真实值由首次运行后校准并写 `config/calibration.json` |
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
│   └── 0.28-py311/    python.exe  Lib/site-packages/...  VERIFIED.json
└── ffmpeg/
    └── 7.1/           ffmpeg.exe  ffprobe.exe  VERIFIED.json

<workdir>/models/
├── whisper/whisper-medium-q5_0/ggml-medium-q5_0.bin
└── separators/uvr/Kim_Vocal_2.onnx

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
- 选择结果**必须显示在 UI**（设置 → 硬件 → "当前后端：CUDA 12（自动）"），且允许手动锁定（锁定后不自动降级，OOM 直接报错——给高级用户确定性）。

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
| whisper.cpp cuda12 二进制 | ❌ | ~48 MB |
| Python embed + audio-separator + onnxruntime-gpu | ❌ | ~180–420 MB |
| whisper medium-q5_0 权重 | ❌ | ~540 MB |
| MDX-Net ONNX 权重 | ❌ | ~63 MB |
| **portable zip 合计** | — | **目标 ≤ 40 MB**（仅主程序 + 图标 + 文档） |

**首次安装最小可用集合**（均衡档）：ffmpeg + whisper cuda + medium-q5_0 + 一个 MDX 模型 ≈ **720 MB**（不含 Python 分离运行时；若走 Python 分离路线再加 ~300 MB）。

> 若最终实测超预算，优先砍的是「Python 分离运行时」——改为纯 ONNX Runtime 的 C++ 分离器（打包 ~25 MB）。这是 ADR-002 的备选路径，登记为 CHG-0002 的决策点。
