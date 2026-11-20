# CHG-0008 · 运行参数控制面与后端兼容矩阵工程契约

| 字段 | 值 |
|---|---|
| ID | CHG-0008 |
| version | 1 |
| status | **approved**（owner：「你把我说的这些事，整体去立项然后开发一下吧」） |
| prd_ref | `tasks/CHG-0008/PRD.md` |
| supersedes | — |
| approved_by | owner（2026-11-18 会话指令） |
| approved_at | 2026-11-18 |

---

## 1. 背景 / 目标 / 约束 / 假设 / 未决问题

### 1.1 背景
当前 whisper 调用是"无选项即默认"：`whisper.go` 只传 `-m -f -oj -of -pp -l -t`，其余 40+ 个真实参数全部不可达。
`-l` 默认 `en`，中文素材会被当英文硬转。分离只有一个布尔开关，stem 目标不可选。后端只在 NVIDIA 上判 CUDA，AMD/Intel 直接掉 CPU。

### 1.2 目标
见 `PRD.md` §2 G-1…G-6。一句：**把引擎真实能力翻译成可选、可填、可复现的选项，并让每种显卡都能跑。**

### 1.3 约束
- 参数必须能追溯到 `whisper-cli --help` 的真实条目（本机 1.9.5 实测），**不许凭记忆编 flag**
- Go 仍不碰张量（ADR-002 不变）
- 不破坏既有 AC：AC-06/07（构建）、AC-12（真 e2e）、CI 三 job
- 新选项必须中英双语（CHG-0007 的 `i18n-check.sh` 会拦硬编码）
- 只做 Windows x64

### 1.4 假设
- 分离模型元数据可从模型族推断（MDX/VR 2-stem，Demucs 4-stem），并在 registry 显式声明
- Vulkan 在 AMD/Intel 上可用（Windows 驱动自带 loader）

### 1.5 未决问题（不阻塞本 CHG 的 P0）
| # | 问题 | 默认处置 |
|---|---|---|
| Q-1 | 分离推理引擎走 ONNX Runtime CGO 还是 Python 运行时 | 本 CHG 不做实现（见 ADR-021），只留接口与能力声明；CHG-0009 决定 |
| Q-2 | `-tr` 的目标语言只能英文，是否值得做"转写→再翻译"两段式 | 本 CHG 只暴露 `-tr`；两段式方案记入 CHG-0009 |
| Q-3 | 4-stem 的输出命名与 bundle 目录结构 | 预留 `audio/stems/{vocals,drums,bass,other}.wav`，本 CHG 不产文件 |

## 2. 现状架构与债务影响

| 位置 | 现状 | 本变更 |
|---|---|---|
| `internal/engine/whisper.go` | 硬编码 6 个 flag | 改为从参数结构体渲染完整参数集 |
| `internal/pipeline/pipeline.go` | `TranscribeOptions{Model, Language, Backend, GPULayers, VAD, Threads}` | 扩展为 `TranscribeOptions` 全字段（**跨界：pipeline 名义属 CHG-0003，同批登记**） |
| `internal/registry/data/registry.json` | 分离模型只有 1 条（MDX），无 stems | 补 `stems` 字段与模型族 |
| `internal/resource` | `SelectBackend` 按 NVIDIA 判 CUDA | 补 AMD/Intel → vulkan 判定与候选链 |
| `cmd/stemscribe/cli.go` | 只透传 model/language | 全参数透传 |
| `frontend/app/settings` | 只有 provider 页 | 新增"运行参数"面板 |

**债务影响**：`TranscribeOptions` 扩张会牵动 `pipeline` 与 `engine` 两侧，必须一次改到位；否则会出现"UI 有选项但引擎不认"的假象。

## 3. 选定设计与 ADR

### ADR-016 · 参数面以引擎真实能力建模，不按 UI 美观分组

**决策**：`internal/engine/params.go` 定义 `WhisperParams` 结构体，**每个字段对应一个实测 flag**，并用一张 `paramSpecs` 表声明：flag 名、语义键、类型、默认值、范围/枚举、分组、单位。
UI 与 CLI 都从这张表渲染；`paramSpecs` 是唯一真相源。

**理由**
- 三处（UI、CLI、文档）各写一遍必然漂移；一张表 + 代码生成式渲染可以杜绝
- 默认值直接来自 `--help` 实测，可以被脚本比对（AC-02）
- 新增参数只需加一行表项，UI 自动出现

**代价**：结构体与表需要人工同步（用测试断言"表里每个 flag 都在结构体里有字段"兜住）。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 让用户手填 flag 串 | 注入面 + 破坏 provenance + 无法校验 |
| 只暴露"预设档位" | 违背 G-1，无法复现具体值 |
| 每个参数写一个 UI 组件 | 40+ 组件，维护地狱 |

### ADR-017 · 输入语言与输出语言是两个维度，不是"选语言"

**决策**
- `input_language`（`-l`，含 `auto`）= 素材里说的是什么语言
- `detect_only`（`-dl`）= 只探测不转写
- `translate`（`-tr`）= 把源语言翻译成**英文**（whisper 只支持这一个目标）

三者独立建模，并在提交前做冲突检测：
1. `.en` 模型 + `input_language != en` → 拒绝（该模型没有多语种词表）
2. `.en` 模型 + `auto` → 拒绝（无法保证是英文）
3. `detect_only` + `translate` → 拒绝（探测模式不产生文本）

**理由**：用户口语里的"输出语言"实际包含三种不同诉求（原文转写 / 翻译 / 只想知道是什么语言）。把三者塞进一个下拉框，会得到"选了中文却输出英文"这类不可解释的结果。

**代价**：UI 多一行；非英文目标翻译需要两段式（本 CHG 不做，见 Q-2）。

**被否决方案**：单一下拉「中文/英文/翻译」——语义混乱且无法表达"识别中文但输出英文"。

### ADR-018 · 后端矩阵：优先"能跑起来"，DirectML 如实标注不支持

**决策** 候选链：`cuda12 → vulkan → cpu`
- NVIDIA + CUDA 运行时已装 → `cuda12`
- 其余有 GPU 的情形（AMD / Intel / NVIDIA 无 CUDA）→ `vulkan`
- 无 GPU 或 Vulkan 不可用 → `cpu`
- DirectML：**在矩阵里标注"上游 whisper.cpp 不支持"**，不做假承诺
- 用户可锁定后端；锁定后不自动降级，失败直接报错（给确定性）

**理由**：用户明确"AMD 卡用 3D 渲染去跑也要能用"。Vulkan 在 Windows 上由驱动提供 loader，AMD/Intel 都能用；这是"让所有显卡都能参与"的唯一现实路径。

**代价**：Vulkan 在老驱动上可能初始化失败 → 必须真降级到 CPU 并写明原因；Vulkan 性能低于 CUDA（值待实测，登记 TBD）。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 只判 NVIDIA | AMD 用户直接掉 CPU，等于放弃一半用户 |
| 声称支持 DirectML | 上游没有该后端，做了就是假承诺 |
| 强制用户手选后端 | 违背「默认可用」 |

### ADR-019 · 分离 stem 是模型属性，不是全局开关

**决策**：registry 的分离模型声明 `stems: []string`。
- MDX-Net / VR 族：`[vocals, instrumental]`
- Demucs 族：`[drums, bass, other, vocals]`
任务的 `stem_target` 只能是"该模型 stems 的子集 + 组合"；UI 按所选模型动态启用。

**理由**：`UVR-MDX-NET-Inst_HQ_3.onnx` 这类模型名字里就写着 Inst（instrumental），说明 stem 能力属于模型；用全局开关表达必然出现"选了 4-stem 却用了 2-stem 模型"的错配。

**代价**：切模型时选项要重算；用户可能困惑为何 4-stem 只对部分模型可选——用灰显 + 原因说明解决。

### ADR-020 · 参数快照必须落盘并与阶段失效联动

**决策**：`transcribe` 阶段的 params hash 由**参与该阶段的全部参数**计算；参数变化 → `transcribe` 及之后阶段失效（复用 `store.Checkpoint.Plan` 的既有机制）。
参数快照写进 `meta.json.provenance.params_snapshot`（不含密钥）。

**理由**：参数不可复现 = 产物不可信（项目重心第 1 条）。既有 hash 机制已经在用，扩参数只需扩展 hash 输入。

**代价**：改任何一个参数都会让转写重跑，用户可能觉得"我只是改了个阈值"。这是**正确**行为，UI 需提示"该改动会使转写重跑"。

### ADR-021 · 分离推理引擎不在本 CHG，但接口与能力声明必须先就位

**决策**：本 CHG 交付 `Separator` 接口、registry 的 stems 元数据、UI 选项、请求管线与 provenance 字段；推理实现交 CHG-0009。
引擎未安装时：**选项禁用 + 说明"分离引擎未就绪（CHG-0009）"**，绝不假装可用。

**理由**：MDX 是 ONNX（UVR 自带 `onnxruntime/capi`），需要 CGO 绑 C API + 实现 STFT/iSTFT + 分块推理，数值正确性必须对照参考实现验证，是独立工程。硬塞进本 CHG 会导致两边都半成品。

**代价**：本 CHG 结束时"instrumental 分离"仍不可真跑。这一点必须写进 PRD 非目标与 TRACEABILITY 缺口，不允许含糊。

## 4. 接口 / 数据 / 安全 / 兼容 / 迁移 / 可观测 / 回滚

### 4.1 接口
```go
// internal/engine/params.go
type WhisperParams struct {
    InputLanguage string  // -l  ("auto" 允许)
    DetectOnly    bool    // -dl
    Translate     bool    // -tr
    Threads       int     // -t
    Processors    int     // -p
    BestOf        int     // -bo
    BeamSize      int     // -bs
    Temperature   float64 // -tp
    TempInc       float64 // -tpi
    NoFallback    bool    // -nf
    MaxContext    int     // -mc
    MaxLen        int     // -ml
    SplitOnWord   bool    // -sow
    AudioCtx      int     // -ac
    NoSpeechThold float64 // -nth
    EntropyThold  float64 // -et
    LogprobThold  float64 // -lpt
    WordThold     float64 // -wt
    SuppressNST   bool    // -sns
    InitialPrompt string  // --prompt
    CarryPrompt   bool    // --carry-initial-prompt
    VAD           bool    // --vad
    VADThreshold  float64 // -vt
    VADMinSpeech  int     // -vspd
    VADMinSilence int     // -vsd
    VADPad        int     // -vp
    FlashAttn     bool    // -fa / -nfa
    NoGPU         bool    // -ng
    GPUDevice     int     // -dev
    OutSRT, OutVTT, OutTXT, OutJSON, OutJSONFull, OutLRC, OutCSV bool
}

type ParamSpec struct {
    Key, Flag, Group string
    Kind  string   // int|float|bool|enum|string
    Def   any
    Min, Max any
    Enum  []string
    Unit  string
}
func Specs() []ParamSpec          // UI/CLI 唯一真相源
func (p WhisperParams) Args() []string   // 渲染为 flag
func (p WhisperParams) Validate(model registry.Model) error  // 冲突检测
```

### 4.2 数据
| 位置 | 字段 |
|---|---|
| `registry.json` models[] | 新增 `stems []string`（分离模型必填） |
| `meta.json.provenance.params_snapshot` | 参与转写的参数快照 |
| `settings.json` | `stages.transcribe` 全参数默认值 |

### 4.3 安全
- 参数值全部走类型化结构体，**不经 shell 拼接**（`exec.Command` 参数数组）
- `--prompt` 长度上限（n_text_ctx/2），超限截断并提示，不静默
- 数值范围前置校验，拒绝越界（避免引擎内部不可解释的崩溃）

### 4.4 兼容 / 迁移
- `settings.json` 老文件缺字段 → 用默认值补齐（`Load` 已有该模式）
- 老 bundle 无 `params_snapshot` → 读时不报错，只在 UI 显示"未记录"
- 既有 CLI 用法不变（`--model` 保留）

### 4.5 可观测
- `engine.spawn` 事件带完整参数（脱敏后）
- 冲突拒绝 → `CFG_CONFLICT` 错误码 + 可行动提示

### 4.6 回滚
参数面是加法；`WhisperParams{}` 零值 = 当前行为（保守默认）。回滚只需 revert 相关 commit；已产 bundle 不受影响。

## 5. 里程碑

| # | 里程碑 | 文件 | 验收 |
|---|---|---|---|
| M1 | 参数规格表 + 结构体 + 渲染/校验 | `internal/engine/params.go`、`params_test.go` | AC-01/02/09 |
| M2 | 引擎接入全参数 | `internal/engine/whisper.go` | AC-03/04 |
| M3 | registry stems + 分离能力声明 | `registry.json`、`internal/registry` | AC-08/09 |
| M4 | 后端矩阵 + 降级/锁定 | `internal/resource`、`cmd/stemscribe/wire.go` | AC-06/07 |
| M5 | 参数快照与阶段失效 | `internal/pipeline`、`internal/store`、`cmd/stemscribe/cli.go` | AC-05 |
| M6 | UI 参数面板（双主题双语） | `frontend/app/**`、`lib/i18n.ts` | AC-10 |
| M7 | 文档同步 | `docs/UI-DESIGN.md`、`docs/RESOURCE-SCHEDULER.md`、README | AC-10 |

## 6. DoR / DoD

**DoR**：PRD 就绪且由 owner 下达 ✅；AC 可验证 ✅；目标文件清单明确 ✅；Q-1…Q-3 有默认处置 ✅

**DoD**：AC-01…AC-10 全绿并写入 TRACEABILITY；`go build/vet/test` 全绿；`i18n-check.sh` 通过；真 e2e（视频→文字）仍绿；CHG-0002 的既有验证不回退。

## 7. 评审与交付门控

| 门 | 条件 |
|---|---|
| SPEC_READY | 本文档完整 |
| **SPEC_APPROVED** | owner 已明确批准（「整体去立项然后开发」） |
| 实现 | 批准后开工 |
| commit / push | owner 既有「全自动开发测试 commit push」授权 |

## 8. 追溯与证据计划

| AC | 判据（能红） | 验证 |
|---|---|---|
| AC-01 | `paramSpecs` 覆盖 `--help` 中所有可调项，且每个 spec 都有对应结构体字段 | `go test ./internal/engine -run TestSpecsCoverage` |
| AC-02 | 每个 spec 的默认值等于二进制 `--help` 实测默认 | `scripts/param-defaults-check.ps1` 对照输出 |
| AC-03 | `input_language=zh` 时命令含 `-l zh`；`translate` 时含 `-tr`；`detect_only` 时含 `-dl` | 适配器单测断言 args |
| AC-04 | 参数快照落 `meta.json`，且改 `beam_size` 会使 transcribe 失效 | 单测 + 手工 |
| AC-05 | 冲突检测四类全拦：`.en`+非en、`.en`+auto、detect+translate、VAD 无模型 | 表驱动单测 |
| AC-06 | 后端矩阵四情形：NVIDIA+CUDA→cuda12；AMD→vulkan；无 GPU→cpu；锁定生效 | `go test ./internal/resource` |
| AC-07 | 降级必带 reason 且写入事件；锁定后不降级 | 单测 + 真跑 |
| AC-08 | registry 分离模型都有 `stems`；MDX/VR=2、Demucs=4 | `go test ./internal/registry` |
| AC-09 | 引擎未就绪时选项禁用并给原因（不假装） | UI 手工 + 后端状态字段 |
| AC-10 | 新选项中英双语、无硬编码、双主题渲染正常 | `i18n-check.sh` + 截图 |

## 9. 独占文件清单

| 范围 | 归属 |
|---|---|
| `internal/engine/**`（params/whisper/separator 接口） | CHG-0008（与 CHG-0002 交叠，CHG-0002 已关闭该轮） |
| `internal/registry/**`（stems 字段与数据） | CHG-0008 |
| `internal/resource/**`（后端矩阵） | CHG-0008（与 CHG-0006 交叠，登记） |
| `internal/pipeline/**`、`internal/store/**`（参数快照，**跨界**） | CHG-0008（同批登记理由：参数失效逻辑必须在 Plan 内） |
| `cmd/stemscribe/**` | CHG-0008 |
| `frontend/app/**`、`frontend/lib/i18n.ts` | CHG-0008（与 CHG-0005 交叠，登记） |
| `internal/runctl/**`、`internal/webapi/**`（**跨界**） | CHG-0010 所有；CHG-0008 仅因 AC-09 需要在这里计算“哪选项当前不可用”（引擎/模型的实存状态只有这一层看得到），不得改动运行控制语义 |
| `scripts/param-defaults-check.ps1` | CHG-0008 |
| `docs/{UI-DESIGN,RESOURCE-SCHEDULER,MODEL-REGISTRY}.md` | CHG-0008 |
