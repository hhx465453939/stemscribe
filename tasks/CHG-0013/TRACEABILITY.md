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
| UX 审计 | ✅ | `docs/UX-AUDIT.md`（按旅程逐项，含痛感分级） |
| AC-01 参数生效 | ⬜ | — |
| AC-02 输出目录 | ⬜ | — |
| AC-03 多文件串行 | ⬜ | — |
| AC-04 打开产物 | ⬜ | — |
| AC-05 选模型 | ⬜ | — |
| AC-06 双语 | ⬜ | — |

## 3. 未覆盖（来自审计表，不假装解决）

P1：取消运行、卸载模型；P2：资料库单集查看、系统通知、错误日志入口、首启动引导；
P3：删除/重跑、快捷键。
