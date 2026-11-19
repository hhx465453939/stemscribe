# CHG-0010 · TRACEABILITY

## 1. 需求 → AC → 验证

| 需求 | 来源 | AC | 验证 | 落点 | 里程碑 |
|---|---|---|---|---|---|
| F-01 点击即反馈 | U-1 | AC-01 | 截图 + 手工点击 | `frontend/app/components/NewTask.tsx`、`lib/api.ts` | M4 |
| F-02 真实进度 | U-2 | AC-02 | `runctl` 单测 + 手工 | `internal/runctl/**` | M2 |
| F-03 失败可解释 | U-3 | AC-03 | 用不存在的模型跑 | `internal/runctl/status.go`、前端 | M2/M4 |
| F-04 完成可交付 | U-4 | AC-04 | 真实视频跑到 done | 同上 | M4 |
| F-05 界面/窗口同源 | U-6 | AC-05 | 代码结构 + curl + 窗口双验 | `internal/webapi/**`、`shell.go`、`main_gui.go` | M3 |
| F-06 双语双主题 | — | AC-06 | `i18n-check.sh` + 截图 | `frontend/lib/i18n.ts` | M4 |
| F-07 状态可重连 | U-5 | AC-07 | `GET /api/runs/latest` | `internal/runctl/service.go` | M2 |
| F-08 构建同步 | — | AC-08 | 时间戳比对 + 启动 | `scripts/build.ps1` | M5 |
| F-09 CLI 不退化 | U-6 | AC-05 | CLI 输出逐字比对 + e2e | `cmd/stemscribe/cli.go` | M1 |

## 2. 里程碑状态

| 里程碑 | 状态 | 证据 |
|---|---|---|
| M1 抽取 `runctl.Run`，CLI 改调用 | ✅ | `cmd/stemscribe/wire.go`、`synthetic.go` 删除；`cli.go` 只留 flags/打印/退出码；真视频 e2e 仍产字（`segments: 2`） |
| M2 异步 `Service` + `Status` | ✅ | 单运行互斥、进度由真实事件推导；复跑时为全 `stage.skip` 故 `已用 0 秒` 属实，未伪造 |
| M3 `webapi` + shell/Wails 双挂载 | ✅ | `webapi.New(svc, fallback)` 同时挂 `webui.ServeHandler` 与 Wails `AssetServer.Handler`；curl 实测 `POST /api/runs` → `{"id":"r-1"}` |
| M4 前端进度区 + i18n | ✅ | 截图 `docs/images/app-progress.png`：`已完成 · test.mp4`、`7/7 个阶段`、`2 段逐字稿`、产物目录 + 复制路径 |
| M5 `dist/` 同步 + 真跑 + 截图 | ✅ | `scripts/build.ps1 -Version 0.2.0` 产出 `dist/stemscribe-cli.exe`(8.55 MB) 与 `dist/Stemscribe.exe`(12.19 MB) |

## 3. 实测数据回填（禁止编造）

| 项 | 值 | 来源 |
|---|---|---|
| 点击到首帧反馈 | ≤500 ms（`POST` 立即返回 `{id}`，页面随即轮询） | `curl` 实测响应即返 |
| 复用运行的实际耗时 | <1 s（全部阶段 `stage.skip`） | `started_at` = `ended_at` = `05:06:43Z` |
| 真实转写运行 | `segments: 2`，`7/7` 阶段，`100%` | `/api/runs/latest` |
| 轮询间隔 | 500 ms | `frontend/app/components/NewTask.tsx` |
| 二进制体积 | CLI 8.55 MB / GUI 12.19 MB | `scripts/build.ps1` 输出 |

## 4. 缺陷记录

| # | 缺陷 | 根因 | 状态 |
|---|---|---|---|
| 1 | 「开始」按钮无 `onClick`，点击零反应 | 前端从未接运行控制面 | 本 CHG 修 |
| 2 | GUI 仅有 `Version()` 绑定 | 窗口层未暴露任何运行入口 | 本 CHG 修 |
| 3 | `CHG-0005 Frontend` 一直 `planned`，运行控制面无人认领 | 立项缺口 | 本 CHG 认领控制面部分 |
| 4 | 进度报 `3/7` 却 `100%` | 完成数只统计 `stage.end`，未把「已被 checkpoint 判定有效」的 skip 计入 | 已修 |
| 5 | 进度报 `9/7` | 修正后把「被设置禁用」的 skip 也计入，而分母已扣除它们 | 已修（用 `disabledNote` 区分两种 skip） |
| 6 | 首次点击必失败（`missing model: whisper-medium`） | 模型解析只看设置、不看**已安装**集合 | 已修：未装时选已装最优模型并 `MODEL_SUBSTITUTED` 事件说明 |

## 6. 已知遗留（非本 CHG 范围，不隐藏）

- `frontend/app/components/TaskBoard.tsx` 仍是写死的示例任务（「示例：42 分钟行业访谈」等）。
  这属于**向用户展示虚构数据**，与项目「不许编造」同源，但修它需要列出真实 bundle 列表
  （新端点 + 读取 `libraries/*/meta.json`），应另立子项，不在本 CHG 内偷偷改掉。

## 5. 未覆盖（如实标注）

- 暂停/取消按钮的交互细节：只接通 `context` 取消，UI 控件留待后续 CHG。
- 多任务并行：明确不做。
