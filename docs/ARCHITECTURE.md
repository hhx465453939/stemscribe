# 架构

> 决策依据见 `DECISIONS.md`（ADR-001…008）。本文只描述**结构、契约、边界**。

## 1. 分层

```
┌──────────────────────────────────────────────────────────────┐
│ L4  UI        Next.js 15 (static export) · WebView2          │
│               任务看板 / 单集详情 / 模型中心 / 设置            │
└────────────────────────┬─────────────────────────────────────┘
                         │ HTTP /api/**（同一个 handler 挂到 loopback 与 WebView2）
┌────────────────────────▼─────────────────────────────────────┐
│ L3  App Core  Go：用例编排 / 状态机 / 事件总线                 │
│     internal/runctl   internal/webapi                          │
│     internal/pipeline   internal/store   internal/provider     │
└───┬──────────────┬───────────────┬──────────────┬────────────┘
    │              │               │              │
┌───▼────┐  ┌──────▼──────┐  ┌─────▼──────┐  ┌────▼─────────┐
│resource│  │  registry   │  │  provider  │  │  mcpserver   │
│显存预算 │  │ +downloader │  │ LLM 网关   │  │ Agent 接入    │
└───┬────┘  └──────┬──────┘  └─────┬──────┘  └──────────────┘
    │              │               │
┌───▼──────────────▼───────────────▼───────────────────────────┐
│ L2  Engines（外置进程，JSONL 协议）                            │
│  whisper.cpp（cuda/vulkan/directml/cpu）                       │
│  audio-separator（python-embed + onnxruntime）                 │
│  ffmpeg（转码/切片，随运行时安装）                              │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ L1  Storage   workdir：models / runtimes / libraries / cache  │
│               vault（DPAPI）· logs（脱敏）· tmp（原子写）      │
└──────────────────────────────────────────────────────────────┘
```

**边界铁律**
- L4 不允许直接读文件系统或 spawn 进程；一切经 L3。
- L3 不允许做张量运算；一切推理经 L2。
- L2 不允许访问网络（除 ffmpeg 不需要网）；不允许写 workdir 之外的路径。
- 每层只依赖下一层，**禁止跨层回调**（L2 不得直接通知 L4，必须经 L3 事件总线）。

## 2. 进程模型

| 进程 | 生命周期 | 职责 | 崩溃影响 |
|---|---|---|---|
| `stemscribe.exe`（主） | 应用存续 | UI 宿主 + 用例编排 + 下载 + DB | 应用退出 |
| `whisper-cli.exe` | 单任务阶段 | 转写 | 任务失败 → 降级重启 |
| `python.exe`（分离） | 单任务阶段（可常驻复用） | 分离 | 任务失败 → 重试/降级 |
| `ffmpeg.exe` | 秒级 | 转码/切片/探测 | 阶段失败 |
| `stemscribe.exe mcp` | agent 会话 | MCP server（只读） | agent 工具不可用 |

**孤儿防护**：Windows 用 **Job Object**（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`）把子进程绑到主进程；主进程异常退出时子进程被系统回收，避免 GPU 显存被僵尸进程占死。

**退出顺序**：停止接受新任务 → 向子进程发 `cancel` → 等待 ≤5 s → `TerminateJobObject` → flush checkpoint → 关日志。

## 3. 模块职责

| 包 | 职责 | 不做什么 |
|---|---|---|
| `internal/pipeline` | 阶段 DAG 编排、状态机、事件发布、checkpoint | 不做具体推理、不直接碰 UI |
| `internal/engine` | 引擎发现/启动/协议编解码/进度解析/降级 | 不做业务判断（用哪个模型由 resource+pipeline 决定） |
| `internal/registry` | 内置 registry 读取、校验、模型元数据 | 不负责下载 |
| `internal/downloader` | 分片并发、续传、sha256、原子落盘、镜像切换、限速 | 不做解压/安装策略 |
| `internal/resource` | NVML 探测、显存预算、LRU 常驻、降级决策 | 不启动进程 |
| `internal/provider` | LLM 协议适配、能力探测、流式解析、重试 | 不存密钥 |
| `internal/vault` | DPAPI 加解密、密钥生命周期 | 不做业务配置 |
| `internal/store` | bundle 读写、SQLite（索引/任务队列/FTS）、导出 | 不解析音频 |
| `internal/mcpserver` | MCP 工具实现（只读） | 不写库、不触发任务 |
| `internal/config` | workdir 解析、settings 迁移、schema 版本 | 不存密钥 |
| `cmd/stemscribe` | 入口、子命令（gui / mcp / cli）、单实例锁 | 不含业务逻辑 |

## 4. IPC 契约（L3 ↔ L4）

Wails bindings 暴露**粗粒度用例方法**，不暴露实体 CRUD：

```go
// 任务
func (a *App) Enqueue(req EnqueueRequest) (TaskID string, err error)
func (a *App) CancelTask(id TaskID) error
func (a *App) RetryTask(id TaskID) error
func (a *App) SkipStage(id TaskID, stage StageName) error

// 查询
func (a *App) ListTasks(filter TaskFilter) ([]TaskView, error)
func (a *App) GetLibraryItem(videoID string) (ItemView, error)
func (a *App) GetTranscript(videoID string, startMs, endMs int) ([]Segment, error)

// 模型与运行时
func (a *App) ListCatalog() (CatalogView, error)          // 已装/可装/下载中
func (a *App) Install(itemID string) (JobID string, err error)
func (a *App) Remove(itemID string) error
func (a *App) SetMirror(url string) error

// Provider
func (a *App) UpsertProvider(p ProviderConfig) error
func (a *App) RefreshModels(providerID string) ([]ModelInfo, error)
func (a *App) SetSecret(providerID, apiKey string) error       // 返回值永不回显
func (a *App) TestProvider(providerID string) (ProbeResult, error)

// 系统
func (a *App) ProbeHardware() (HardwareReport, error)
func (a *App) SetWorkdir(path string) error
```

**事件推送（Go → UI）**统一走 `runtime.EventsEmit("evt", payload)`，单一事件通道 + `type` 判别：

```jsonc
{ "type":"task.progress", "task_id":"...", "stage":"transcribe",
  "current":128, "total":240, "eta_s":200, "detail":"medium-q5_0" }
{ "type":"task.state",    "task_id":"...", "from":"running", "to":"failed",
  "error":{"code":"ENGINE_OOM","message":"...","hint":"建议降档到 small"} }
{ "type":"download.progress","item_id":"...", "bytes":..., "total":..., "speed_bps":... }
{ "type":"resource.vram", "used_mb":8192, "total_mb":12288, "holders":["whisper:medium"] }
```

**约定**：进度事件频率上限 5 Hz（引擎原生更高频输出在 L3 侧节流），避免 WebView 事件队列堆积。

## 5. 引擎协议（L3 ↔ L2）

统一 JSONL：**每行一个 JSON 对象**，stdout 只放协议消息，stderr 放人类日志（同时被采集到 `runs/*.jsonl`）。

请求（stdin，单行）：
```jsonc
{"id":"r1","cmd":"transcribe","input":"C:/.../vocals16k.wav",
 "params":{"model":"ggml-medium-q5_0.bin","language":"zh","threads":8,
           "max_context":-1,"vad":true,"beam_size":5,"gpu_layers":99},
 "out_dir":"C:/.../text"}
```

响应（stdout）：
```jsonc
{"id":"r1","ev":"ready","engine":"whisper.cpp","version":"1.7.4","backend":"cuda12"}
{"id":"r1","ev":"progress","stage":"decode","pct":42,"segments_done":128}
{"id":"r1","ev":"segment","idx":128,"start_ms":612340,"end_ms":616100,
 "text":"……","confidence":0.91}
{"id":"r1","ev":"log","level":"warn","msg":"skipped low-conf segment 2"}
{"id":"r1","ev":"done","ok":true,"outputs":["C:/.../text/transcript.json"],"elapsed_ms":182400}
{"id":"r1","ev":"error","code":"OOM","message":"CUDA out of memory","retryable":true}
```

**契约要求**
- 引擎必须实现 `{"cmd":"probe"}`（返回版本/后端/可用显存）与 `{"cmd":"cancel"}`（协作式中断，≤2 s 响应）。
- 未知 `ev` 必须被 L3 忽略而非报错（向前兼容）。
- 引擎输出非法 JSON 行时，L3 记录并继续（不因一行脏数据杀任务）。

## 6. 目录布局（仓库）

```
cmd/stemscribe/          main.go · app.go（Wails 绑定）· mcp.go · cli.go
internal/
  config/                工作目录解析、settings schema 迁移
  registry/              内置 catalog 加载与校验
  downloader/            分片/续传/sha256/原子落盘/镜像
  engine/                protocol · supervisor · whispercpp · separator · ffmpeg
  pipeline/              阶段实现 + DAG + 状态机 + checkpoint
  resource/              nvml 探测 · 预算 · LRU · 降级
  provider/              openai_compat · anthropic · capabilities · prompt
  vault/                 DPAPI
  store/                 bundle · sqlite · export
  mcpserver/             MCP tools
frontend/                Next.js 15 App Router（static export）
build/                   wails.json · windows/*.manifest · icons/
docs/                    ARCHITECTURE / PIPELINE / … / schema/
tasks/CHG-*/             变更契约
scripts/                 dev.ps1 · smoke-test.ps1 · verify-contract.sh · redact-scan.sh
.github/workflows/       ci.yml · release.yml
```

## 7. 错误模型

统一错误对象（L2→L3→L4 全程透传，不吞不改写）：

```jsonc
{ "code":"ENGINE_OOM",          // 稳定枚举，UI 可据此给建议
  "message":"CUDA out of memory while loading medium-q5_0",
  "hint":"显存不足：建议降档到 small 或关闭其它占用显存的程序",
  "retryable":true,
  "stage":"transcribe",
  "context":{"model":"medium-q5_0","required_mb":6800,"free_mb":2100} }
```

**错误码分层**：`DL_*`（下载）· `FS_*`（文件）· `ENGINE_*`（引擎）· `LLM_*`（润色）· `CFG_*`（配置）· `HW_*`（硬件）。

**重试策略**：仅 `retryable=true` 自动重试，指数退避 1s/4s/12s，最多 3 次；`ENGINE_OOM` 不原样重试，而是先降档再重试一次。

## 8. 状态机（任务）

```
queued → preparing → downloading → separating → transcribing
      → aligning → polishing → exporting → done
              ↘ failed（任意阶段）
              ↘ canceled（任意阶段）
done/failed/canceled → (retry) → queued（从最近 checkpoint 续）
```

- `polishing` 在未开润色时直接跳过（不入状态机，记 `skipped`）。
- `aligning` 负责把分离/转写分段边界对齐、合并被切断的句子（见 `PIPELINE.md` §4）。
- 每个状态转移都必须落 `runs/<run_id>.jsonl` 一行（可审计）。

## 9. 安全边界

| 面 | 措施 |
|---|---|
| 网络 | 主进程只出站到 registry 声明的下载域名 + 用户配置的 LLM endpoint；无本地监听端口（MCP http 模式除外，仅 127.0.0.1 + token） |
| 文件 | 引擎进程只能拿到 workdir 内的路径；下载文件名白名单校验（防 `../` 穿越） |
| 密钥 | DPAPI 加密，日志层脱敏，禁止出现在 bundle 与崩溃报告 |
| 进程 | Job Object 绑定，防孤儿；引擎二进制启动前校验 sha256 |
| 供应链 | 所有下载物 sha256 强校验；release 产物附 `SHA256SUMS.txt`；CI 用 pinned action 版本 |
