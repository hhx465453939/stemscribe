# CHG-0013 · CHG Log

## 变更摘要

按 `docs/UX-AUDIT.md` 修 P0：参数真正生效、输出目录可选、多文件接线、打开产物目录、单任务选模型。
本 CHG 的规约是「只修审计表里的 🔴，不加新功能」。

## 关键决策

### ADR-029 · 产物目录可配，但结构不变

`Settings.OutputDir` 为空时维持 `workdir/libraries/<videoID>`；设置后为 `<OutputDir>/<videoID>/`。
不变的是**一个视频一个自包含目录**：bundle 里有 audio/text/meta.json，可整体搬走或备份。
不采用「所有产物倒进一个扁平目录」，那样同名文件会互相覆盖，且失去可搬性。

### ADR-030 · 参数的唯一来源

新建任务提交参数时读参数页写的同一份 `localStorage` 数据，不做第二份默认值。
参数键与 `engine.Specs()` 的 key 一致，后端 `ParamsFromMap` 负责校验并**拒绝未知键**——
宁可报错，也不要静默丢弃用户改过的项。

## 影响面

- `internal/config`：新增 `OutputDir`（此前无 CHG 认领，本次登记）。
- `internal/runctl/run.go`：产物落点。
- `internal/webapi`：settings 与 queue 路由（queue 属 CHG-0012 的 M2，本次补齐）。
- `cmd/stemscribe-gui`：文件对话框、拖入、打开目录。
- `frontend`：新建任务页重写。

## 验证策略

1. 曲线验证：`PUT /api/settings` → `POST /api/queue` 两个文件 → 轮询确认串行与落点。
2. 产物校验：读 `meta.json.params_snapshot` 证明参数生效。
3. 界面：截图 + `i18n-check`。
4. 原生对话框/拖入需在窗口内手工验证（owner E2E）。
