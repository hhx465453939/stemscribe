# CHG-0019 · TRACEABILITY — 判据 → 实现 → 测试 → 证据

| AC | 实现 | 测试 | 证据 |
|---|---|---|---|
| AC-01 | `frontend/app/components/NewTask.tsx`（`sepModels = catalog.installed.filter(kind === "separator")` 渲染「分离模型」下拉） | 前端类型检查；`contrast.py` 双主题对比 | `tmp/evidence/chg0019/M1-STAGE1-UI.md` 判据 1；`stage1-dark.png` / `stage1-light.png` |
| AC-02 | 同一组件，选项文本拼 `display_name · architecture · N stems · 非商用`；数据来自 registry（ADR-019） | 无前端单测，靠页面实测 | `M1-STAGE1-UI.md` 判据 2（列出的四个选项原文） |
| AC-03 | `NewTask.tsx` `changeSeparator()` 重置 `stems` 并按新模型 stems 重渲染 | 页面实测：UVR5 2 项 ↔ `demucs-htdemucs-6s` 6 项 | `M1-STAGE1-UI.md` 判据 3（切换与恢复记录） |
| AC-04 | `span.stem-lock` + `--lamp-fill/--lamp-ink`（`frontend/app/globals.css`）；`aria-label` 用 `task.stem.locked` | 双主题截图核对 | `M1-STAGE1-UI.md` 判据 4/6；两张截图 |
| AC-05 | `internal/pipeline/pipeline.go` `pruneStems()`；`stems` 事件 `kept=/dropped=` | `pipeline_test.go` `TestSeparateDropsUnselectedStems` | `e2e-stems.log`：`kept=vocals dropped=bass,drums,guitar,other,piano` |
| AC-06 | `internal/runctl/run.go` `separationHash(..., keep)`；`stageParams["separate"]` 使用该哈希 | `run_test.go` `TestSeparationHashTracksRetainedStems` | `e2e-stems.log`：`resume: separate: parameters changed` |
| AC-07 | `recordSeparateRuntime()`：候选集 → `os.Stat` → 排序写 `meta.provenance.engines.separate.stems` | 由真实运行核对 meta | `local-570254dd1e1af6d9/meta.json` 的 `stems: ["vocals"]` |
| AC-08 | `run.go` `dropStaleAudio()`（分离启用时清理非 live wav；保护 `full/16k/vocals16k`；live 未知则不删）；`runOnce` 成功路径调用并 emit `removed stale=` | `run_test.go` `TestDropStaleAudioRemovesPreviousModelStems`（含未知模型不删的护栏） | `e2e-stale-sweep.log`：`separate skipped (already valid)` 后 `separate removed stale=other`，派生文件仍在 |
| AC-09 | `run.go` `normalizeStems()`（去重排序、必须含 vocals、未知 stem 报错并列出可选值）；`runOnce` 入队前调用 | `run_test.go` `TestNormalizeStems` | 单测输出 |
| AC-10 | `frontend/lib/i18n.ts`：`task.model = 转写模型 / Transcription model`，`task.separator = 分离模型 / Separation model` | `pnpm typecheck`（`Record<Key,string>` 强制齐平） | 截图同屏两个下拉 |
| AC-11 | `i18n.ts` zh 为 `as const`，en 为 `Record<Key,string>` | `pnpm --dir frontend run typecheck` | Checkfix 记录 |
| AC-12 | —— | —— | `M1-STAGE1-UI.md` + `e2e-stems.log` + `e2e-stale-sweep.log` + 双主题截图 |

## 链路完整性（入口/出口）

- 入口：`POST /api/queue`（`internal/webapi/handler.go` 解析 `separator`、`stems`）与
  CLI `-separator` / `-stems`（`cmd/stemscribe/cli.go`）。
- 出口：`meta.json` 的 `provenance.engines.separate.stems`、`audio/` 实际文件、
  `stems` 事件（CLI 打印 `· separate kept=… dropped=…` / `removed stale=…`）。

## 未覆盖 / 已知限制

1. 前端只有类型检查与页面实测，没有组件级单测（项目当前无 React 测试栈）。
2. 历史 bundle 的孤儿清理只在下一次以相同输入运行且分离启用时发生；
   不做启动期全库扫描（见 SPEC §4）。
3. `E:\VideoDownload\怪物般的作品\local-570254dd1e1af6d9`（上一轮 Demucs 实验产物）
   仍保留旧 UVR5 的 `instrumental.wav`；以该输入再跑一次即会被清理。
   未直接删除该目录，避免误删用户输出目录中的运行数据。
