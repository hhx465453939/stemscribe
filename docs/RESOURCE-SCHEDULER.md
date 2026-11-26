# 资源调度：显存 / 内存 / IO

> 对应 ADR-004，用户明确要求「CUDA 调用、CPU 内存缓存」必须参考现有工具的成熟做法。
> 本文给**可实现的具体规则**，不是原则性描述。经验值标注 `校准前`，实测后回填 `config/calibration.json`。

## 1. 测量层

| 数据 | 来源 | 频率 | 失败回退 |
|---|---|---|---|
| 显存 total/free/used | NVML（`nvml.dll`，`nvmlDeviceGetMemoryInfo`） | 运行中 1 Hz；决策前即时一次 | 解析 `nvidia-smi --query-gpu=memory.total,memory.free --format=csv,noheader,nounits`（1 次/决策，不轮询） |
| GPU 型号 / 驱动 / CUDA | NVML + `nvmlSystemGetDriverVersion` | 启动一次 | `nvidia-smi` |
| 计算能力（sm_xx） | NVML `nvmlDeviceGetCudaComputeCapability` | 启动一次 | 未知 → 按 sm_75 保守处理 |
| 系统内存 | `GlobalMemoryStatusEx`（Win32） | 5 s | — |
| 磁盘剩余 | `GetDiskFreeSpaceExW` | 决策前 | — |
| GPU 利用率 | NVML `nvmlDeviceGetUtilizationRates` | 运行中 1 Hz | 隐藏该指标 |

**不做的事**：不用 WMI 查显存（`Win32_VideoController.AdapterRAM` 是 32 位字段，12 GB 显卡会显示 4 GB，本机实测已确认此坑：`AdapterRAM=4293918720` 而 NVML 报 12288 MiB）。

## 2. 显存预算

```
budget_mb = free_mb - reserve_mb
reserve_mb = max(1536, total_mb * 0.12)        # 12 GB 卡 → 1536 MB
reserve 用途：WebView2/合成器占用、CUDA context (~300 MB)、碎片、驱动内部
```

**准入判定**（启动任何引擎前）：

```
required = model.vram_estimate_mb
         + runtime.vram_overhead_mb            # whisper cuda 约 320 MB
         + segment_factor_mb                   # 分段越长中间激活越大，见 §4
if required <= budget_mb        -> 直接启动
elif 可驱逐且驱逐后满足          -> 驱逐 LRU 常驻模型后启动
else                            -> 降档模型（见 §5）；仍不满足则 CPU
```

**校准机制**（关键：经验值必然不准）
- 每次任务结束记录引擎自报峰值（whisper.cpp 无此输出 → 用 NVML 1 Hz 采样的 max）写入 `config/calibration.json`：
  ```jsonc
  { "whisper/ggml-medium-q5_0@cuda12": { "peak_mb": 4120, "samples": 7, "updated_at": "..." } }
  ```
- 后续准入用 `max(registry_estimate, calibrated_p50)`，取保守值。
- 用户可在设置里「重算显存基准」（清空校准重新采样）。

## 3. 模型常驻与驱逐（LRU）

```
缓存项 = {engine, backend, model_id, loaded_at, last_used_at, vram_mb, pid}

on_need(required):
  if resident(model)                 -> touch(last_used_at); return hit
  if budget 足够                     -> load; return
  候选 = 可驱逐项（非当前任务持有）
  sort by (last_used_at ASC, vram_mb DESC)      # 先淘汰最久未用，同序淘汰更占地的
  for c in 候选:
       evict(c)                                   # 向引擎发 unload 或 terminate
       wait_until(free_mb >= required, timeout=3s)
       if 满足: load; return
  降档（§5）
```

- **单任务持有锁**：正在跑任务的模型不可驱逐（否则任务崩）。
- **跨阶段复用**：同一任务内 `separate` → `transcribe` 是两个引擎，分离引擎在转写开始后可被驱逐（默认驱逐，因为多半不再用；批量场景则保留以复用，由队列上下文决定）。
- **显式预加载**：批量队列首条任务开始前，按队列统计预加载高频模型，减少反复 load（load 一次 `medium` 约 1–2 s，频繁切换会累积）。

## 4. 分段参数 ↔ 显存对照（校准前经验值）

| 引擎 | 参数 | 关系 | 默认 | 显存紧张时 |
|---|---|---|---|---|
| whisper.cpp | `-ngl`（GPU 层数） | 层数越多显存越多、越快；`-ngl 99` = 全量上卡 | 99（若预算允许）else 按 §5 降 | 逐级降：99 → 24 → 12 → 0（0=纯 CPU） |
| whisper.cpp | `-bs`/`--beam-size` | 影响速度与显存（解码缓存） | 5 | 3 → 1（贪心） |
| whisper.cpp | VAD 切片 | 单段越短显存越省 | 开，段 ≤ 30 s | 段 ≤ 15 s |
| whisper.cpp | 模型量化 | q5_0 比 f16 省 ~45% 显存 | medium-q5_0 | small-q5_0 → base |
| separator (MDX/ONNX) | `segment_size`（秒） | **显存近似随段长线性** | 15 s | 12 → 8 → 4 |
| separator | `batch_size`（多段并行） | 线性放大 | 1 | 固定 1 |
| separator | 精度 | fp16 省一半 | fp16（CUDA） | int8（ONNX 量化） |
| separator | `overlap`（交叉淡化） | 影响质量不影响显存 | 2 s | 保持 2 s（质量底线） |

> 这张表就是 UVR 里 `segment_size` / `batch_size` / `use_half_precision` 三个滑杆的等价物，只是由预算器自动决策并允许高级用户覆盖。

## 5. 降级链（每级都必须可解释）

```
cuda12 → vulkan → cpu
```

> **`directml` 不在链上（ADR-018）。** whisper.cpp 没有 DirectML 后端，列进降级链等于向用户承诺一条并不存在的路。AMD/Intel 由 Vulkan 承接（自包含、不依赖厂商运行时），NVIDIA 走 CUDA 取上限，CPU 永远可跑。任何一级下降都必须带原因串并写进事件日志（ADR-004）。

**降级触发条件**
| 触发 | 动作 | 记录 |
|---|---|---|
| CUDA 初始化失败（驱动/运行时缺失/版本不符） | 落 vulkan | `HW_CUDA_UNAVAILABLE` + 原始错误串 |
| 无 NVIDIA 但检测到 AMD/Intel | 直接选 vulkan（不试 cuda） | `HW_VENDOR_AMD` / `HW_VENDOR_INTEL` |
| 无可用 GPU，或 vulkan loader 缺失 | 落 cpu | `HW_NO_GPU` / `HW_VULKAN_UNAVAILABLE` |
| CUDA OOM（即使降到最小模型仍 OOM） | 落下一后端 | `ENGINE_OOM` + `required/free` 数值 |
| 引擎崩溃 2 次 | 落下一后端 | `ENGINE_CRASH` |
| 用户手动锁定后端 | 不降级，直接报错 | 尊重用户（高级模式） |

**模型降档顺序（同一后端内先降模型，再降后端）**
```
whisper:  large-v3 → medium → small → base → tiny
量化:     f16 → q5_0（体积/显存小，质量损失可接受）
separator: 大 MDX 模型 → 小 MDX 模型（仍失败则报错；**不存在跳过分离**，ADR-037）
```

> 顺序原则：**先牺牲精度，再牺牲速度，最后牺牲功能**。分离不再是"可牺牲功能"：它是链路的前置阶段，
> 缺依赖就失败并给出安装命令（ADR-037）。
>
> 分离实测（2026-11-26，E5-2696 v3 18C/36T，CPU onnxruntime）：`Kim Vocal 2` 30 s 音频
> 峰值 RSS **3022 MB**、**~15 s**（RTF ≈0.50）；30 min 整集峰值 RSS **≈5.1 GB**、
> **18 min 58 s**（RTF ≈0.63）。
> 装 CUDA 运行时后同一 30 min 素材 **3 min 51 s**（231 s，RTF ≈0.13，4.9×），显存峰值
> **4617 MiB**（空闲基线 2372 MiB，即模型+工作区约 **2.3 GB**）；CPU 版仍不占显存。
> `vram_estimate_mb` 记 2300。
> onnxruntime 按核数开线程，限制并发时要同时限制它的线程数，否则 5 GB 级 RSS 会挤占转写。

## 6. 内存（RAM）策略

| 场景 | 策略 | 理由 |
|---|---|---|
| 大 WAV 读写 | **流式分块**（512 KB 缓冲），禁止 `os.ReadFile` 整个 500 MB 音频 | Go 中一次读入 30 min 44.1k stereo float32 ≈ 635 MB，叠加引擎即爆内存 |
| 音频切片 | `io.SectionReader` + 临时文件，不复制整段到内存 | — |
| 波形峰值图（UI） | 预计算 min/max 峰值存 `<workdir>/cache/peaks/<video_id>.bin`（每 100 ms 一个 bin，int8 量化） | 避免 UI 反复解码整个音频；典型 30 min 音频峰值文件 ~70 KB |
| JSON 产物 | 流式 `json.Encoder` 逐段写入 `transcript.partial.jsonl`，阶段末合并为 `transcript.json` | 崩溃可续，且不会因单次 marshaling 大对象卡顿 |
| SQLite | WAL 模式 + `synchronous=NORMAL`；FTS5 索引单独表 | 并发读（MCP server）与写（pipeline）不互斥 |
| 引擎进程 | 分离引擎常驻时限制 Python 堆（`PYTHONMALLOC`、`--max-old-space` 不适用；用进程 RSS 上限监控 + 超限重启） | 防长批量内存泄漏 |
| 页缓存 | 不主动 `drop_caches`（Windows 无此语义）；改为**顺序读写 + `FILE_FLAG_SEQUENTIAL_SCAN`** 提示 | 让系统页缓存为下次读服务 |

**明确禁止**
- ❌ 把整集音频 base64 塞进 JSON 传引擎（体积膨胀 33% 且全量驻留）
- ❌ 用 `bytes.Buffer` 累积整段转写结果再落盘
- ❌ 在 UI 主线程解码音频做波形（必须后端预计算）

## 7. IO 与并发

| 项 | 规则 |
|---|---|
| 磁盘 IO | 下载与推理**不写入同一磁盘卷**时并行；同卷时限速下载（默认 8 MB/s）以免拖慢模型读取 |
| 临时文件 | 全部落 `<workdir>/tmp/`，启动时清理 >24 h 的残留；原子写统一 `.tmp` 后缀 |
| 并发上限 | 全局 `max_concurrent = clamp(floor(budget / peak_per_task), 1, 2)`；显式硬上限 2 |
| 引擎并发 | 同一 GPU 上 whisper 与 separator **不并发**（争显存 + 争 PCIe），串行调度 |
| 事件节流 | 进度事件 5 Hz 上限；UI 侧再合并（同一 stage 只保留最新） |

## 8. 参考工具的做法对照（本机实测 + 公开实现）

| 工具 | 关键做法 | 本项目的取舍 |
|---|---|---|
| **UVR**（`F:\software\Ultimate Vocal Remover`，内含 `torch` + `onnxruntime` + `models/{MDX_Net,VR,Demucs}`） | 分 `segment_size` 控制显存；`batch_size` 提吞吐；`use_half_precision` 省显存；每段后释放中间张量 | 采纳分段 + 半精度 + batch=1；把三滑杆换成自动预算决策，保留高级覆盖 |
| **Buzz**（`E:\software\Buzz\_internal`：`faster_whisper-1.0.3` + `ctranslate2` + `torch-2.2.1+cu121` + `onnxruntime-1.17.1`） | 统一转 16 kHz mono；`compute_type`（float16/int8）按显存选；VAD 过滤静音；任务队列串行 | 采纳 16k mono + VAD + 队列串行；用 whisper.cpp 替代 CTranslate2（少一个 Python 依赖） |
| **WhisperDesktop**（Const-me/Whisper：`ggml-large-v3.bin` 3.1 GB + `Whisper.dll` + `lz4.txt`） | DirectCompute 后端，权重**压缩后放显存/内存**（lz4）以降低占用；模型常驻；层数可调 | 采纳「模型常驻 + 层数可调」；**压缩权重暂不采纳**（whisper.cpp 无此机制，且换来的 CPU 解压开销在 12 GB 卡上无必要），登记为 CHG-0002 的观测项 |
| **bilibili-video-downloader**（Tauri 客户端 + 自带 ffmpeg） | 自带 ffmpeg 做合并；分片下载 | 采纳自带 ffmpeg 运行时 + 分片续传 |

> 诚实标注：以上为**本机安装产物实测证据**（见 `TRACEABILITY.md` §3）。源码级实现细节（例如 UVR 具体在哪个函数调 `empty_cache`）尚未逐行核对，属 CHG-0002 的调研任务，不在本轮假装已读过源码。

## 9. 待实测回填（禁止编造）

| 项 | 状态 |
|---|---|
| `ggml-large-v3` 在 3060 12G 上 `-ngl 99` 峰值显存 | TBD — CHG-0002 |
| `ggml-medium-q5_0` 峰值显存与 RTF | TBD — CHG-0002 |
| MDX-Net ONNX 不同 `segment_size` 的峰值显存曲线 | TBD — CHG-0002 |
| CUDA→Vulkan 降级后 RTF 劣化幅度 | TBD — CHG-0002 |
| 峰值缓存（peaks）实际体积 | TBD — CHG-0003 |
