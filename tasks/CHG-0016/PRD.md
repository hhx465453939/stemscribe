# CHG-0016 · PRD · CUDA 真用上 + 桌面基本盘

> 状态：**已批准（2026-11-26，owner）**，范围 R1–R8。SPEC 见 `SPEC.md`（待 owner 明确批准后方可改代码）。
> 触发：owner 2026-11-26 端到端试用反馈（原文见 `CHG.md` 摘要）。

## 1. 用户问题（一句话）

我拿一台有 RTX 3060 的机器跑这个工具，界面却显示 vulkan、操作上还缺一堆任何桌面
转写工具都该有的按钮；我每次都得自己指出该有什么，这不合格。

## 2. 事实基线（实证，避免修错东西）

| # | 事实 | 证据 |
|---|---|---|
| F1 | GPU 可用：RTX 3060 12 GB，驱动 566.36，空闲 9867 MiB | `nvidia-smi` |
| F2 | 当前转写**已经在 GPU 上**，走 Vulkan0（模型 3094 MB 全在显存） | `dist/workdir/logs/engine.log` |
| F3 | CUDA 运行时未发布：registry 条目 `size:1 / sha256:null / tbd:true`，安装被拒 | `internal/registry/data/registry.json` |
| F4 | CI 的 cuda 构建是 opt-in 且需要 CUDA runner，托管 runner 没有 → 从未产出 | `.github/workflows/runtime.yml` |
| F5 | 我们 release `runtime-v1.9.5` 只有 cpu / vulkan 资产 | `gh release view runtime-v1.9.5` |
| F6 | 本机可自建：VS2019 BuildTools + CUDA 12.4.99 已安装 | `vcvars64.bat`、`nvcc --version` |
| F7 | `probe` 报 cuda12（硬件偏好），运行实际用 vulkan（安装集决定） | `stemscribe probe` vs engine.log |
| F8 | 能力页文案仍称分离未实现 | `internal/runctl/capabilities.go` |
| F9 | 拖入即跑（ADR-040），无暂存、无开始按钮 | `frontend/app/components/NewTask.tsx` |
| F10 | 无停止/取消：`Service.Start` 用 `context.Background`，无 cancel 句柄 | `internal/runctl/service.go` |
| F11 | 输出目录只能手输；无选择器、无打开按钮 | `NewTask.tsx` |
| F12 | 资料库只有只读表格；模型中心没有打开目录入口 | `frontend/app/library/page.tsx`、`frontend/app/models/page.tsx` |

## 3. 目标

1. 有 NVIDIA + CUDA 运行时的机器上，**实际后端 = CUDA**，并且 UI / 日志 / meta 三处
   口径一致；没有 CUDA 时显式说明"在用 Vulkan（也是 GPU）"，不再让人猜。
2. 桌面端补齐一套**合格工具的最低操作集**，并把这份清单固化进 `docs/UI-DESIGN.md`，
   之后任何页面改动都要对照，避免"想起来才加"。
3. 任务可停止、可预期：停止后子进程在数秒内退出，已跑完的阶段不重做。

## 4. 范围

### R1 CUDA 运行时（真产出、真校验、真选择）
- 用仓库既有 `stage-runtime.ps1` 的规则自建 cuda12 运行时（ADR-013：不信任陌生人二进制）；
- 归档内必须自带 `cudart64_12.dll` / `cublas64_12.dll`（用户机器不一定装 CUDA 工具包）；
- `--help` 冒烟 + 安装后 `sha256` 实测写 registry（禁止占位值）；
- 后端选择：CUDA 已装且可用 → 用 CUDA；否则 Vulkan；再否则 CPU。每次降级都可见。

### R2 后端可见性（消除 F7/F8）
- `probe`、`/api/capabilities`、任务 meta 统一报告**实际使用**的后端与理由；
- 侧栏显示 `CUDA 12 · RTX 3060` / `Vulkan · RTX 3060` / `CPU`，附一句原因；
- 能力页文案改正：分离可用；润色未实现（事实如此）。

### R3 拖入区 + 暂存 + 开始（替代"拖入即跑"）
- 首页第一屏是**拖放区**：整块可拖、区内有"浏览文件…"按钮（原生多选）；
- 拖入/选中只**加入待处理列表**（每行：文件名、大小、可移除），不创建任何运行；
- 主按钮「开始转换」：有待处理项才可点；点击后全部入队并按队列串行执行；
- 可选"清空列表"；已开始的项从待处理列表移出。

### R4 输出目录
- 输入框 + 「选择…」（原生目录选择器）+ 「打开」（资源管理器定位）；
- 目录不可写时在点击时拒绝并说明，不等到跑完才报错；
- 留空 = `<workdir>/libraries`（现状保留）。

### R5 停止任务
- 运行中任务行有「停止」；等待中行有「移除」；停止后状态 = `canceled`（不是 failed）；
- 停止含子进程：whisper-cli / python / ffmpeg 在 ≤5 s 内退出（含孙进程）；
- 停止不删除已完成阶段的产物与 checkpoint；再次开始可从断点续跑；
- 队列支持「停止当前」；全局停止不是本 CHG 必需项（可选）。

### R6 资料库
- 每行「打开目录」（打开该集 bundle 目录）；可选「打开 txt」；
- 表格显示模型与耗时（已有 meta 字段的直接展示，不新造数据）。

### R7 模型中心
- 已安装行显示：版本、大小、安装路径；提供「打开目录」；
- 未安装行保留安装按钮与进度；
- 新机器缺项时首页横幅可跳转到模型页（CHG-0015 已有，保留）。

### R8 任务行信息
- 队列行显示当前阶段名（如"分离中"/"转写中"）与已用时间，而不是只有一个百分比。

## 5. 非目标

- GPU 版人声分离（onnxruntime-gpu）——另立 CHG；
- 批量暂停/恢复、队列重排序；
- 自动更新、安装包签名、macOS；
- 逐字稿查看器（CHG-0016 原计划，仍顺延）。

## 6. 桌面工具基本盘（本 CHG 固化为标准清单）

写进 `docs/UI-DESIGN.md` §"桌面工具基本盘"，作为后续页面的验收项：

| 类别 | 必须有 |
|---|---|
| 输入 | 拖放区（整块可拖）+ 区内浏览按钮 + 多选 |
| 提交 | 明确的开始/执行按钮；危险或长任务开始前可确认；进行中显示阶段与进度 |
| 中止 | 运行中可停止；等待中可移除；停止有明确结果状态 |
| 目录 | 输出目录可原生选择、可在资源管理器打开；产物可一键定位 |
| 列表 | 每行有与状态匹配的动作（打开目录 / 停止 / 移除 / 查看文本） |
| 安装 | 模型/运行时能看到装在哪、多大、版本，并可打开目录 |
| 诚实 | 后端、降级、缺失依赖、失败原因必须显性；不许伪造进度或成功 |

## 7. 验收标准

| AC | 判定（能红） |
|---|---|
| AC-01 CUDA 真用上 | 装好 CUDA 运行时后跑 20 s 片段：日志出现 CUDA 设备行、meta `backend=cuda12`；同一片段与 Vulkan 各跑一次，记录耗时对比（不预设谁快） |
| AC-02 CUDA 可安装 | `runtime install whispercpp-cuda12-win-x64` 在干净 workdir 成功；registry 的 size/sha256 为实测值；归档内自带 cudart/cublas DLL；`--help` 冒烟通过 |
| AC-03 后端口径一致 | `probe`、`/api/capabilities`、meta 三处对同一次运行报告同一后端；卸载 CUDA 运行时后三处同时变为 vulkan 且给出理由 |
| AC-04 拖入→暂存→开始 | 拖 3 个文件：待处理列表 3 行、队列 0、`libraries/` 无新目录；点开始后队列 3；每行可移除 |
| AC-05 输出目录 | 选择器回填绝对路径；「打开」在资源管理器定位；运行产物落在该目录；不可写目录被即时拒绝 |
| AC-06 停止 | 分离阶段点停止：≤5 s 内 python/ffmpeg 进程消失、任务状态 canceled、后续阶段不启动；重开任务可从断点继续 |
| AC-07 资料库 | 每行「打开目录」打开对应 bundle；API 返回的目录与实际一致（自动化断言） |
| AC-08 模型中心 | 已安装行「打开目录」打开模型/运行时目录；显示版本与大小 |
| AC-09 任务行 | 运行中显示阶段名与耗时；停止/移除/打开目录按钮随状态出现 |
| AC-10 文案诚实 | 能力页分离=可用；润色=未实现；不存在"未实现"的过期说明 |
| AC-11 质量门 | `go build/vet/test -race`、`pnpm lint/typecheck/build`、`verify-contract`、`i18n-check`、`contrast` 全绿；中英双语键对齐 |

## 8. 风险与对策

| 风险 | 对策 |
|---|---|
| 拉取 whisper.cpp 源码受网络限制（GitHub 单连接约 76 KB/s） | 浅克隆 + 必要时用已缓存镜像；失败则改走路线 B（上游官方 1.8.2 cublas 包，实测可达，457 MB） |
| CUDA 自建产物体积大、无法上传 | 本机优先用；发布到我们 release 若不现实，registry 保持 `tbd`，UI 明示"CUDA 运行时未发布，当前用 Vulkan"；不写假 sha |
| CUDA DLL 缺失导致 exe 起不来 | stage 脚本强制捆绑 cudart/cublas，`--help` 冒烟不过不发包 |
| Windows 下只杀直接子进程，孙进程残留 | 停止时对进程树 `taskkill /T /F`（或 Job Object），AC-06 用进程表断言 |
| 拖入语义从"即跑"改为"暂存+开始" | 更新 ADR-040 与 CHG-0015 里相关描述；回归队列测试 |
| 停止造成 checkpoint 半写 | 停止只发生在阶段边界或由 ctx 触发的子进程退出；已写 checkpoint 原子不变，续跑测试覆盖 |

## 9. 待决策

1. **CUDA 路线**（PRD 推荐 A）：
   - A. 本机自建 whisper.cpp 1.9.5 CUDA（与现有版本一致；分发另议）
   - B. 用上游官方 ggml-org 1.8.2 cublas Windows 包（可立即分发，版本较旧）
   - C. 先 A，构建失败再 B（推荐：A 成功则版本一致；失败有退路）
2. **是否接受"CUDA 运行时暂不发布、仅本机自建可用"**：接受则以本机实测为准，
   其他 NVIDIA 用户继续用 Vulkan（仍是 GPU），并在 UI 明示。

## 10. 里程碑（批准 PRD 后写入 SPEC）

| # | 内容 | 对应 AC |
|---|---|---|
| M1 | CUDA 构建/安装/选择/可见性 + 测速 | AC-01~03 |
| M2 | 拖入区、暂存列表、开始按钮 | AC-04 |
| M3 | 输出目录选择/打开 | AC-05 |
| M4 | 停止任务（后端取消 + API + UI + 进程树） | AC-06 |
| M5 | 资料库/模型中心打开目录、任务行阶段与耗时、文案修正 | AC-07~10 |
| M6 | E2E、Checkfix、文档同步、提交（需 owner 同意） | AC-11 |
