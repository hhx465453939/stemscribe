# CHG-0010 · CHG Log

## 变更摘要

把「用户点开始」真正接到「管线开始跑」，并把「管线跑到哪」接回界面。
在此之前，`NewTask.tsx` 的按钮没有 `onClick`、GUI 只暴露 `Version()`，界面在说谎。

## 关键决策

### ADR-022 · 运行服务单一入口

窗口与浏览器**共用同一个 `http.Handler`**（`internal/webapi`），分别挂到 shell 的 HTTP server 与
Wails `AssetServer.Handler`。理由：两套逻辑必然漂移；同一 handler 让「窗口能用」与「浏览器能用」
成为同一件事，且可以用 curl 验证窗口路径的行为。

代价：Wails 侧要把 `/api/` 与静态资源分流，需要验证不冲突。

### ADR-023 · 进度不得伪造

进度只由 `pipeline` 的真实 `stage.start`/`stage.end` 事件推导，分母为本次**启用**的阶段数。
禁止计时器递增的假进度条。理由：假进度比没有进度更坏——它让用户相信一件没发生的事，
与本项目「不许静默降级」同源。

### ADR-024 · 运行编排下沉到 `internal/runctl`

`cmd/stemscribe/cli.go` 的编排代码移入 `internal/runctl`，CLI 改为调用并把事件打印出来。
理由：编排逻辑若留在 `cmd`，窗口层要么复制一份（必然漂移），要么反向依赖 `cmd`（不可接受）。
约束：**CLI 输出必须逐字不变**，作为抽取是否改变行为的回归判据。

## 影响面

- 新增 `internal/runctl/**`、`internal/webapi/**`。
- `cmd/stemscribe/{cli,shell,wire}.go` 编排迁移（引擎参数语义不变，仍归 CHG-0008）。
- `cmd/stemscribe-gui/**` 增加运行绑定与 API 挂载。
- `frontend/app/components/NewTask.tsx` 增加运行与进度区；`frontend/lib/i18n.ts` 增键。
- `scripts/build.ps1` 产出同步的 `dist/`。

## 不做

见 SPEC §2。明确排除多任务并行、润色/分离真实推理、SSE/WebSocket。

## 验证策略

1. 单测：`runctl` 阶段序列、状态机、错误映射。
2. 结构验证：浏览器与窗口共用 handler（代码检查 + 两处 curl/窗口行为一致）。
3. 真跑：`C:/Temp/test.mp4` 从界面启动跑到 `done`，逐字稿有字。
4. 构建：`scripts/build.ps1` 后 `dist/` 内 exe 与 assets 反映当前源码。
