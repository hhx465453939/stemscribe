# CHG-0019 · CHG Log

## 变更摘要

owner 指出新建任务页只暴露了转写模型，阶段 1（音轨分离）既不能选模型，
也不能选输出音轨。本 CHG 把阶段 1 做成任务级可选：分离模型下拉（registry 已安装
separator，含 Demucs 全家族）、输出音轨多选（跟随模型声明、人声锁定）、
未勾选轨在分离后删除且可审计；同时修复端到端测试暴露的「换模型后旧轨残留」。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-27 | proposed | owner 提出需求（原话见 PRD §1） | owner |
| 2026-11-27 | in_progress | 前端下拉/多选/锁定标记 + `/api/queue` + runctl/pipeline 保留集链路落地 | agent |
| 2026-11-27 | in_progress | 双主题截图核对；i18n 齐平；hash 含 keep；单测补齐 | agent |
| 2026-11-27 | in_progress | 真实端到端（Demucs 6s → 只留 vocals）暴露旧 UVR5 的 `instrumental.wav` 残留，登记为缺陷 | agent |
| 2026-11-27 | in_progress | 新增 `dropStaleAudio`：分离启用时清理非 live wav（保护派生文件、未知模型不删）；单测 + 真实 resume-skip 运行验证 | agent |
| 2026-11-27 | in_progress | 补齐 PRD/SPEC/TRACEABILITY 与 E2E 证据；待 Checkfix 与提交授权 | agent |

## 设计决定

1. **保留集在应用层裁剪**：上游 audio-separator 没有单轨模式，分离永远算全部轨；
   删除是磁盘决策，必须发生在 checkpoint 记录 outputs 之前，否则下次 plan 会找已删文件。
2. **keep 进入分离哈希**：空集合记为 `all`，非空排序后拼接。界面/磁盘不一致的状态不可能通过 resume 出现。
3. **人声不是控件而是事实**：用 `span.stem-lock` 而不是 disabled checkbox，避免暗色主题下
   被画成「未勾选」的相反语义。
4. **旧轨清理放在 runctl 而非 pipeline**：它需要 registry 的模型声明与 bundle 路径；
   pipeline 保持只依赖本次运行产物。清理条件是「分离启用」，包含 resume 判定 already valid 的情况。

## 改动文件

| 文件 | 改动 |
|---|---|
| `frontend/app/components/NewTask.tsx` | 分离模型下拉、音轨多选、`changeSeparator` 重置、提交 `separator`/`stems` |
| `frontend/app/globals.css` | `.stem-lock` 实心琥珀勾 |
| `frontend/lib/api.ts` | `enqueue()` 增加 `separator`、`stems`；`SettingsView.separator` |
| `frontend/lib/i18n.ts` | `task.separator`、`task.stems`、`task.stemsHint`、`task.stem.*`、`task.stem.locked`；`task.model` 改「转写模型」 |
| `internal/webapi/handler.go` | `/api/queue` 接收 `separator`、`stems` |
| `internal/runctl/run.go` | `Options.ExportStems`、`normalizeStems` 预校验、hash 含 keep、`recordSeparateRuntime` 按 stat 记录、新增 `dropStaleAudio` |
| `internal/pipeline/pipeline.go` | `SeparateOptions.Keep`、`pruneStems`、`stems` 事件 |
| `cmd/stemscribe/cli.go` | `-stems` 旗标、`splitCSV`、打印 `stems` 事件 |
| `internal/runctl/run_test.go` | `TestNormalizeStems`、`TestSeparationHashTracksRetainedStems`、`TestDropStaleAudioRemovesPreviousModelStems` |
| `internal/pipeline/pipeline_test.go` | `TestSeparateDropsUnselectedStems` |

## 验证

- 单测：`go test ./internal/runctl/ ./internal/pipeline/` 通过。
- 真实运行：`tmp/evidence/chg0019/e2e-stems.log`（Demucs 6s → vocals）、
  `e2e-stale-sweep.log`（resume 跳过分离仍清孤儿）、`e2e-restore.log`（误删后的恢复）。
- UI：`tmp/evidence/chg0019/M1-STAGE1-UI.md` + 双主题截图。
- 详表：`TRACEABILITY.md`。

## 遗留

- `E:\VideoDownload\怪物般的作品\local-570254dd1e1af6d9` 仍带旧 UVR5 的
  `instrumental.wav`（修复前状态，未擅自删除用户输出目录里的运行数据）；
  以该输入再跑一次即清理。
- 未做启动期全库扫描（见 SPEC §4）。
- commit 与 push 分开：本轮只提交，push 待 owner 明确授权。
