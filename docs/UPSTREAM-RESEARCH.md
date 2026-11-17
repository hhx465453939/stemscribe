# 上游工具机制调研（对照与复用）

> 主人在需求里点名「CUDA 调用、CPU/内存缓存等请参考这几个工具的源项目」。
> 本文分两层写：**已实测确认的环境事实**（有证据）与**待源码级复核的项**（不假装已读）。
> 证据来源与日期见 `tasks/CHG-0001/TRACEABILITY.md` §3。

## 1. 四个参照对象

| 工具 | 类型/技术栈 | 本项目借鉴什么 |
|---|---|---|
| **bilibili-video-downloader** | Tauri 2 桌面客户端（`com.lanyeeee.*` bundle id）+ 自带 ffmpeg | 桌面客户端分发形态、自带 ffmpeg 运行时、分片下载 |
| **Ultimate Vocal Remover (UVR)** | Python + PyTorch/Tkinter，模型库 MDX-Net / VR / Demucs | 分离模型的**分段 + 半精度 + 批大小**三参数模型，显存控制思路 |
| **Buzz** | Python + wxPython，`faster-whisper` + CTranslate2 + torch + onnxruntime | 任务队列串行、统一 16 kHz mono、`compute_type` 按显存选精度、VAD 过滤 |
| **WhisperDesktop (Const-me/Whisper)** | C++ + DirectCompute + ggml | 单文件分发、模型常驻、GPU 层数可调、**压缩权重缓存**（lz4） |

## 2. 实测环境事实（有证据，可直接依据）

| 事实 | 证据 |
|---|---|
| Buzz 的引擎栈 = `faster_whisper-1.0.3` + `ctranslate2` + `torch-2.2.1+cu121` + `onnxruntime-1.17.1` + `whisper.dll` | `ls E:\software\Buzz\_internal` |
| UVR 的引擎栈 = `torch` + `onnxruntime` + `models/{MDX_Net_Models,VR_Models,Demucs_Models}` + `lib_v5` + `ffmpeg.exe` | `ls F:\software\Ultimate Vocal Remover` |
| WhisperDesktop 权重 `ggml-large-v3.bin` = 3,095,033,483 B，且目录内存在 `lz4.txt` | `ls -la F:\software\WhisperDesktop` |
| WhisperDesktop 仅 `Whisper.dll`(576 KB) + 主 exe(332 KB) —— **整个引擎不到 1 MB** | 同上 |
| bilibili-video-downloader 自带 `com.lanyeeee.bilibili-video-downloader-ffmpeg.exe` | `ls E:\software\bilibili-video-downloader` |
| 本机 GPU = RTX 3060 12288 MiB，驱动 566.36，CUDA 12.7 | `nvidia-smi` |
| **WMI `AdapterRAM` 对本机 12 GB 卡报 4,293,918,720 B（≈4 GB，32 位字段截断）** | `Get-CimInstance Win32_VideoController` |

> 最后一条是本项目**必须避开的坑**：显存探测绝不能走 WMI，必须走 NVML（见 `RESOURCE-SCHEDULER.md` §1）。

## 3. 机制对照表（借鉴 → 本项目决策）

### 3.1 显存控制

| 工具做法 | 本项目 | 差异理由 |
|---|---|---|
| UVR：`segment_size` 滑杆控制显存（段越长越吃显存） | 采纳其模型，但**由预算器自动决策**（15/12/8/4 s），高级模式可覆盖 | 用户不该为 OOM 负责 |
| UVR：`batch_size` 提吞吐（同时处理多段） | **固定 1** | 12 GB 单卡串行更快（避免争用），且批处理会让进度粒度变粗 |
| UVR：`use_half_precision`（fp16） | CUDA 默认 fp16；ONNX 路径用 int8 量化模型 | 省显存换极小质量损失，值得 |
| Buzz：`compute_type` 按显存选 `float16/int8` | 采纳为「量化档位自动选择」 | 与 UVR 半精度同源思路 |
| WhisperDesktop：DirectCompute + 权重 **lz4 压缩放显存** | **暂不采纳**，登记为观测项 | whisper.cpp 无此机制；12 GB 卡上压缩换来的 CPU 解压开销无必要。若 8 GB 卡用户反馈 OOM，再评估 |

### 3.2 分段与流式

| 工具做法 | 本项目 |
|---|---|
| UVR：分段 + 段间交叉淡化拼接 | 采纳：15 s + 2 s 重叠 + 线性交叉淡化 + 拼接后响度归一化（-14 LUFS） |
| WhisperDesktop/Buzz：一次性喂整段音频给 Whisper | **改为 VAD 切片流式**：段级结果即时落盘，支持中断续跑（它们做不到） |
| Buzz：统一转成 16 kHz mono 再转写 | 采纳，且额外保留 44.1 kHz stereo 供分离使用（Buzz 不做分离，故只需一份） |

### 3.3 模型管理与分发

| 工具做法 | 本项目 |
|---|---|
| UVR：模型散落在安装目录 `models/`，随安装包分发 | **改为按需下载 + sha256 校验 + 原子落盘**（UVR 的 5 GB 安装包含大量用户永远不用的模型） |
| WhisperDesktop：单个 `.bin` 丢在 exe 旁边 | 采纳「文件即模型」的简单性，但加 registry 元数据与校验 |
| Buzz：首次运行下载模型（已具备该能力） | 采纳，并补上断点续传、镜像源、许可展示 |
| bilibili-downloader：自带 ffmpeg | 采纳（ffmpeg 作为 runtime 类条目下载，不随包） |

### 3.4 内存与 IO

| 工具做法 | 本项目 |
|---|---|
| Buzz：任务队列串行执行 | 采纳；并加显存预算准入（它没有预算概念，用户只能事后看 OOM） |
| UVR：Python 进程内长驻，反复推理不重启 | 采纳「模型常驻 + LRU 驱逐」；但分离引擎崩溃可重启（进程隔离，优于 UVR 的单进程模型） |
| 各家：整段 WAV 读入内存 | **改为流式分块**（512 KB），因为本项目要处理批量长音频 |
| — | 新增：波形峰值预计算缓存（`cache/peaks/*.bin`，int8 量化），避免 UI 反复解码 |

## 4. 明确不照抄的地方

| 不抄 | 原因 |
|---|---|
| UVR 的 Tkinter 单机 GUI 架构 | 与本项目 WebView2 UI 不可比；且其参数面板把 20+ 滑杆平铺，违背「默认可用」 |
| Buzz 的 wxPython 多引擎并列（Whisper/Whisper.cpp/Faster-Whisper） | 三套引擎维护成本高；本项目单一引擎 + 后端变体即可覆盖 |
| WhisperDesktop 的 DirectCompute-only | 绑定 Windows + DX 路径，无法复用 CUDA 生态；本项目走 whisper.cpp 多后端 |
| UVR 把全部模型打进安装包 | 体积不可持续，且分发第三方权重有许可风险 |
| 任何工具"用户自己调显存参数"的思路 | 违背 PRD 非功能要求（默认可用） |

## 5. 待源码级复核清单（CHG-0002 任务，不在本轮假装完成）

| # | 要复核的事 | 目标文件（上游） | 验收 |
|---|---|---|---|
| R-1 | UVR 具体在哪释放中间张量、`segment_size` 与显存的实测曲线 | `ultimatevocalremovergui` 的 `separate.py` / `vr_arch` | 产出对照表，写入 `RESOURCE-SCHEDULER.md` §4 |
| R-2 | Buzz 的 `compute_type` 选择逻辑与 VAD 参数默认值 | `buzz/transcriber/*.py` | 确认本项目默认值是否需要调整 |
| R-3 | Const-me/Whisper 的 lz4 压缩权重机制（压缩率、解压时机、显存收益） | `Whisper/WhisperModel*.cpp` | 结论：是否值得为 8 GB 卡引入 |
| R-4 | whisper.cpp 的 `--processors` / `-ngl` / VAD 在实际长音频上的最优组合 | `ggml-org/whisper.cpp` `examples/cli` | 产出 3 档预设的参数表 |
| R-5 | audio-separator 的 ONNX 后端在 CUDA/DirectML 上的实际可用性与坑 | `nomadkaraoke/python-audio-separator` | 决定「Python 分离」是否降级为「纯 ONNX C++ 分离」 |
| R-6 | B 站下载接口的稳定性与备选（yt-dlp 兼容层） | — | 决定 `internal/pipeline/source` 的双引擎策略 |

> 复核方式：`git clone --depth 1` 到 `E:\Development\_ref\`（不进入本仓库），按上表逐项产出结论并回填本文 §3/§5。

## 6. 对主需求的三条直接回应

| 主人的原话 | 本项目的落地 |
|---|---|
| 「这块对于 cuda 调用……烦请参考这几个工具的源项目」 | CUDA 路径：`whisper.cpp` cuda12 变体 + NVML 显存预算 + 四级降级链（`RESOURCE-SCHEDULER.md` §2/§5）；参照 UVR/Buzz 的半精度与量化选择 |
| 「cpu 内存的缓存调用」 | 三层：① 模型常驻 LRU（避免反复 load）② 波形峰值与 `calibration.json` 持久缓存 ③ 大音频流式分块读写 + 顺序扫描提示（`RESOURCE-SCHEDULER.md` §3/§6） |
| 「从模型下载到安装的项目工作目录」 | `workdir/{runtimes,models,libraries,cache,config,logs,tmp}` 自包含，整目录可迁移（`MODEL-REGISTRY.md` §4） |
