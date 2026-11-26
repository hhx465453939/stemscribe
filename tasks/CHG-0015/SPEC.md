---
change_id: CHG-0015
version: 2
status: approved
prd_ref: PRD.md
health_ref: docs/UX-AUDIT.md
supersedes: CHG-0015 v1（未批准的“首跑诚实”草案，已被 owner 指令替换）
approved_by: owner（2026-11-26 会话指令：按本计划执行）
approved_at: 2026-11-26
---

# CHG-0015 · Core Pipeline：强制人声分离 + 最大模型 + 拖入即跑

| 字段 | 值 |
|---|---|
| status | **approved**（owner 指令：先写计划，再按计划执行） |
| 目标 | 拖入视频 → 自动抽 vocal（UVR5）→ 自动转录 → 出 txt |
| 依赖 | CHG-0002（引擎/运行时）、CHG-0010（运行控制）、CHG-0011（安装）、CHG-0012（队列）、CHG-0013（输出/参数） |
| 关联 ADR | ADR-002（推理走外置进程）、ADR-004（不许静默降级）、ADR-023（进度不伪造） |

## 1. 背景（owner 的明确要求）

现有链路把“人声分离”当成可选阶段，且分离引擎是空实现（`Whisper.Separate` 返回
`ErrNotImplemented`），运行时里还有“未实现就跳过并继续”的分支。结果是：用户拖进一个带 BGM
的视频，软件直接拿原声去转写，稿子质量差；界面上“分离”开关是开的，但实际没做——这是**假承诺**。

owner 的要求（原话归纳，不得改写）：

1. **抽 vocal 是必做题，不是选择题**：每条链路先跑 UVR5 分离，再拿 vocal 转写。
2. **模型用最大/最好的**：分离用 UVR5 里最好的模型；转写默认用最大的 whisper 模型。
3. **一般主机都能运算**：显存/设备不足时自动降档，但**必须可见**，不许静默。
4. **最简操作**：视频文件（可多个）拖进去 → 自动排队 → 自动分离 → 自动转录 → 出 txt。
5. **基本 UX 必须有**：多文件队列、输出目录、模型、语言等选项。
6. 代码完成后**及时 commit/push**。

## 2. 非目标

VAD、逐字稿查看器、取消运行、卸载模型、LLM 润色、URL 导入：本 CHG 不做，登记到 CHG-0016（未立项）。

## 3. 设计

### 3.1 分离引擎：外置 UVR5（Python）进程

- 新增 `internal/engine/separator.go`：实现 `pipeline.Engine` 的 `Separate`。
- 推理不在 Go 内（ADR-002）：调用 `audio-separator`（UVR5 的推理栈）的 Python 进程。
- Python 运行时：`workdir/runtimes/separator/venv/`（按需安装，不随 40 MB 安装包）。
  解析顺序：`STEMSCRIBE_SEPARATOR_PYTHON` → `workdir/runtimes/separator/venv/Scripts/python.exe`。
- 辅助脚本 `internal/engine/separator_helper.py` 用 `go:embed` 打包，首次运行时落到
  `workdir/runtimes/separator/helper.py`，避免依赖仓库路径。
- 协议：参数 `--input --outdir --model`；成功时 stdout 输出一行 JSON
  `{"vocals":"...","instrumental":"..."}`；失败非零退出并带 stderr，Go 侧 wrap 错误。
- **模型**：registry `family: separator` 新增默认条目；必须是 hf-mirror 绝对 URL，
  `size` 与 `sha256` 实测后回填。候选按质量排序：UVR MDX-NET（Inst HQ 3/4/5、Voc FT）
  → HP2/HP5（VR）。M1 以“能下载、能实测、能跑通”的实际结果定稿，未实测的写 `tbd`，
  **不得编造**。分离模型文件与转写模型分别落 `models/separator/<id>/`。
- 分离输出固定为 `audio/vocals.wav`、`audio/instrumental.wav`；`transcribe` 只读 `vocals.wav`。

### 3.2 强制分离

- `runctl.Run` 不再有 `o.Separate && settings.Stages.Separate` 双重开关；真实引擎一律分离。
- 删除“分离未实现 → 告警后跳过”的分支。运行时/模型缺失时**直接失败**，错误里给出
  “安装分离运行时/模型”的可执行指引（AC-02）。
- UI 不再提供“关闭分离”；文案固定为“自动人声分离（必选）”。
- CLI 的 `--no-separate` 仅保留给合成引擎测试，真实引擎忽略并报错说明（不作为用户选项）。

### 3.3 最大模型与可见降档

- 默认 ASR：`whisper-large-v3`（registry 实测 sha256 后回填，可一键安装）。
- 运行前解析：请求模型未安装 → 用已装的最佳模型，并写 `MODEL_SUBSTITUTED`。
- 显存不足：首次失败若判定为 OOM，自动降一档（large-v3 → medium）并写
  `ENGINE_OOM` + `MODEL_SUBSTITUTED` 事件，UI 任务行可见（AC-03）。
- 不做“按机器猜模型”的复杂矩阵；以“默认最大 + 可见降档”为准，规则简单可审计。

### 3.4 拖入即跑（多文件）

- Wails：`runtime.OnFileDrop` 收到绝对路径 → `EventsEmit("stemscribe:files-dropped", paths)`。
- 前端：监听事件 → 把路径加入列表 → **立即** `POST /api/queue`（无需再点“开始”）。
- 队列沿用现有后端串行 worker（ADR-027）；拖入 N 个文件就排 N 个，前一个完成自动接下一个。
- 保留“浏览文件…”按钮；浏览器外壳仍只能手填路径（平台限制，界面写明）。

### 3.5 基本选项

新建任务页保留/补齐：

| 选项 | 默认 | 持久化 |
|---|---|---|
| 输出目录 | 空 = `workdir/libraries/` | `settings.output_dir` |
| ASR 模型 | `whisper-large-v3` | `settings.stages.whisper_model` |
| 语言 | `zh` | `settings` + 请求 `params.input_language` |
| 分离 | 固定开启，不可关 | 不提供开关 |

### 3.6 TXT 产物

`export` 阶段在现有 `transcript.{json,md,srt,vtt}` 之外写 `text/transcript.txt`：
纯文本，一段一行，不带时间码（要时间码用 srt/md）。

## 4. 验收标准

| AC | 判据（能红） | 验证 |
|---|---|---|
| AC-01 强制分离 | 真实运行先产出 `audio/vocals.wav` 与 `audio/instrumental.wav`，`transcribe` 的输入是 vocals；`meta.json` 记录分离模型 | `runs/*.jsonl` 阶段顺序 + 文件 mtime + meta |
| AC-02 不许静默跳过 | 分离运行时或模型缺失时运行**失败**，错误含安装指引；`runs/*.jsonl` 不得出现 `SEPARATE_UNAVAILABLE` | 临时 workdir 复现 + 断言日志 |
| AC-03 最大模型默认 | 干净 settings 的运行使用 `whisper-large-v3`；若降档，`MODEL_SUBSTITUTED` 在任务行可见 | 真跑 + 截图 + run log |
| AC-04 TXT | `text/transcript.txt` 存在，行数 = `transcript.json` 段数 | 脚本断言 |
| AC-05 拖入即跑 | 桌面窗口拖入 N 个视频，队列立即出现 N 项并串行跑完 | 手工 E2E + 队列快照 |
| AC-06 选项生效 | 改输出目录/模型/语言后运行，产物落点与 `params_snapshot` 反映选择 | 两次运行对比 |
| AC-07 端到端 | 真实带 BGM 视频跑通：vocals + txt + srt 齐全；Checkfix 全绿 | 本机实跑 + 全量命令 |

## 5. 独占文件

`internal/engine/{separator.go,separator_helper.py,locate.go}`、`internal/runctl/{run.go,helpers.go}`、
`internal/config/config.go`、`internal/registry/{registry.go,data/registry.json}`、
`internal/pipeline/pipeline.go`、`cmd/stemscribe/cli.go`、`cmd/stemscribe-gui/main_gui.go`、
`frontend/app/components/NewTask.tsx`、`frontend/lib/{api,i18n}.ts`、
`docs/{PIPELINE,MODEL-REGISTRY,RESOURCE-SCHEDULER,ARCHITECTURE}.md`、`tasks/README.md`。

与 CHG-0002/0008/0012/0013 交叠处在本表登记；不同时改其他 CHG 的独占文件。

## 6. 里程碑（按序，逐个可验收）

| M | 内容 | 完成判据 |
|---|---|---|
| M1 | 分离运行时 + UVR 模型 + `Separator` 适配 + 强制分离 | AC-01、AC-02 |
| M2 | large-v3 实测可装 + 默认 + 可见降档 | AC-03 |
| M3 | TXT + 拖入多文件自动队列 + 选项 | AC-04、AC-05、AC-06 |
| M4 | 真实视频 E2E + Checkfix + commit/push | AC-07 |

## 7. 风险与回滚

| 风险 | 处置 |
|---|---|
| `audio-separator` 体积大/安装慢 | 运行时按需安装；安装失败直接报错，不静默跳过（AC-02） |
| hf-mirror 无合适 MDX 权重 | 退回已镜像的 HP2（VR），在 `MODEL-REGISTRY.md` 记录取舍；仍须实测 sha256 |
| 显存不足 | 降一档 + 可见事件；不做隐式 CPU 切换 |
| 拖入在浏览器壳不可用 | 仅桌面窗口支持；界面写明，不伪装 |
| 回滚 | 分离阶段可回退为“失败即止”（不会静默降级）；模型/运行时条目可删除 |

## 8. Definition of Ready / Done

**DoR**：owner 指令已给出（本篇 status=approved）；分离运行时与模型来源可用；large-v3 可下载。

**DoD**：AC-01…AC-07 全部有可复现证据；`go build/vet/test -race`、`pnpm lint/typecheck/build`、
`verify-contract`、`i18n-check`、`contrast` 全绿；实测数据回填 `TRACEABILITY.md`；commit/push 完成。
