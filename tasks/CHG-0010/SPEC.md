# CHG-0010 · Run Control & Live Progress

| 字段 | 值 |
|---|---|
| status | **approved** |
| owner | 本会话（owner 已授权全自动开发/测试/提交/推送） |
| 依赖 | CHG-0002（引擎与运行时，已闭环）、CHG-0008（参数面，已闭环） |
| 关联 ADR | ADR-002（Go 只编排）、ADR-004（不许静默降级）、ADR-016（参数表单一真相源）、ADR-022（运行服务单一入口）、ADR-023（进度不得伪造） |

## 1. 背景与问题（为什么必须做）

实测事实，不是推测：

- `frontend/app/components/NewTask.tsx` 的「开始」按钮**没有任何 `onClick`**。
- `cmd/stemscribe-gui/main_gui.go` 只暴露 `Version()` 一个绑定，**没有启动任务的入口**。

因此用户单击「开始」后：没有请求、没有事件、没有状态变化——**界面在说谎**，它让用户以为任务已经开始。
这违反项目重心第 1 条（产物必须可信）的延伸：**过程也必须可观察**。一个不给反馈的按钮，
与一个失败的按钮，对用户是同一件事。

同时 CLI 已有完整运行链路（demux→transcribe→align→export），且已验证产出真实文字。
所以问题不在推理能力，而在**运行控制面缺失**：没有人把「用户点了开始」接到「管线开始跑」，
也没有人把「管线跑到哪了」接回界面。

## 2. 范围与不做

**做**

- 新建 `internal/runctl`：唯一的运行入口（同步执行 + 异步服务 + 状态快照）。
- 新建 `internal/webapi`：一个 `http.Handler`，提供运行控制与状态查询。
- 同一 handler 挂载到**两处**：`cmd/stemscribe/shell.go`（浏览器）与 Wails `AssetServer.Handler`（原生窗口）。
  这是 ADR-022 的核心：**不是两套逻辑，是同一条路径的两种外壳**。
- 前端进度区：阶段、进度、耗时、失败原因、产物路径。
- `scripts/build.ps1` 产出与源码同步的 `dist/`。

**不做（明确划出，避免范围膨胀）**

- 不做多任务并发队列（一次一个运行，状态机简单可审计）。
- 不做暂停/取消按钮的精细语义（先接 `context` 取消，UI 后续）。
- 不做 SSE/WebSocket：轮询 500ms 足够，且实现面小、易验证（ADR-023）。
- 不做润色/分离阶段的真实推理（分别归 CHG-0004/CHG-0003）。

## 3. 设计

### 3.1 单一运行服务（ADR-022）

```
                 ┌──────────────┐
  browser ──HTTP─▶│  webapi      │──▶ runctl.Service ──▶ pipeline.Runner ──▶ engine
  window  ──HTTP─▶│ (http.Handler)│         │
                 └──────────────┘         └─ Status 快照（可重连）
```

- `runctl.Service.Start(opts)` 返回 `runID`，立即返回，不阻塞请求。
- 运行在 goroutine 内，事件经 `emit` 回调写进 `Status`。
- `Status` 保留在内存（含最近一个运行），因此**刷新页面/重开窗口仍能接回**。

### 3.2 进度不得伪造（ADR-023）

进度由 `pipeline` 的真实事件推导：

- 分母 = 本次**启用**的阶段数（`separate`/`polish` 被禁用时不计入）。
- 分子 = 已 `stage.end` 的阶段数。
- 界面同时显示当前阶段名，因此即使百分比粗，用户也知道在干什么。

禁止：用计时器递增的假进度条、用固定 sleep 模拟阶段。

### 3.3 CLI 与窗口同源

`cmd/stemscribe/cli.go` 的编排逻辑移入 `runctl`，CLI 改为调用它并把事件打印出来。
CLI 输出必须与今天逐字一致（回归判据），否则说明抽取改变了行为。

## 4. 契约

### 4.1 HTTP

| 方法 | 路径 | 语义 |
|---|---|---|
| `POST` | `/api/runs` | 启动一个运行；body 为 `RunRequest`；返回 `{id}` |
| `GET` | `/api/runs/latest` | 最近一个运行的 `Status`（无则 404 + `{error}`） |
| `GET` | `/api/runs/{id}` | 指定运行的 `Status`（未知 id 返回 404） |
| `GET` | `/api/capabilities` | 当前可用性：模型/后端/分离是否就绪（供 AC-09 置灰用，本 CHG 只暴露不置灰） |

### 4.2 `RunRequest`

```json
{
  "input": "C:/Temp/test.mp4",
  "model": "whisper-tiny",
  "params": { "input_language": "en", "beam_size": "1" }
}
```

`params` 的键与 `engine.Specs()` 的 `key` 一一对应（ADR-016 的延续：界面、CLI、API 共用一张表）。

### 4.3 `Status`

```json
{
  "id": "r-1a2b3c",
  "state": "running",
  "stage": "transcribe",
  "stage_index": 4,
  "stage_total": 6,
  "percent": 67,
  "segments": 0,
  "artifact_dir": "",
  "message": "",
  "events": [{"at": "2026-11-19T...", "stage": "demux", "kind": "end"}],
  "started_at": "2026-11-19T...",
  "ended_at": ""
}
```

`state` ∈ `queued` | `running` | `done` | `failed`。

## 5. 里程碑

| 里程碑 | 内容 | 文件 | 变绿 AC |
|---|---|---|---|
| M1 | 抽取 `runctl.Run`，CLI 改为调用它（输出不变） | `internal/runctl/**`、`cmd/stemscribe/cli.go` | AC-05 |
| M2 | 异步 `Service` + `Status` 快照 | `internal/runctl/service.go` | AC-02/AC-07 |
| M3 | `webapi` + shell 挂载 + Wails `AssetServer.Handler` 挂载 | `internal/webapi/**`、`cmd/stemscribe/shell.go`、`cmd/stemscribe-gui/**` | AC-05 |
| M4 | 前端进度区（启动/阶段/完成/失败）+ i18n | `frontend/app/**`、`frontend/lib/i18n.ts` | AC-01/AC-03/AC-04/AC-06 |
| M5 | `dist/` 构建同步 + 真实点击验证 + 截图 | `scripts/build.ps1`、`docs/images/**` | AC-08 |

**DoD**：AC-01…AC-08 全绿并写入 TRACEABILITY；`go build/vet/test` 全绿；`i18n-check.sh` 通过；
真实视频从界面启动能跑到 `done` 且逐字稿有字；`dist/` 内的 exe 与 assets 与当前源码一致。

## 6. 验收标准

| AC | 判据 | 验证方式 |
|---|---|---|
| AC-01 | 点击「开始」后 ≤500ms 出现进度区，显示「已启动」+ 输入文件名；按钮进入忙碌态且不可重复点击 | 截图 + 手工点击 |
| AC-02 | 进度按真实 pipeline 事件推进；分母为启用阶段数；禁止计时器假进度 | `internal/runctl` 单测断言阶段序列 + 手工观察 |
| AC-03 | 失败时进度区转失败态，显示失败阶段 + 引擎原话（stderr 末行）+ 可行动建议 | 用不存在的模型跑一次 |
| AC-04 | 完成后显示 segments 数与产物目录 | 真实视频跑到 done |
| AC-05 | 浏览器与原生窗口共用同一 `http.Handler`；前端只有一条 fetch 代码路径 | 代码结构 + curl 与窗口双验证 |
| AC-06 | 进度区中英双语、双主题，`i18n-check` 通过、无硬编码 CJK | `scripts/i18n-check.sh` + 截图 |
| AC-07 | 刷新页面/重开窗口能接回当前运行状态 | `GET /api/runs/latest` 手工验证 |
| AC-08 | `scripts/build.ps1` 产出的 `dist/stemscribe.exe` 与 `dist/stemscribe-cli.exe` 反映当前源码（含最新 `frontend/out`） | 构建后比时间戳 + 启动验证 |

## 7. 风险与对策

| 风险 | 对策 |
|---|---|
| 抽取 `runctl` 改变 CLI 行为 | 以现有 CLI 输出为基线逐字比对；`go test ./...` + 真视频 e2e 双证 |
| Wails 挂载 handler 后与静态资源冲突 | handler 只接管 `/api/`，其余交回 `Assets`；用 curl 验证两条路径都通 |
| 轮询造成状态撕裂（读到半写） | `Status` 读写全部走 `Service.mu`，返回拷贝 | 
| 长运行阻塞 webview | 运行在 goroutine，HTTP 立即返回；轮询本身无阻塞 |

## 8. 依赖与外部条件

- 需已安装运行时与 `whisper-tiny` 模型（本机已具备，`dist/workdir` 内实测可用）。
- 无新增第三方 Go 依赖。
- 无新增前端依赖（轮询用原生 `fetch`）。

## 9. 独占文件清单

| 范围 | 归属 |
|---|---|
| `internal/runctl/**`（新建） | CHG-0010 |
| `internal/webapi/**`（新建） | CHG-0010 |
| `cmd/stemscribe-gui/**` | CHG-0010 |
| `cmd/stemscribe/{cli,shell,wire}.go` | CHG-0010（编排迁移，**不改变**引擎参数语义；参数面仍归 CHG-0008） |
| `internal/pipeline/**`、`internal/store/**` | 只读引用，**不改**（如需改动另立子项并同批登记） |
| `frontend/app/**`、`frontend/lib/{i18n,api}.ts` | CHG-0010（与 CHG-0007 交叠，主题/字典机制沿用，不重写） |
| `scripts/build.ps1` | CHG-0010 |
| `docs/{ARCHITECTURE,PIPELINE,UI-DESIGN}.md`、`tasks/README.md` | CHG-0010 |
