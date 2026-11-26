# CHG-0016 · SPEC · CUDA 真用上 + 桌面基本盘

> 状态：**SPEC v1，待 owner 批准**。批准前不改代码、不 commit、不 push。
> 上游：`PRD.md`（已批准，范围 R1–R8）。owner 决策：CUDA 走「先自建 1.9.5，失败再回退上游 1.8.2 cublas 包」（PRD §9 路线 C）。

## 1. 目标与判定

```
目标 = 有 NVIDIA GPU 的机器上，实际后端 = CUDA 且 probe / 界面 / meta 三处口径一致；
       首页具备「拖入→暂存→开始」、输出目录选择/打开、运行可停止、资料库与模型中心可定位文件；
       这套最低操作集写进 UI-DESIGN，成为后续页面的验收项。
判据 = §6 的 AC 全部能红（有自动化断言或明确的人工步骤 + 证据文件）。
代价 = CUDA 归档约数百 MB（cublas），本机构建耗时以小时计；分发受上传带宽限制；
       拖入语义从「即跑」改为「暂存」，需推翻 ADR-040（记录在 CHG.md）。
```

## 2. 决策

| # | 决策 | 理由 |
|---|---|---|
| D1 | 自建 whisper.cpp **v1.9.5 CUDA**，与现有 cpu/vulkan 同版本；构建失败再走上游 v1.8.2 cublas 包 | 版本一致优先；失败有退路（PRD §9） |
| D2 | 归档**自带** `cudart64_12.dll` / `cublas64_12.dll` / `cublasLt64_12.dll` | 用户机器不一定装 CUDA 工具包；缺 DLL 的 exe 起不来 |
| D3 | registry 的 size/sha256 **只写实测值**；上传失败就保持 `tbd:true`，UI 明示「CUDA 运行时未分发」 | AGENTS.md 铁律 2；不许写假校验值 |
| D4 | 后端选择 = **硬件偏好 ∩ 已安装运行时**；probe / capabilities / meta 共用同一个函数 | 消除「probe 说 cuda12、实际跑 vulkan」 |
| D5 | 停止 = 取消 context + Windows 进程树 `taskkill /T /F`；状态 `canceled`，不是 `failed` | 孙进程（python → ffmpeg）不能残留 |
| D6 | 停止不删除已完成阶段与 checkpoint；再次开始从断点续跑 | 停止是暂停语义，不是销毁 |
| D7 | 拖入只进**客户端暂存列表**；点「开始转换」才 `POST /api/queue`。暂存不落盘 | 改动最小、语义最清楚；刷新丢暂存可接受（文件还没入队） |
| D8 | 暂存列表用 React 状态，不新增后端接口 | 不引入「未入队任务」的第二真相源 |
| D9 | 停止当前项时队列继续跑下一项；等待项用「移除」 | PRD R5；「移除」已有接口 |

**ADR 记录**（写入 `CHG.md`，不改历史 ADR 正文）：
- **ADR-041 · 停止即取消（canceled ≠ failed）**：`Service` 持有每 run 的 cancel 句柄，进程树被杀，checkpoint 保留。
- **ADR-042 · 拖入暂存（推翻 ADR-040 的「拖入即入队」）**：拖入/浏览只进暂存；用户显式点开始。

## 3. 设计

### 3.1 CUDA 运行时（R1）

**新增 `scripts/build-runtime-cuda.ps1`**（本机构建入口，不依赖 CI）：

```text
参数：-WhisperVersion v1.9.5  -SrcDir tmp/build/whisper.cpp  -CudaArch "52;61;75;86"
      -Jobs 8  -ReuseSource
步骤：
1. VS 环境：vswhere 找 VS（要求 VC.Tools.x86.x64）→ vcvars64.bat；
   回退已知路径 C:\Program Files (x86)\Microsoft Visual Studio\2019\BuildTools\VC\Auxiliary\Build\vcvars64.bat；
   都找不到 → 明确报错退出（不猜）。
2. CUDA：$env:CUDA_PATH 或 C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v12.* 中最新的；
   必须存在 bin\nvcc.exe；记录 nvcc 版本。
3. 源码：git clone --depth 1 --branch <tag> --recurse-submodules
   https://github.com/ggml-org/whisper.cpp <SrcDir>；-ReuseSource 时跳过已存在的目录。
4. 配置（在 vcvars 环境内执行）：
   -G Ninja（有 ninja）否则 -G "Visual Studio 16 2019" -A x64
   -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF
   -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_SERVER=OFF
   -DGGML_CUDA=ON -DGGML_VULKAN=OFF
   -DCMAKE_CUDA_ARCHITECTURES=<CudaArch>
   -DCMAKE_CUDA_COMPILER=<cuda>\bin\nvcc.exe
   -DCMAKE_CUDA_FLAGS=-allow-unsupported-compiler
5. 构建目标 whisper-cli（Release，-j Jobs）。
6. 调 .github/scripts/stage-runtime.ps1 -Backend cuda12 -Version <tag> -CudaBinDir <cuda>\bin
7. 输出：dist/whispercpp-<ver>-cuda12-win-x64.zip + dist/cuda12.json（size/sha256 实测）。
```

**`stage-runtime.ps1` 修改**：
- 新增可选参数 `-CudaBinDir`；`cuda12` 时必填且目录存在。
- cuda12 分支：从 `-CudaBinDir` 复制 `cudart64*.dll`、`cublas64*.dll`、`cublasLt64*.dll`（构建树里若有同名文件以 CUDA bin 为准）。
- **硬断言**三个文件都在 stage 目录：`cudart64_12.dll`、`cublas64_12.dll`、`cublasLt64_12.dll`；缺一即失败，不打包。
- `PROVENANCE.txt` 的 commit：`$env:GITHUB_SHA` → `git rev-parse HEAD` → `local`。
- 保留 `--help` 冒烟（非 0/1 退出即失败）——DLL 不全的 exe 会在这里红。

**`.github/workflows/runtime.yml`**：
- `workflow_dispatch` 增加输入 `cuda_runner`（默认空）；`build-cuda` 的 `runs-on` 改为表达式（空 → `windows-latest`）。
- `build-cuda` 步骤改为调用 `stage-runtime.ps1` 时传 `-CudaBinDir "$env:CUDA_PATH\bin"`；其余 flag 与本机脚本一致（arch 列表可由输入覆盖）。
- 宿主 runner 无 CUDA 时仍按现行为失败并给出「需要 CUDA runner」的原话，不静默跳过。

**发布与 registry**：
1. `gh release upload runtime-v1.9.5 dist/whispercpp-1.9.5-cuda12-win-x64.zip --clobber`。
2. 成功后把 `internal/registry/data/registry.json` 的 `whispercpp-cuda12-win-x64` 改为：
   ```json
   "archive": {
     "url": "https://github.com/<owner>/stemscribe/releases/download/runtime-v1.9.5/whispercpp-1.9.5-cuda12-win-x64.zip",
     "size": <实测字节>,
     "sha256": "<实测>",
     "dest": "whisper-cli-cuda12.zip"
   }
   ```
   删除 `tbd`。**上传失败则不写**，条目保持 `tbd:true`，AC-02 记为未达成（CHG 不关闭）。
3. 安装落点：`workdir/runtimes/whisper/1.9.5-cuda12/`（现有 `FindRuntime` 约定）。
4. 后端选择顺序：`cuda12 → vulkan → cpu`，只考虑**已安装**；任何降级在 reason 里点名。

**路线 B 回退**（仅当 A 失败）：上游 `whisper-cublas-12.4.0-bin-x64.zip`（v1.8.2，实测 457,274,520 B）→ 作为 `version 1.8.2` 的 cuda12 条目，url 指上游 release，sha256 实测；CHG.md 记录版本不一致这一事实。

### 3.2 后端真相（R2）

新增 `runctl.BackendReport(workdir) BackendStatus`，`probe` 与 `/api/capabilities` 都只调它：

```go
type BackendStatus struct {
    Backend        registry.Backend // 实际会用的
    Reason         string           // 人话，含「未安装 → 降级到 X」
    RuntimeVersion string           // 已安装运行时的版本，无则空
    Installed      bool
}
```

- `resource.GPUName() string`：`nvidia-smi --query-gpu=name --format=csv,noheader`，缓存（复用 smiprobe 思路）；失败返回空，UI 显示 `—`。
- `/api/capabilities` 字段新增：`backend_runtime_version`、`gpu_name`；`backend` / `backend_reason` 语义不变但值改为实际。
- `probe` 输出：
  ```
  backend      : vulkan (installed 1.9.5)
  reason       : NVIDIA GPU with CUDA 12 runtime, but cuda12 runtime is not installed; using vulkan
  gpu          : NVIDIA GeForce RTX 3060
  ```
- meta：`provenance.engines.transcribe.backend` = 实际后端（`Composite` 内嵌 `*Whisper`，直接读 `Backend`）；`engines.separate.backend = "cpu"`（audio-separator 走 CPU onnxruntime，这是事实）。
- 能力页文案：
  - `separation = 分离运行时 + 分离模型都已安装`；`separation_reason` 分别写「已就绪（UVR5 · <版本>）」或缺什么；
  - `polish` 保持 false，理由「LLM polishing needs a provider (CHG-0004)」；
  - **删除** `"vocal separation is not implemented yet (CHG-0003)"`。
- 侧栏：`backend` 行改为 `VULKAN · RTX 3060`（大写后端 + 去掉 `NVIDIA GeForce ` 前缀），`title` 显示 `backend_reason`；无 GPU 名时只显示后端。

### 3.3 停止任务（R5）

**Service**（`internal/runctl/service.go`）：
- 新增 `StateCanceled State = "canceled"`。
- `Status` 新增 `ElapsedMS int`（Get/Latest 时按 `StartedAt` 实时计算，运行中才增长）。
- `Service` 内部 `cancels map[string]context.CancelFunc`；`Start` 用 `context.WithCancel(context.Background())`，`execute(ctx, …)` 用该 ctx；结束时 delete。
- `Cancel(id) error`：未知 → `ErrRunNotFound`；已结束 → `ErrNotRunning`；否则 `cancel()`。
- `execute` 结束判定：`errors.Is(err, context.Canceled) || ctx.Err() != nil` → `StateCanceled`，`Message = "stopped by user"`；否则 `StateFailed`。
- 测试钩子：包内 `var runFn = Run`，`execute` 调 `runFn`，便于单测不依赖 GPU/模型。

**Queue**（`internal/runctl/queue.go`）：
- `Runner` 接口加 `Cancel(id string) error`。
- `QueueState` 加 `QueueCanceled = "canceled"`。
- `QueueItem` 加 `Stage string`、`ElapsedMS int`、`Stopping bool`。
- `Stop(id) error`：仅 `running` 项；置 `Stopping=true` 后调 `runner.Cancel(run_id)`；返回后由轮询把项置 `canceled`。
- 轮询里镜像 `st.Stage` / `st.ElapsedMS`；`StateCanceled` → 项 `canceled`、`Stopping=false`；随后 worker 取下一项（现有逻辑）。
- 等待项仍用 `Remove`。

**进程树**（`internal/engine`）：
- 新增 `proc_windows.go` / `proc_unix.go`：`wireCancel(cmd *exec.Cmd)` 设置
  `cmd.Cancel = func() error { return killTree(cmd.Process) }`、`cmd.WaitDelay = 3 * time.Second`。
  Windows `killTree`：`taskkill /T /F /PID <pid>`，失败再 `Process.Kill()`；Unix 有进程组则杀组，否则 `Kill`。
- 应用到：`whisper.go defaultExec`、`separator.go runWithEnv`、`ffmpeg.go` 的启动、`separator_install.go` 的探测命令。
- pipeline 已经把 ctx 传进引擎，取消会沿 `exec.CommandContext` 生效（现有 `defaultExec` 注释里的 AC-08 承诺就此真正成立）。

**meta / 资料库**：
- `run.go` 失败分支：若 `errors.Is(err, context.Canceled)` → `Status = "canceled"`，失败阶段 `State = "canceled"`（不是 failed）。
- `library.go` 原样读出，UI 对 `canceled` 显示「已停止」。
- checkpoint 不在取消时改写；恢复时 `Plan` 会重跑未完成的阶段（由现有 checkpoint 语义保证，E2E 验证）。

### 3.4 首页：拖入区 → 暂存 → 开始（R3，取代 ADR-040）

**组件结构（NewTask.tsx）**：

```text
新建任务
┌ 拖放区 .dropzone（1px dashed --line-hi，2px 圆角，padding 32，min-height 160）┐
│  拖入视频或音频文件                         （--fs-lead，左对齐）            │
│  MP4 · MKV · MOV · WEBM · WAV · MP3 · M4A · FLAC                            │
│  [ 浏览文件… ]  （次按钮，原生多选）                                        │
│  文件只加入下方待处理列表，点「开始转换」才执行                              │
└──────────────────────────────────────────────────────────────────────────────┘
待处理 3                                                    [清空]
文件名                             大小              操作
a.mp4                             412 MB            [移除]
b.mkv                             1.2 GB            [移除]
──────────────────────────────────────────────────────────────
输出目录 [D:\transcripts              ] [选择…] [打开]
模型 [large-v3 ▾]   语言 [中文 ▾]            [保存设置]
[ 开始转换 (3) ]   ← 主按钮：有待处理且环境就绪时可点
▸ 手动输入路径（折叠，键盘/浏览器兜底）
──────────────────────────────────────────────────────────────
队列 2/3
文件        状态/阶段          阶段·耗时     动作
c.mp4      运行中 分离中       分离中 · 12:03 [停止]
d.mp4      待处理              —            [移除]
e.mp4      已完成              —            [打开目录]
```

**行为**：
- 拖入（Wails `runtime.OnFileDrop`）与「浏览文件…」（`App.PickFiles`）都只调 `addStaged(paths)`：按路径去重、追加 `{path,size}`；**不发起任何网络请求**。
- 文件大小来自新绑定 `App.StatFiles(paths) []FileMeta{Path,Size,Err}`；浏览器壳无此能力，显示 `—`。
- 「开始转换」：把暂存全部 `POST /api/queue`；成功后才从暂存移除（被拒绝的留在列表并提示）；失败不动列表。
- 环境未就绪（`missing.length > 0`）：横幅保留，主按钮 `disabled` 且 `aria-describedby` 指向横幅。
- 手动输入路径折叠在 `<details>` 内，保留多行文本框 + 同样的开始逻辑（浏览器壳可用）。
- 纯前端模块 `frontend/lib/staging.ts`：`addStaged(list, files)` / `removeStaged(list, path)` / `dedupe`，供组件与测试使用。

**队列行动作矩阵**：

| 状态 | 动作 |
|---|---|
| running | `[停止]`；`stopping` 时禁用并显示「正在停止…」 |
| waiting | `[移除]` |
| done | `[打开目录]` |
| failed | 有 `artifact_dir` → `[打开目录]`；否则只显示原因 |
| canceled | 有 `artifact_dir` → `[打开目录]`；否则无动作 |

**队列行信息**：状态词 + 阶段名（`stage.separate` 等 i18n）+ 已用时间 `mm:ss`（来自 `elapsed_ms`）。

### 3.5 输出目录（R4）

- 行内三段：输入框（可手输）/ `[选择…]` / `[打开]`。
- `[选择…]` → 新绑定 `App.PickFolder(initial string) (string, error)`（Wails `OpenDirectoryDialog`，初始目录 = 当前值）。选中后写入输入框并**立即 `PUT /api/settings`**（保存默认）；失败（不可写）则原样报错、不回填。
- `[打开]` → `OpenFolder(outputDir)`；空值 → 打开 `<workdir>/libraries`（`settings.workdir` 已由接口返回）。
- 可写性检查：新增 `runctl.CheckOutputDir(dir string) error` = `MkdirAll` + 写入并删除临时探针文件 `.stemscribe-write-test-<rand>`；`PUT /api/settings` 与 `POST /api/queue`（`output_dir` 非空时）都调用它，失败返回 400 与原因。
- 留空语义不变：`<workdir>/libraries`。

### 3.6 资料库（R6）

- `/api/library` 已返回 `dir`；`BundleSummary` 增加 `model_id` 与 `transcribe_backend`（读 `provenance.engines.transcribe`）。
- 表格新增「模型」列（`model_id` + 后端小字）与「操作」列：`[打开目录]`（`shell().OpenFolder(dir)`；浏览器壳复制路径并提示）。
- 不新增逐字稿查看器（PRD 非目标）。

### 3.7 模型中心（R7）

- `CatalogItem` 增加 `path`、`version`；已安装才填 `path`：
  - whisper 模型：`engine.FindModel` 的目录；
  - whisper 运行时：`engine.FindRuntime(...).Root`，版本取运行时目录名（如 `1.9.5`）；
  - 分离运行时：`runtimes/separator`，版本 `engine.SeparatorVersion`；
  - separator 模型：模型文件目录；
  - ffmpeg：`FindFFmpeg` 的父目录，版本取 registry。
- 已安装行显示：版本、大小、路径（等宽、溢出省略、`title` 全路径）、`[打开目录]`。
- 未安装行保持安装按钮/进度；CUDA 条目在 `tbd` 时不可安装（现有拒绝逻辑），行内给原话原因。

### 3.8 接口契约

| 方法 | 路径 | 变更 |
|---|---|---|
| GET | `/api/capabilities` | +`backend_runtime_version`、+`gpu_name`；`separation_reason` 反映真实安装状态 |
| GET | `/api/queue` | 项 +`stage`、+`elapsed_ms`、+`stopping`；`state` 可为 `canceled` |
| POST | `/api/queue` | `output_dir` 非空时先 `CheckOutputDir`；不可写 → 400 |
| POST | `/api/queue/{id}/stop` | 仅 running → 202；非 running → 409；未知 → 404 |
| DELETE | `/api/queue/{id}` | 不变（仅 waiting） |
| POST | `/api/runs/{id}/cancel` | 202；已结束 → 409；未知 → 404 |
| GET | `/api/library` | 项 +`model_id`、+`transcribe_backend`（`dir` 已存在） |
| GET | `/api/models` | 项 +`path`、+`version` |
| PUT | `/api/settings` | `output_dir` 用 `CheckOutputDir`（可写探针） |

Wails 绑定新增：`PickFolder(initial string) (string, error)`、`StatFiles(paths []string) []FileMeta`；`OpenFolder` 增加文件/目录都存在才打开、否则返回原话错误。

### 3.9 视觉规范（taste：主 cg-01 瑞士平面 + 辅 cg-07 只取「单一光源」）

- **不加新颜色**；只用 `docs/UI-DESIGN.md` §3/§14 的语义 token。
- 拖放区：`--surface` 底、1px **dashed** `--line-hi`、2px 圆角、无阴影；hover/focus 边变 `--lamp`，**不发光**。
- 暂存表沿用 `.grid`：行高 40px、无斑马纹、数值右对齐等宽。
- 主按钮「开始转换 (n)」= 唯一允许的空态光源（`--lamp-glow` ≤0.08）；有暂存即出现，不另外加光。
- 运行中行：左侧 2px `--lamp` 竖线 + 极淡 `--lamp-glow`（≤0.10），与既有「单一光源」规则一致；`stopping` 时竖线变 `--line-hi`。
- 状态点：running=`--lamp` 呼吸；done=`--bone-dim`；fail=`--fail`；queued=`--line-hi`；**canceled=`--line-hi` + 「已停止」文字**（不新增色相）。
- 图标只用单线条；不用 emoji；动效沿用 §9（≤250ms，禁弹跳）。
- 页面最大宽 1040px、左对齐；所有新文案进 i18n（中英同键）。

**新增 i18n 键（zh / en）**：

| key | zh | en |
|---|---|---|
| `task.dropTitle` | 拖入视频或音频文件 | Drop video or audio files |
| `task.dropHint` | 文件只加入待处理列表，点「开始转换」才执行 | Files are only staged; press Start to run |
| `task.browse` | 浏览文件… | Browse… |
| `task.staged` | 待处理 | Staged |
| `task.clear` | 清空 | Clear |
| `task.start` | 开始转换 | Start |
| `task.startCount` | 开始转换 ({n}) | Start ({n}) |
| `task.manualPaths` | 手动输入路径 | Enter paths manually |
| `task.sizeUnknown` | — | — |
| `task.outputPick` | 选择… | Choose… |
| `task.outputOpen` | 打开 | Open |
| `task.stop` | 停止 | Stop |
| `task.stopping` | 正在停止… | Stopping… |
| `task.canceled` | 已停止 | Stopped |
| `stage.preparing` | 准备中 | Preparing |
| `stage.canceled` | 已停止 | Stopped |
| `library.openDir` | 打开目录 | Open folder |
| `library.colModel` | 模型 | Model |
| `models.openDir` | 打开目录 | Open folder |
| `models.version` | 版本 | Version |
| `models.path` | 安装路径 | Install path |

> 复用既有：`task.pick`、`task.remove`、`task.openFolder`、`task.outputDir`、`run.*`、`stage.*` 的分离/转写/对齐/导出。

### 3.10 文档

| 文件 | 变更 |
|---|---|
| `docs/UI-DESIGN.md` | §7 新建任务/队列行更新；§8 增拖放区、停止按钮、canceled 状态点；新增 **「桌面工具基本盘」** 清单（PRD §6 原表，作为页面验收项） |
| `docs/MODEL-REGISTRY.md` | CUDA 运行时条目、构建/发布流程、DLL 捆绑要求、失败时 `tbd` 的诚实语义 |
| `docs/PIPELINE.md` | 取消语义（阶段边界、checkpoint 保留、进程树） |
| `docs/ARCHITECTURE.md` | 进程树取消与 Wails 目录/文件信息绑定 |
| `docs/GAP-ANALYSIS.md` | CUDA 状态从「未产出」更新为实测结果（或阻塞事实） |
| `tasks/CHG-0015/CHG.md` | ADR-040 标注「已被 CHG-0016 ADR-042 取代」，不改原文其余内容 |
| `tasks/README.md` | CHG-0016 状态与范围同步 |

## 4. 独占文件

```
internal/registry/data/registry.json
internal/engine/{whisper,separator,ffmpeg,locate}.go
internal/engine/proc_windows.go (新)  internal/engine/proc_unix.go (新)
internal/engine/*_test.go（新增/追加）
internal/runctl/{service,queue,run,helpers,catalog,capabilities,library}.go
internal/runctl/outputdir.go (新)  internal/runctl/*_test.go
internal/webapi/handler.go  internal/webapi/*_test.go
internal/resource/{scheduler,smiprobe}.go（仅加 GPUName，不改选择算法）
internal/store/bundle.go（仅 EngineInfo 已有点位填值，如有必要）
cmd/stemscribe-gui/main_gui.go
cmd/stemscribe/cli.go（仅 probe 输出）
frontend/app/components/{NewTask,Sidebar}.tsx
frontend/app/{library,models,params}/page.tsx
frontend/lib/{api,i18n}.ts  frontend/lib/staging.ts (新)
frontend/app/globals.css（仅新增类，不改既有 token）
.github/workflows/runtime.yml  .github/scripts/stage-runtime.ps1
scripts/build-runtime-cuda.ps1 (新)
docs/{UI-DESIGN,PIPELINE,MODEL-REGISTRY,ARCHITECTURE,GAP-ANALYSIS}.md
tasks/CHG-0015/CHG.md（仅 ADR-040 注记）  tasks/CHG-0016/**  tasks/README.md
```

## 5. 测试计划

**Go（全部进 `go test ./... -race`）**

| 测试 | 断言 |
|---|---|
| `TestServiceCancelMarksCanceled` | 用阻塞的 `runFn` 启动 → Cancel → 状态 `canceled`（不是 failed），`ElapsedMS` 非负 |
| `TestServiceCancelUnknownAndFinished` | 未知 id → 错误；已完成 → 409 对应错误 |
| `TestQueueStopRunningContinuesNext`（fake Runner） | 停止当前项：项变 canceled、下一项开始；等待项不受影响 |
| `TestQueueSnapshotCarriesStageAndElapsed` | 快照带 `stage`、`elapsed_ms`、`stopping` |
| `TestRemoveWaitingStillWorks` | 现有语义回归 |
| `TestKillTreeKillsGrandchildren`（windows tag） | 启动 `cmd /c "ping -n 30 ..."` 孙进程，取消后 `tasklist` 5s 内无残留 |
| `TestBackendReportUsesInstalledRuntime` | 临时 workdir 只造 vulkan 运行时 → backend=vulkan，reason 含「not installed」 |
| `TestCapabilitiesBackendMatchesBackendReport` | 两处同源 |
| `TestCapabilitiesSeparationReasonTracksInstall` | 无分离运行时 → false + 缺项；伪造安装记录 → true |
| `TestCatalogInstalledItemsExposePathAndVersion` | 已安装项 `path` 存在、`version` 非空 |
| `TestCheckOutputDirRejectsFile` | 以文件路径当目录 → 错误；正常目录 → nil |
| `TestQueueRejectsUnwritableOutputDir`（handler） | 400 + 原因 |
| `TestStopAndCancelRoutes` | 202/409/404 矩阵 |
| `TestLibraryExposesModelAndBackend` | 用 meta.json fixture |

**前端/契约**：`pnpm lint`、`pnpm typecheck`、`pnpm build`、`scripts/verify-contract.sh`、`scripts/i18n-check.sh`、`python scripts/contrast.py`（新类不引入新色值）。

**E2E / 人工（证据落 `tmp/evidence/chg0016/`，不入库）**

| 场景 | 步骤 | 证据 |
|---|---|---|
| CUDA 对照 | 20 s 片段，`settings.runtime.locked_backend=cuda12` 跑一次、`vulkan` 跑一次 | `engine.log` 的 CUDA 设备行、两次 `runs/*.jsonl` 的 transcribe 耗时、两份 meta 的 backend |
| 停止 | 20 s 片段进入分离阶段后点停止 | 停止前后 `tasklist` 快照、队列 JSON、meta `canceled`、重跑日志显示 separate 重跑而 demux 跳过 |
| 拖入暂存 | 桌面窗口拖 3 个文件 | 拖入后截图、`GET /api/queue` 为 0、`libraries/` 无新目录；点开始后队列 3 |
| 目录动作 | 资料库/模型中心点「打开目录」 | 资源管理器截图 + API 返回路径一致性断言 |
| 中英/明暗 | 首页与队列在 4 种组合下截图 | 对比度脚本通过、无新增色值 |

## 6. 验收标准（全部可红）

| AC | 判定 | 方式 |
|---|---|---|
| **AC-01 CUDA 真用上** | 装好 cuda12 运行时后跑 20 s 片段：`engine.log` 出现 CUDA 设备/`using CUDA0`，meta `transcribe.backend=cuda12`；与 vulkan 同片段各跑一次并记录耗时（**不预设谁快**） | E2E 脚本 + 证据 JSON |
| **AC-02 CUDA 可安装** | 干净 workdir：`stemscribe runtime install whispercpp-cuda12-win-x64` 退出 0；`runtimes/whisper/1.9.5-cuda12/` 含 `whisper-cli.exe` + 三个 CUDA DLL；`whisper-cli --help` 退出 0/1；registry size/sha256 为实测且无 `tbd` | 命令 + `Get-FileHash` + 目录清单；缺 DLL / sha 为 null 即失败 |
| **AC-03 口径一致** | 同一次运行的 `probe`、`/api/capabilities`、meta 报同一后端；临时移走 cuda12 运行时后三处同时变 `vulkan`，reason 含「cuda12 … not installed」 | 三份输出对比；不一致即失败 |
| **AC-04 拖入→暂存→开始** | 拖 3 个文件：暂存 3 行、`GET /api/queue` 项数 0、`libraries/` 无新目录；点开始后队列 3；每行可移除 | 人工步骤 + 截图 + 队列 JSON；任一越权即失败 |
| **AC-05 输出目录** | 选择器回填绝对路径并保存；「打开」在资源管理器定位；产物落在该目录；不可写路径在点击时 400（临时探针文件可复现） | 人工 + `PUT /api/settings` 400 断言 + E2E 产物路径 |
| **AC-06 停止** | 分离阶段点停止：≤5 s 内 python/ffmpeg 进程消失（含孙进程）、任务 `canceled`、后续阶段不启动；同 workdir 再次开始从 checkpoint 续跑 | `tasklist` 前后快照 + 队列/元数据 + 二次运行日志 |
| **AC-07 资料库** | 每行「打开目录」打开对应 bundle；`dir` 来自 API 且与磁盘一致 | API 断言 + 人工打开 |
| **AC-08 模型中心** | 已安装行显示版本、大小、路径，「打开目录」打开实际路径 | 人工 + `path` 存在性断言 |
| **AC-09 任务行** | 运行中显示阶段名与耗时；动作随状态出现（停止/移除/打开目录） | 截图 + 队列 JSON `stage/elapsed_ms` |
| **AC-10 文案诚实** | 能力页 `separation=true`（已装时）；`polish=false`；全仓 `grep` 无 `not implemented yet (CHG-0003)` 与伪造 CUDA 声明 | `grep` + capabilities JSON |
| **AC-11 质量门** | `go build/vet/test -race`、`pnpm lint/typecheck/build`、`verify-contract`、`i18n-check`、`contrast`、`redact-scan`、`secrets-scan` 全绿；中英键数一致 | Checkfix 输出 |

> AC-04/05 含人工步骤（Wails 原生对话框与 OS 拖放无法在纯 CLI 自动化）；人工步骤必须有截图与命令输出，不能只写「已验证」。AC-01/02/03/06/07/09/10 以脚本断言为主。

## 7. 里程碑与执行顺序（严格按序）

| # | 内容 | 产出 | AC |
|---|---|---|---|
| **M0** | 环境自检：vcvars64、nvcc、cmake（+ninja 可选）、源码浅克隆通路；记录耗时与失败点 | `tmp/evidence/chg0016/m0-toolchain.txt` | — |
| **M1** | CUDA：脚本 → 构建 → stage（DLL 断言 + `--help`）→ 上传 → registry 实测 → 干净 workdir 安装 → 后端对照测速 → 后端真相统一（probe/capabilities/meta/侧栏/文案） | 脚本、zip、registry 条目、`ac01/ac02/ac03` 证据 | AC-01~03、AC-10（分离文案部分） |
| **M2** | 首页：拖入区 + 浏览 + 暂存列表 + 开始按钮（推翻 ADR-040）+ StatFiles 绑定 + i18n/CSS | 新首页布局、ADR-041/042 | AC-04 |
| **M3** | 输出目录：PickFolder + 打开 + 可写探针（PUT/queue 前置校验） | 绑定与校验、E2E 产物路径 | AC-05 |
| **M4** | 停止：Service cancel + Queue stop + 进程树 + canceled 状态 + meta/UI + 续跑 | 后端/API/UI、进程表证据 | AC-06 |
| **M5** | 资料库/模型中心打开目录；队列行阶段+耗时；能力页剩余文案 | API 字段与页面 | AC-07~10 |
| **M6** | 全量 E2E、Checkfix、脱敏/密钥扫描、文档同步、CHG 关闭、显式列文件提交（**需 owner 同意**） | 证据汇总 | AC-11 |

**M1 时间盒**：源码获取或编译连续失败两次且无法在 90 分钟内解决 → 切换路线 B，并在 `CHG.md` 记录已试方案与失败原因；未达 AC-02 不得宣称完成。

## 8. 风险与回滚

| 风险 | 对策 / 回滚 |
|---|---|
| whisper.cpp 源码拉取慢或失败（GitHub 单连接约 76 KB/s） | `--depth 1` + 仅必要子模块；失败两次走路线 B（上游 1.8.2 cublas 包，实测可达 457 MB） |
| CUDA 编译耗时/磁盘占用大 | 固定 `CMAKE_CUDA_ARCHITECTURES`，只构建 `whisper-cli`；产物放 `tmp/build`（已 gitignore）；构建失败不影响现有 vulkan 路径 |
| 归档无法上传（体积/网络） | registry 保持 `tbd:true`，不写假 sha；UI 明示「CUDA 未分发，当前 Vulkan（也是 GPU）」；CHG 不关闭 AC-02 |
| DLL 不全导致 exe 起不来 | stage 脚本硬断言三件 DLL + `--help` 冒烟；不过不发包 |
| 取消后孙进程残留 | `taskkill /T /F` + `WaitDelay`；AC-06 用进程表断言；仍残留则升级 Job Object（记录为后续方案） |
| 取消与阶段完成竞态 | 以 ctx 错误为准：只有 `context.Canceled` 记 canceled；若阶段刚好完成则如实显示 done |
| 拖入语义变更影响老用户习惯 | ADR-042 明示；拖入区文案写清「只加入列表」；保留手动路径入口 |
| 原生对话框/路径含中文或空格 | E2E 用中文路径测一次；路径始终作为单个参数传给 explorer |
| 停止导致 checkpoint 半写 | checkpoint 只在阶段成功时原子写入；E2E 验证二次运行重跑未完成阶段 |
| 暂存刷新丢失 | 已记录在设计里：未入队文件丢失可接受，文案不承诺持久化 |

## 9. 完成定义

1. §6 全部 AC 有证据（脚本输出/截图/JSON），AC-02 未达成则 CHG 不关闭。
2. Checkfix + 脱敏/密钥扫描全绿；无新增色值；中英键一致。
3. 文档同步（§3.10）；CHG.md 状态流与 ADR-041/042 记录完整。
4. 提交前 owner 明确同意；显式列文件，禁止 `git add .`；不提交 `tmp/`、`workdir/`、任何实测绝对路径或密钥。
