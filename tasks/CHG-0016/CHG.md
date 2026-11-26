# CHG-0016 • CHG Log

## 变更摘要

修两件事：**CUDA 必须真的被用上**，以及**桌面工具的基本操作必须齐**（拖入区+浏览、
暂存后点开始、输出目录选择/打开、停止任务、资料库与模型中心打开所在目录）。

本 CHG 由 owner 2026-11-26 端到端试用后的明确指令触发；上一条 CHG-0015 只保证
"跑通"，本条补"用得下去"。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-26 | draft | PRD 草案（含 CUDA 现状实证与 UI 基本盘清单） | agent |
| 2026-11-26 | approved | owner 批准 PRD 范围 R1–R8，CUDA 路线选 C（先自建 1.9.5，失败回退上游 1.8.2） | owner |
| 2026-11-26 | proposed | SPEC v1 完成，待 owner 明确批准（批准前不改代码、不 commit） | agent |
| 2026-11-26 | approved | owner 明确批准 SPEC，进入 goal-driven 执行（M0→M6） | owner |
| 2026-11-26 | in_progress | M0–M5 落地并留证；M1 干净 workdir 安装下载中 | agent |
| 2026-11-26 | in_progress | M1 干净 workdir 安装通过（退出 0、`--help` 0、849.8 MB）；AC-01–AC-11 全绿、全量 Checkfix 绿；待 owner 授权提交 | agent |
| 2026-11-26 | closed | owner 授权后分三个 commit 提交并推送（54ec8b4 / bdd0ff8 / 86a99ba），AC-01–AC-11 全绿 | owner + agent |

## 实现记录（2026-11-26）

**CUDA（M1）**

- 构建：`scripts/build-runtime-cuda.ps1`（VS2019 BuildTools + CUDA 12.4.99 + Ninja/CMake），
  whisper.cpp v1.9.5（commit d1be6fd），archs 52/61/75/86，`-Xfatbin=-compress-all`。
- 产物：`whisper-cli.exe` 303 MiB（fatbin 压缩前 539 MiB）+ `cudart64_12.dll` /
  `cublas64_12.dll` / `cublasLt64_12.dll`；zip 601,795,069 B，sha256 `f7705367…92de7a`。
- 已上传 release `runtime-v1.9.5`（资产大小与本地一致），`SHA256SUMS.txt` 已补该行。
- registry `whispercpp-cuda12-win-x64` 写入实测 size/sha256 并删除 `tbd`。
- 干净 workdir 安装验证：从 release 下载 → `VERIFIED.json` 记录实测 sha256 → `whisper-cli --help`
  退出 0 → 装后目录 849.8 MB（`tmp/evidence/chg0016/ac02-install.txt`）。
- 实测（120 s 素材，large-v3，RTX 3060）：CUDA total 21.77 s vs Vulkan 27.25 s（compute 1.37×）；
  RTF 0.117 vs 0.160。20 s 短片段差距小（模型加载占比大），不能拿它下结论。
- 三件 DLL 缺一不可（实测）：exe 只导入 cudart/cublas，但 `cublas64_12.dll` 自身导入
  `cublasLt64_12.dll`；只放两件的干净目录启动即 `0xC0000135`。

**验证中修掉的真实缺陷**

- `stage-runtime.ps1`：`--help | Select-Object -First 5` 提前掐断管道，exe 句柄未释放，
  `Compress-Archive` 报"文件被另一进程使用"；改为 `Start-Process` 落文件并 `-Wait`。
  同一处 `2>&1` 在 PS 5.1 + `ErrorActionPreference=Stop` 下会把原生 stderr 变成
  NativeCommandError 抛出（whisper-cli 的 usage 正好走 stderr）。
- `Service` 终态没有冻结耗时，取消/完成后 `elapsed_ms` 归零；现在终态写入实际耗时，
  `snapshot()` 只在运行中重算。
- 设置页可写探针的错误文案重复前缀（"cannot use that folder: cannot use that folder:"）。
- **下载器从未实现重试**：PIPELINE §6 写着 DL_NET 退避 1s/4s/12s 重试 3 次，代码里只有
  `Retryable()` 分类、没有任何重试循环。实测 574 MB 资产在 50% 被 GitHub HTTP/2
  `PROTOCOL_ERROR` 打断后，安装直接失败退出。现在 `Fetch` 对有界可重试错误
  （DL_NET / 大小不符 / 5xx / 408 / 429）按 1s/4s/12s 重试，`.part` 保留并续传；
  404/403/sha 不符不重试。新增 `TestFetchRetriesInterruptedTransfer`（截断连接→续传→校验）
  与 `TestFetchDoesNotRetryNotFound`。
- 库页状态列换行（"已停 止"），补 nowrap；空暂存提示与拖入区提示重复，拆成独立文案。

**UI（M2–M5）**

- 首页：拖入区（浏览按钮在内）+ 暂存表（名称/大小/移除/清空）+ 输出目录（选择…/打开）
  + 模型/语言 + 「开始转换（N）」；拖入只暂存路径（ADR-042），无复制、无提前转码。
- 队列行：阶段名 · 百分比 · 耗时；动作矩阵 运行中→停止（stopping 时禁用）/ 等待→移除 /
  已完成→打开目录 / 已停止且无产物→移除。
- 资料库：模型列（model · backend）、每行「打开目录」、已停止状态。
- 模型中心：版本、安装路径（悬停显示全路径）、已安装行「打开目录」。
- 证据截图：`tmp/evidence/chg0016/ui/{home,library,models,params}.png`
  （loopback shell + headless Chrome，原生窗口因 RDP 会话无法截屏）。

## 关键事实（2026-11-26 实测，不是猜测）

1. `nvidia-smi`：RTX 3060 / 566.36 / 12288 MiB（空闲 9867 MiB）——GPU 存在且可用。
2. 实际运行**已经在用 GPU**，只是走 Vulkan：`dist/workdir/logs/engine.log` 有
   `ggml_vulkan: 0 = NVIDIA GeForce RTX 3060 ... matrix cores: KHR_coopmat`、
   `Vulkan0 total size = 3094.36 MB`、`using Vulkan0 backend`。
3. CUDA 没被用上的根因**不是没查 nvidia-smi**，而是 CUDA 运行时从未产出：
   - `internal/registry/data/registry.json` 的 `whispercpp-cuda12-win-x64`
     `size: 1`、`sha256: null`、`tbd: true`（未发布即拒绝安装，ADR-025）；
   - `.github/workflows/runtime.yml` 的 `build-cuda` 是 opt-in 且要求 CUDA runner，
     GitHub 托管 runner 没有 CUDA 工具链，所以从未产出资产；
   - 我们自己的 release `runtime-v1.9.5` 只有 cpu / vulkan 两个 zip（已核对）。
4. 可见性有欺骗性：`probe` 报 "backend: cuda12"（硬件偏好），实际运行是 vulkan；
   两处口径不一致，用户看到 `vulkan` 时无法判断是"降级"还是"坏了"。
5. 能力页仍在说 "vocal separation is not implemented yet (CHG-0003)"，与事实相反
   （分离已上线）——属于同一类"文案撒谎"。
6. 本机可自建 CUDA：VS2019 BuildTools（含 `vcvars64.bat`）+ CUDA 12.4.99 (`nvcc`) 已就位。

## 已决（2026-11-26 owner）

- CUDA 获取路线：**C 先自建 1.9.5，失败再回退上游 1.8.2 cublas 包**。
- PRD 范围 R1–R8：**批准**，按此写 SPEC。

## 决策增补（SPEC 阶段）

### ADR-041 · 停止即取消（canceled ≠ failed）

运行中的任务可停止：取消 context，Windows 进程树 `taskkill /T /F` 清掉
whisper-cli / python / ffmpeg 及其孙进程，任务状态记 `canceled` 而非 `failed`。
已完成的阶段与 checkpoint 保留，再次开始从断点续跑。理由：对用户来说「我按了停止」
不是失败；把它写成 failed 会污染资料库与告警，也会诱导重跑已完成的分离。

### ADR-042 · 拖入暂存（取代 ADR-040）

ADR-040 的「拖入即入队」被推翻：拖入/浏览只把文件加入首页暂存列表，用户点
「开始转换」才入队。理由：多文件场景下，拖入即跑意味着用户来不及检查文件、选项、
输出目录；「开始」是这类工具的常识性许可动作。CHG-0015 的 ADR-040 原文只加
「已被 ADR-042 取代」注记，不改其余内容。
