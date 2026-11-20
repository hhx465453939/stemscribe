# CHG-0012 · Multi-file Intake & Run Queue

| 字段 | 值 |
|---|---|
| status | **approved** |
| owner | 本会话（owner 已授权全自动开发/测试/提交/推送） |
| 依赖 | CHG-0010（运行控制 + 进度，已闭环）、CHG-0011（安装，已闭环） |
| 关联 ADR | ADR-002（Go 只编排）、ADR-004（不许静默降级）、ADR-022（单一 handler）、ADR-023（进度不得伪造）、ADR-027（串行队列，不并发跑推理） |

## 1. 背景与问题

「新建任务」只有一个输入框：一次只能说一个文件，说完就得等它跑完再说下一个。
实际使用是**一次处理好几个视频**，于是用户被迫守着一个接一个地手动点击。

同时 CHG-0010 的 `Service` 是单运行设计（一次一个），没有排队层——
这本来是刻意的（避免两个写入者抢同一个 bundle），但现在变成了**使用体验的瓶颈**。

## 2. 范围与不做

**做**

- 录入端支持**多文件**：原生窗口可「浏览多个文件」与「拖入」，浏览器外壳可手动填路径（兜底）。
- 队列：一次排入 N 个文件，按加入顺序**串行**执行，前一个结束立刻接下一个（丝滑衔接）。
- 每个文件独立状态：`waiting` / `running` / `done` / `failed`，失败不阻塞后面的。
- 队列可移除未开始项；运行中项不可移除（不打断正在跑的推理）。

**不做**

- 不做并发推理（同一 GPU 上并行只会互相抢显存，见 ADR-027）。
- 不做暂停/恢复队列（另行立项）。
- 不做浏览器端上传大文件（几个 GB 走 HTTP 上传是错的解法）。

## 3. 设计

### 3.1 队列在 runctl，不在前端

前端只管「排进去」和「看状态」，串行推进由后端负责。理由：前端轮询一旦断掉（刷新页面、关窗口），
前端驱动的队列就会停在半路；后端驱动的队列不受影响，这也是 AC-06 可重连的前提。

```
POST /api/queue {inputs:[...]} ──► Queue.Enqueue ──► 单个 worker goroutine
                                          │              │ 逐个调用 Service.Start 并等待完成
GET  /api/queue ◄────────────────────────┴── {items[], current, done, total}
```

### 3.2 状态由真实运行驱动

每项的 `state` 来自它自己的 `runctl.Status`，不是队列自己记的账：
队列只保留「顺序 + 该跑哪个」，进度与结果仍由 `Service` 提供（ADR-023 同源）。

### 3.3 路径从哪来

| 外壳 | 浏览多个 | 拖入 | 说明 |
|---|---|---|---|
| 原生窗口（Wails） | `runtime.OpenMultipleFilesDialog` | `OnFileDrop` 回调（绝对路径） | 唯一能拿到真实路径的方式 |
| 浏览器外壳 | 不支持（安全限制） | 不支持 | 提供手动填路径输入框，并显式标注原因 |

> 浏览器拿不到绝对路径是平台安全策略，不是实现偷懒。假装支持（例如上传字节）会让几个 GB
> 的视频走 HTTP 复制一遍，比不支持更糟。

## 4. 契约

```json
POST /api/queue  {"inputs":["C:/a.mp4","C:/b.mkv"]}   → 202 {"added":2,"total":2}
GET  /api/queue  → 200
{
  "items":[
    {"id":"q-1","input":"C:/a.mp4","state":"running","percent":42,"segments":0,"message":""},
    {"id":"q-2","input":"C:/b.mkv","state":"waiting","percent":0,"segments":0,"message":""}
  ],
  "current":"q-1","done":0,"total":2
}
DELETE /api/queue/{id}  → 204（仅 waiting 项可删；running 项返回 409）
```

## 5. 里程碑

| 里程碑 | 内容 | 文件 | 变绿 AC |
|---|---|---|---|
| M1 | `runctl.Queue`（串行 + 状态镜像） | `internal/runctl/queue.go` | AC-02/AC-03/AC-05 |
| M2 | webapi 三端点 | `internal/webapi/handler.go` | AC-01/AC-06 |
| M3 | 原生浏览 + 拖入绑定 | `cmd/stemscribe-gui/main_gui.go` | AC-04 |
| M4 | 新建任务页：多文件列表 + 排队 + 逐项进度 | `frontend/app/components/NewTask.tsx`、`frontend/lib/{api,i18n}.ts` | AC-01/AC-07 |
| M5 | 真排队 2 个文件 + 截图 | `docs/images/app-queue.png` | AC-02/AC-03 |

**DoD**：AC-01…AC-07 全绿；`go build/vet/test` + `typecheck`/`lint`/`i18n-check` 全绿；
真排队两个视频文件，第二个在第一个结束后自动开始，且两集产物都在。

## 6. 验收标准

| AC | 判据 | 验证方式 |
|---|---|---|
| AC-01 | 可一次加入多个文件（多选/拖入/手动多行），列表按加入顺序显示 | 截图 + curl |
| AC-02 | 串行执行：前一个 `done` 后，下一个自动进入 `running`，无需人工点击 | curl 轮询观察 |
| AC-03 | 任一失败不阻塞后续：失败项标记 `failed`，后面的继续跑 | 用一个不存在的文件排在中间 |
| AC-04 | 原生窗口可「浏览多个文件」和「拖入」，路径为绝对路径 | 手工（窗口） |
| AC-05 | 排队可移除 `waiting` 项；`running` 项拒绝移除 | curl DELETE |
| AC-06 | 刷新页面/重开窗口能接回队列现状（后端持有队列） | 刷新后截图 |
| AC-07 | 双语、双主题、无硬编码 CJK | `i18n-check.sh` + 截图 |

## 7. 风险与对策

| 风险 | 对策 |
|---|---|
| 队列与 `Service` 双份状态不一致 | 队列不自己记进度，只记顺序与「谁在跑」，其余取自 `Service.Status()` |
| 刷新页面导致队列丢失 | 队列在后端；`GET /api/queue` 返回全量快照 |
| 一个坏文件卡死整队 | 失败立即落 `failed`，worker 继续下一项（AC-03） |
| 拖入无扩展名/非视频文件 | 加入时校验扩展名，非媒体文件直接拒绝并说明 |
| 浏览器用户以为能拖 | 界面显式说明「拖入/浏览需在桌面窗口中使用」 |

## 8. 依赖与外部条件

- Wails v2 的 `runtime.OpenMultipleFilesDialog` 与 `OnFileDrop`（v2 内置，无新依赖）。
- 本机已有 whisper 运行时与模型，可真实排队两个视频验证。
- 无新增第三方依赖。

## 9. 独占文件清单

| 范围 | 归属 |
|---|---|
| `internal/runctl/queue.go`（新建） | CHG-0012 |
| `internal/webapi/handler.go`（新增 queue 路由） | CHG-0012（与 CHG-0010 交叠，登记） |
| `cmd/stemscribe-gui/main_gui.go`（文件对话框 + 拖入） | CHG-0012 |
| `frontend/app/components/NewTask.tsx`（重写为多文件） | CHG-0012（与 CHG-0010 交叠，登记） |
| `frontend/lib/api.ts`、`frontend/lib/i18n.ts` | CHG-0012 |
| `docs/{PIPELINE,UI-DESIGN}.md`、`tasks/README.md` | CHG-0012 |
| `internal/runctl/service.go` | **只读引用，不改**（队列复用它的 Start/Status） |
