# CHG-0013 · UX Essentials（审计 P0）

| 字段 | 值 |
|---|---|
| status | **approved** |
| 依据 | `docs/UX-AUDIT.md`（本 CHG 的输入，先审计后开发） |
| 依赖 | CHG-0010（运行控制）、CHG-0011（安装）、CHG-0012（队列后端） |
| 关联 ADR | ADR-004、ADR-022、ADR-027 |

## 1. 背景

`docs/UX-AUDIT.md` 按用户旅程走了一遍，结论是：**后端能力普遍超前于界面**，
且参数页是**假承诺**（界面写「改过的项会写进产物元数据」，实际 `NewTask` 提交 `params: {}`）。

本 CHG 只修审计表里的 🔴 P0，不顺手加新功能。

## 2. 范围

| 审计项 | 修法 |
|---|---|
| A6 参数不生效 | 前端把参数页的 localStorage 值随请求发送；参数页文案改为真实行为 |
| A4 输出目录不能选 | `Settings.OutputDir`；运行时若设置则产物落该目录下 `<videoID>/` |
| A3 只能手打路径 | 原生窗口：`OpenMultipleFilesDialog` + `OnFileDrop`；浏览器：多行路径输入兜底 |
| B1 多文件未接线 | 前端接 `/api/queue`（队列后端 CHG-0012 已就绪；本次补 webapi 路由） |
| A9/E3 找不到产物 | `OpenFolder` 绑定 + 界面按钮；浏览器回退为「复制路径」 |
| A5 不能选模型 | 新建任务页从 `/api/models` 的 installed 列表选模型 |

**不做**：卸载模型、资料库单集查看、系统通知、首启动引导（P1/P2，留在审计表）。

## 3. 设计

- **输出目录**：`OutputDir` 为空 = 现状（`workdir/libraries/<videoID>`）；非空 = `<OutputDir>/<videoID>/`。
  仍然一个视频一个自包含目录，可搬走。不做「把散文件倒进一个目录」——那会破坏 bundle 的可搬性。
- **参数**：前端读 `stemscribe.params`（参数页写的同一 key），转成 `{key: value}` 发给 `/api/runs`。
  后端 `ParamsFromMap` 已支持验证与拒绝未知键。
- **路径来源**：原生窗口用 Wails 对话框与拖入（返回绝对路径）；浏览器用多行文本（每行一个）。
  浏览器拿不到绝对路径是平台限制，界面明说，不假装。

## 4. 契约

```
GET  /api/settings            → {output_dir, model, params_defaults?}
PUT  /api/settings {output_dir, model}
GET  /api/queue               → 队列快照（CHG-0012）
POST /api/queue {"inputs":[], "output_dir"?, "model"?, "params"?} → {added, rejected[]}
DELETE /api/queue/{id}        → 204 / 409（running 不可删）
```

## 5. 验收标准

| AC | 判据 | 验证 |
|---|---|---|
| AC-01 | 参数页改 `beam_size`，新建任务的请求体里带 `beam_size`，且产物 `params_snapshot` 出现该值 | curl + 读 meta.json |
| AC-02 | 设置输出目录后，产物落在该目录下的 `<videoID>/`；改回空则回到 `libraries/` | 真跑 + `ls` |
| AC-03 | 可一次提交多个路径，队列按顺序串行跑完（前一个 done 后下一个自动 running） | curl 轮询 |
| AC-04 | 界面可一键打开产物目录（原生窗口）；浏览器显示复制路径 | 截图 + 手工 |
| AC-05 | 新建任务页可从已安装模型里选模型，选择生效（`meta.json` 的 engine 记录一致） | 真跑 |
| AC-06 | 双语双主题、无硬编码 CJK | `i18n-check.sh` + 截图 |

## 6. 独占文件清单

| 范围 | 归属 |
|---|---|
| `internal/config/config.go`（加 `OutputDir`） | CHG-0013（此前无 CHG 认领，本次登记） |
| `internal/runctl/run.go`（产物落点） | CHG-0013（与 CHG-0008/0010 交叠，登记） |
| `internal/webapi/handler.go`（settings/queue 路由） | CHG-0013（与 CHG-0010/0012 交叠，登记） |
| `cmd/stemscribe-gui/main_gui.go`（对话框/拖入/打开目录） | CHG-0013 |
| `frontend/app/components/NewTask.tsx`、`frontend/lib/{api,i18n}.ts` | CHG-0013 |
| `docs/{UX-AUDIT,UI-DESIGN}.md`、`tasks/README.md` | CHG-0013 |
| `internal/runctl/queue.go` | CHG-0012（本 CHG 只调用，不改） |
