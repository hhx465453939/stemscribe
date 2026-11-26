# CHG-0016 · TRACEABILITY

## 1. 需求 → AC → 验证

| 需求 | 来源 | AC | 验证 | 落点 |
|---|---|---|---|---|
| CUDA 真的被用上 | R1 | AC-01 | 引擎日志 `using CUDA0` + meta backend + 对照测速 | `scripts/build-runtime-cuda.ps1`、`.github/scripts/stage-runtime.ps1`、registry |
| CUDA 可安装 | R1 | AC-02 | 干净 workdir 真装 + DLL 清单 + sha256 | registry、`internal/runctl/install.go` |
| 后端口径一致 | R2 | AC-03 | probe / `/api/capabilities` / meta 三处比对 | `internal/runctl/{helpers,capabilities}.go`、`scripts/stage-runtime.ps1` |
| 拖入→暂存→开始 | R3 | AC-04 | 截图 + 队列 JSON | `frontend/app/components/NewTask.tsx`、`frontend/lib/staging.ts`、`cmd/stemscribe-gui/main_gui.go` |
| 输出目录选择/打开/校验 | R4 | AC-05 | PUT 400/200 + 产物路径 | `frontend/app/components/NewTask.tsx`、`internal/runctl/outputdir.go`、`internal/webapi/handler.go` |
| 停止任务 | R5 | AC-06 | 进程表 + 状态 + 续跑日志 | `internal/runctl/{service,queue}.go`、`internal/engine/proc_*.go`、`frontend/.../NewTask.tsx` |
| 资料库打开目录 | R6 | AC-07 | 截图 + API `dir` | `frontend/app/library/page.tsx`、`internal/runctl/library.go` |
| 模型中心查看文件 | R7 | AC-08 | 截图 + `path` 存在性 | `frontend/app/models/page.tsx`、`internal/runctl/catalog.go` |
| 队列行阶段/耗时 | R8 | AC-09 | 截图 + 队列 JSON | `frontend/.../NewTask.tsx`、`internal/runctl/service.go` |
| 文案诚实 | R2/R8 | AC-10 | grep + capabilities JSON | `internal/runctl/capabilities.go` |
| 质量门 | — | AC-11 | Checkfix 全量输出 | 全仓 |

## 2. 状态

| AC | 状态 | 证据 |
|---|---|---|
| AC-01 CUDA 真用上 | ✅ | 20 s 片段：`ggml_cuda_init: found 1 CUDA devices`、`using CUDA0 backend`、meta `transcribe.backend=cuda12`。120 s 对照：CUDA 21.77 s vs Vulkan 27.25 s（compute 1.37×，RTF 0.117 vs 0.160）——`tmp/evidence/chg0016/{m1-bench-summary.txt,ac01-cuda-vs-vulkan.json}` |
| AC-02 CUDA 可安装 | ✅ | 干净 workdir `runtime install whispercpp-cuda12-win-x64` 退出 0：从 release 下载 573 MB（中途被 HTTP/2 `PROTOCOL_ERROR` 打断在 50%，重启后从 .part 续传到 100%），解压出 `whisper-cli.exe` + 三件 CUDA DLL + LICENSE + PROVENANCE，写 `VERIFIED.json`（sha256 `f7705367…92de7a`），装后 `whisper-cli --help` 退出 0，目录 849.8 MB——完整输出见 `tmp/evidence/chg0016/ac02-install.txt` |
| AC-03 口径一致 | ✅ | probe / capabilities / meta 三处均 `cuda12` + 同一 reason；仅装 cpu+vulkan 时三处同时 `vulkan` 且 reason 说明原因——`tmp/evidence/chg0016/ac03-backend-consistency.txt`；`internal/runctl/backend_test.go` |
| AC-04 拖入→暂存→开始 | ✅（拖放为人工步骤） | 首页截图（拖入区+浏览+空暂存+开始按钮）；拖入只调 `StatFiles`，不发网络请求（代码 + `staging.ts` 注释），`GET /api/queue` 在点开始前为 0——`tmp/evidence/chg0016/ui/home.png` |
| AC-05 输出目录 | ✅ | 空值 200；可建目录 200 且探针文件写入后删除；不可能路径 400（错误文案单前缀）；队列同样 400——`tmp/evidence/chg0016/ac05-output-dir.txt` |
| AC-06 停止 | ✅ | 分离阶段停止 → 202 stopping → `canceled`，无 audio-separator 子进程残留；再跑一次 `separate: not completed` 且 resolve/download/probe/demux `already valid` 续跑完成——`tmp/evidence/chg0016/{m4-stop-live.txt,ac06-resume.log}`；`proc_windows_test.go` 断言孙进程 ≤5 s 被杀 |
| AC-07 资料库 | ✅ | 每行「打开目录」，`dir` 来自 API 且指向 bundle 实目录；截图 `ui/library.png` |
| AC-08 模型中心 | ✅ | 已安装行显示版本/体积/安装路径，「打开目录」按钮，路径悬停显示全路径；截图 `ui/models.png` |
| AC-09 任务行 | ✅ | 运行中显示 `阶段 · 百分比 · m:ss`；动作随状态出现（停止/移除/打开目录）——`ui/home.png` 队列区 + `service_queue_test.go` |
| AC-10 文案诚实 | ✅ | capabilities `separation=true`（`UVR5 via audio-separator 0.48.0 · uvr-mdx-kim-vocal-2`）、`polish=false`；代码中已无 `not implemented yet (CHG-0003)`（仅任务文档作为历史引用保留） |
| AC-11 质量门 | ✅ | 全量 Checkfix 绿：`go build` / `go vet` / `go test ./... -race`（14 包 ok）、`pnpm lint` / `typecheck` / `build`（静态导出 7 页）、`verify-contract.sh`（15 条 AC、schemas 合规）、`i18n-check.sh`（173 键双语对齐、无硬编码中文）、`contrast.py`（12 对全部 ≥4.5:1）、`redact-scan.sh` 0 命中、`secrets-scan.ps1` 0 命中——完整输出 `tmp/evidence/chg0016/ac11-checkfix.txt` |

## 3. 实测数据（禁止编造）

| 项 | 值 | 来源 |
|---|---|---|
| GPU | RTX 3060 / 12288 MiB / driver 566.36 | nvidia-smi |
| whisper.cpp | v1.9.5（commit d1be6fde11ac6e0407606b4e42fe72d34add8037） | PROVENANCE.txt |
| CUDA 归档 | 601,795,069 B / sha256 `f770536717df04b7c17cfbc7bc8590d9262365179995ee401043cf9f4492de7a` | `Get-FileHash`（stage 脚本） |
| 归档内容 | whisper-cli.exe 317,546,496 B + cudart64_12.dll 553,984 B + cublas64_12.dll 100,188,672 B + cublasLt64_12.dll 472,802,816 B | 目录清单 |
| 120 s 基准（CUDA） | load 7.78 s / encode 1.08 s（5 runs）/ total 21.77 s | whisper timings |
| 120 s 基准（Vulkan） | load 8.03 s / encode 1.24 s（5 runs）/ total 27.25 s | whisper timings |
| 20 s 片段 transcribe 阶段 | CUDA 11.07 s vs Vulkan 12.91 s（加载占比大） | runs/*.jsonl |
| 停止响应 | 停止后 ≤1 s 进程消失，状态 canceled | tasklist 轮询 |
| 下载中断与续传 | 574 MB 资产在 50% 被 HTTP/2 `PROTOCOL_ERROR` 打断；重启安装从 `.part` 续传到 100%，校验 sha256 通过（见 AC-02） | `tmp/evidence/chg0016/ac02-install.txt` |
| 安装后目录 | 849.8 MB（exe 317.5 MB + 三 DLL 约 574 MB 解压后） | `Get-ChildItem -Recurse \| Measure-Object Length -Sum` |
