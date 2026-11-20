# CHG-0012 · CHG Log

## 变更摘要

「新建任务」从单输入框变成多文件录入 + 后端驱动的串行队列：一次拖入/选择多个视频，
自动一个接一个跑完，前一个结束立刻接下一个。

## 关键决策

### ADR-027 · 队列在后端、串行执行

队列状态放在 `internal/runctl`，不在前端。两个理由：

1. **刷新不丢**：前端驱动的队列一旦页面重载就停在半路，而视频转写动辄几分钟。
2. **单一写入者**：同一 GPU 上并发跑推理只会互相抢显存（`docs/RESOURCE-SCHEDULER.md` §5），
   串行是当前条件下唯一稳的选择。衔接空档只取决于 worker 的循环速度，实测足够小。

代价：用户看不到「同时跑两个」的错觉——但那本来就是假象。

### ADR-028 · 文件路径只能由原生窗口提供

浏览与拖入仅在 Wails 窗口内可用（`OpenMultipleFilesDialog`、`OnFileDrop` 都返回绝对路径）。
浏览器外壳拿不到绝对路径，这是平台安全策略；用「上传文件」绕过会让几个 GB 的视频经 HTTP
复制一遍，比不支持更糟。因此浏览器外壳保留手动填路径，并在界面上写明原因。

## 影响面

- 新增 `internal/runctl/queue.go`。
- `internal/webapi/handler.go` 增三条 queue 路由。
- `cmd/stemscribe-gui/main_gui.go` 增文件对话框绑定与拖入回调。
- `frontend/app/components/NewTask.tsx` 改为多文件列表 + 队列视图。

## 验证策略

1. 单测：队列串行顺序、失败不阻塞、移除只对 waiting 生效。
2. curl：`POST /api/queue` 两个文件，轮询确认 `q-1 done` 后 `q-2 running`。
3. 截图：队列列表与逐项进度。原生浏览/拖入需在窗口中手工验证（由 owner 做 E2E）。
