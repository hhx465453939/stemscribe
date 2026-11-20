# CHG-0013 · TRACEABILITY

## 1. 审计项 → AC → 验证

| 审计项 | AC | 验证 | 落点 |
|---|---|---|---|
| A6 参数不生效 | AC-01 | 请求体 + `meta.json.params_snapshot` | `frontend/app/components/NewTask.tsx` |
| A4 输出目录 | AC-02 | 真跑后 `ls` | `internal/config/config.go`、`internal/runctl/run.go` |
| B1 多文件 | AC-03 | `curl /api/queue` 轮询 | `internal/webapi/handler.go`、`frontend` |
| A9/E3 打开产物 | AC-04 | 截图 + 手工 | `cmd/stemscribe-gui/main_gui.go` |
| A5 选模型 | AC-05 | `meta.json` | `frontend`、`internal/webapi` |
| — | AC-06 | `i18n-check.sh` | `frontend/lib/i18n.ts` |

## 2. 状态

| 项 | 状态 | 证据 |
|---|---|---|
| UX 审计 | ✅ | `docs/UX-AUDIT.md`（按旅程逐项，含痛感分级与 P0-P3 排序） |
| AC-01 参数生效 | ✅ | 真跑后 `meta.json` → `params_snapshot: {"beam_size":"3"}`；此前是 `{}`（假承诺） |
| AC-02 输出目录 | ✅ | `PUT /api/settings {output_dir:"C:/Temp/out1"}` → 产物落 `C:/Temp/out1/local-<hash>/`（两个 bundle 均在） |
| AC-03 多文件串行 | ✅ | `POST /api/queue` 两文件 → `done 2/2` |
| AC-04 打开产物 | ⚠️ 部分 | `PickFiles`/`OpenFolder` 绑定已加且 GUI 编译通过；**拖入未做**；浏览器回退为复制路径 |
| AC-05 选模型 | ⚠️ 部分 | 下拉只列**已安装** whisper 模型（正确防坑）；未单独做一次「选完再跑」的对比验证 |
| AC-06 双语 | ✅ | `i18n-check` 通过、零硬编码 CJK |

## 2.1 本轮截图发现的新问题（未修，如实登记）

| # | 现象 | 根因猜测 | 优先级 |
|---|---|---|---|
| 1 | 「输出目录」标签被挤成竖排（`输出目`/`录`） | 该 `<label>` 在 flex 行内无 `flex-shrink:0`，被输入框挤压 | P2（难看但可用） |
| 2 | 模型下拉显示 `—` | `settings.stages.whisper` 为 `whisper-medium`（未安装），下拉只列已装项 → 无匹配 | P1（正确行为但易误导，应提示“当前设置未安装”） |
| 3 | 「任务」区（TaskBoard）与「队列」区信息重叠 | 两者都列已完成的 bundle，来源不同（`/api/library` vs `/api/queue`） | P2（待收敛为一个视图） |

## 3. 未覆盖（来自审计表，不假装解决）

P1：取消运行、卸载模型；P2：资料库单集查看、系统通知、错误日志入口、首启动引导；
P3：删除/重跑、快捷键。
