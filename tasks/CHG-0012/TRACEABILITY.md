# CHG-0012 · TRACEABILITY

## 1. 需求 → AC → 验证

| 需求 | 来源 | AC | 验证 | 落点 | 里程碑 |
|---|---|---|---|---|---|
| F-01 多文件录入 | U-1/U-2 | AC-01 | 截图 + curl | `frontend/app/components/NewTask.tsx` | M4 |
| F-02 串行自动衔接 | U-3 | AC-02 | curl 轮询观察两次 `running` 切换 | `internal/runctl/queue.go` | M1 |
| F-03 失败不阻塞 | U-4 | AC-03 | 队中放一个不存在的文件 | 同上 | M1 |
| F-04 原生浏览/拖入 | U-2/U-1 | AC-04 | 窗口手工 | `cmd/stemscribe-gui/main_gui.go` | M3 |
| F-05 可移除未开始项 | U-5 | AC-05 | `DELETE /api/queue/{id}` | 同上 | M2 |
| F-06 刷新可重连 | U-6 | AC-06 | 刷新页面 | `internal/runctl/queue.go` | M2 |
| F-07 双语双主题 | — | AC-07 | `i18n-check.sh` + 截图 | `frontend/lib/i18n.ts` | M4 |

## 2. 里程碑状态

| 里程碑 | 状态 | 证据 |
|---|---|---|
| M1 `runctl.Queue` | ✅ | `internal/runctl/queue.go`：串行 worker + 状态镜像（进度取自 `Service`，不自记账） |
| M2 webapi 三端点 | ✅ | `GET/POST /api/queue`、`DELETE /api/queue/{id}`；实测 `{"added":2,"rejected":["C:/Temp/notmedia.txt"],"total":2}` |
| M3 原生浏览 + 拖入 | ⚠️ 部分 | `PickFiles`（`OpenMultipleFilesDialog`）与 `OpenFolder` 已绑定且 `-tags gui` 编译通过；**拖入（`OnFileDrop`）未做** —— Wails v2 各版本字段名不一致，本机无法验证，不冒险写 |
| M4 新建任务页多文件化 | ✅ | 多行路径 + 浏览按钮 + 队列表（截图 `docs/images/app-queue.png`） |
| M5 真排队 2 个 + 截图 | ✅ | 实测 `done 2/2`；产物落 `C:/Temp/out1/local-<hash>/` |

## 3. 实测数据回填（禁止编造）

| 项 | 值 | 来源 |
|---|---|---|
| 队列串行 | `done 2/2`（test.mp4、tts.wav） | `GET /api/queue` 轮询 |
| 非媒体文件拒收 | `C:/Temp/notmedia.txt` → `rejected` | `POST /api/queue` 响应 |
| 相邻任务衔接 | 未单独测（本次两个 bundle 已存在，均全阶段复用，耗时极短） | — |

## 4. 缺陷记录（含本轮新发现）

| # | 缺陷 | 根因 | 状态 |
|---|---|---|---|
| 1 | 录入端只能单文件、无队列 | CHG-0010 刻意不做队列 | 已修 |
| 2 | 拖入未实现 | Wails `OnFileDrop` 字段随版本变化，本机无法验证 | **未交付**（不冒险） |
| 3 | 「衔接空档」未单独测量 | 两个测试 bundle 已存在，全程复用，测不出真实推理衔接 | **待 owner E2E** |

## 5. 未覆盖

- 并发执行、暂停/恢复、浏览器上传：明确不做（SPEC §2）。
