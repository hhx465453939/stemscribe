# CHG-0002 · 真实推理引擎与运行时工程契约

| 字段 | 值 |
|---|---|
| ID | CHG-0002 |
| version | 1 |
| status | **approved**（owner 明确指令：「cudavulkan 都要有，别拖，一次性弄完，其余的按你的建议走」） |
| prd_ref | `tasks/CHG-0002/PRD.md` |
| supersedes | — |
| approved_by | owner（2026-11-17：「cudavulkan 都要有，别拖，一次性弄完，其余的按你的建议走，人声分离你可以后延」） |
| approved_at | 2026-11-17 |

---

## 1. 背景 / 目标 / 约束 / 假设 / 未决问题

### 1.1 背景
CHG-0001 把骨架与编排立起来了，但 `run` 走的是 `cmd/stemscribe/synthetic.go` 里的**合成引擎**：
它不读音频，按 `--duration-ms` 生成等距占位段。这条链上"下载→校验→安装→调用→降级"每一段都是空的。

### 1.2 目标
见 `PRD.md` §2 G-1…G-6。一句话：**让产物内容来自音频本身，且引擎来源可审计。**

### 1.3 约束
- Go 仍然只编排，不碰张量（ADR-002 不变）
- 运行时与模型**不随安装包分发**（ADR-003 不变）；N-02 的 40 MB 安装包上限不得破
- 不得引入 cgo 依赖（当前 CI 的 go job 是 `CGO_ENABLED` 默认但零 cgo 包；引入 cgo 会连带要求 MSYS2，扩大构建面）
- 只支持 Windows x64（ADR-001 首发范围）
- 不得编造 sha256（`AGENTS.md` 铁律 2）

### 1.4 假设
- GitHub Actions `windows-latest` 具备 CMake 与 MSVC 生成器
- 模型可经 `huggingface.co` 直连；CI 可访问（首次需验证）
- 用户机器上 Vulkan 加载器由显卡驱动提供（Win10+ 常见）

### 1.5 未决问题（阻塞 M1，不阻塞 SPEC 评审）
| # | 问题 | 默认处置 |
|---|---|---|
| Q-1 | whisper.cpp 官方 HF 仓库里**量化档位的确切文件名**（`ggml-medium-q5_0.bin` 是否存在） | M1 逐个 `HEAD` 核实；不存在的档位改用官方确实存在的变体，并把核实结果回填 registry + 本契约 |
| Q-2 | Vulkan SDK 在 CI 的安装方式与耗时（预计 200 MB / 3–5 min） | M1 实测；超 10 min 则降级为"仅 CPU 自建 + Vulkan 由贡献者本地构建" |
| Q-3 | 30 秒真实语音样本从哪来（不能塞版权音频进仓库） | M1 用 TTS 合成（piper/espeak）或 ffmpeg 生成 + 人工校验，产物仅存 hash |
| Q-4 | `whisper-cli` 的 `-oj` 输出字段是否稳定跨版本 | M1 固定版本 + 用 golden file 断言，锁版本 |

## 2. 现状架构与债务影响

| 位置 | 现状 | 本变更 |
|---|---|---|
| `cmd/stemscribe/cli.go` | `newSyntheticEngine()` 硬编码为默认 | 改为按 registry 解析出的真实运行时启动；`--engine synthetic` 显式保留 |
| `internal/pipeline/pipeline.go` | 定义 `Engine` 端口（`Transcribe` / `Separate`） | 端口不变；新增 `Probe()` 与 `Cancel()` 语义（见 §4.1） |
| `internal/registry/data/registry.json` | whisper 三个 runtime + 三个 model，**全部 `sha256: null, tbd: true`** | 填实测 sha256；新增 `vulkan` 变体的真实版本；细化 `quantization` |
| `internal/downloader` | 已实现续传/sha256/原子落盘/穿越防护 | **不改**，直接复用（这是 CHG-0001 留下的最大红利） |
| `internal/resource` | 已有预算、LRU、后端选择与降级链 | 接线：把 `SelectBackend` 的结果真的用于挑运行时 |
| `internal/engine/protocol.go` | 内部 JSONL 帧定义 | 保留为**内部契约**；新增适配器把上游原生输出翻译成它 |

**债务影响**：`synthetic.go` 会从"唯一实现"降级为"测试替身"。必须防止它被误用于生产——用显式 flag 而不是默认值来约束。

## 3. 选定设计与 ADR

### ADR-012 · GPU 后端：Vulkan 与 CUDA **同时交付**（owner 裁决）

**决策**：CI 构建三个变体：`cpu`、`vulkan`、`cuda12`。运行时各为独立资产，按硬件择优加载。

**理由**
- owner 明确要求两者都要：Vulkan 负责**通用性**（AMD/Intel/核显、无需厂商 SDK），CUDA 负责 **N 卡性能上限**
- 两者共用同一套连接层（都只是 whisper.cpp 的编译开关），多一个构建矩阵而非多一套代码
- 用户不需要懂这些：探测→择优→失败降级，全程自动且带原因

**代价（必须写在脸上）**
- CUDA 运行时包显著变大：除 `whisper-cli.exe` 外要带 `cudart`/`cublas` 等 DLL（cublas 单文件百 MB 量级）。这是**下载体积**而不是安装包体积（N-02 不受影响）
- CI 构建矩阵变长：CUDA 需装 CUDA Toolkit，Vulkan 需装 SDK，两者都拉高首跑耗时
- 必须分别验证“不串味”：CUDA 变体不得被当作通用包发给无 N 卡用户（反之亦然）

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 只做 Vulkan | owner 不接受：N 卡用户拿不到应有性能 |
| 只做 CUDA | 对 AMD/Intel 用户等于没 GPU 加速 |
| 运行时下 CUDA 全家桶 + 一个包通吃 | 无 N 卡用户白下数百 MB，且可能因缺驱动 DLL 启动失败 |
| 让用户自选下载 | 违背「默认可用」：用户不该知道 cublas 是什么 |

### ADR-013 · 运行时由本项目 CI 自建并作为 release 资产发布

**决策**：`.github/workflows/runtime.yml` 在 tag 或手动触发时构建两个变体，打包上传到 release，并生成 `SHA256SUMS`。registry 的 `sha256` 由构建产物**实测**回填。

**理由**
- 分发**我们自己的构建**，来源链完整（commit → 产物 → 校验值），符合"引擎来源可审计"（G-2）
- 第三方预编译包的来源无法自证，且其编译选项（是否开 AVX、是否带额外库）不可见
- 与 ADR-003 的"不镜像第三方权重"一致：我们发的是**代码产物**（MIT），不是权重

**代价**
- CI 构建耗时（预计 5–12 min/变体），且 Vulkan SDK 体积拖慢缓存预热
- 每次升级 whisper.cpp 都要重出资产并更新 sha256 → registry 与 release 强耦合（必须同 tag 发布）
- 上游 MIT 许可允许再分发，但仍需在 `THIRD-PARTY-NOTICES.md` 标注

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 用第三方 GitHub 上的 Windows 预编译 | 来源不可审计、编译选项未知，等于把供应链交给陌生人 |
| 首次运行时在用户机器上编译 | 需要完整 MSVC + CMake + SDK，与「默认可用」冲突 |
| 随安装包内置 | 破坏 40 MB 上限（N-02） |

### ADR-014 · 人声分离整体移出本 CHG

**决策**：CHG-0002 只做**转写**，转写对象是原始音轨。分离（UVR/MDX）移交 CHG-0003。

**理由**
- 分离需要独立运行时：Python embeddable + onnxruntime（约 350 MB）或自研 ONNX C++ 分离器（工程量另算）
- 把"转写能跑通"和"分离能跑通"耦在一个 CHG 里，会让两者都延期且难以定位失败
- 对以人声为主的访谈/口播素材，直接在原始音轨上转写通常已可用；分离的收益主要在音乐类

**代价**
- v0.1 的用户体验里"人声分离"勾选框必须**置灰并说明"运行时未就绪（CHG-0003）"**，不能假装能用
- `pipeline.Engine` 的 `Separate` 方法仍需保留（端口不破坏），但返回明确的 `ErrNotImplemented`

### ADR-015 · 引擎适配器负责协议翻译（修正 ADR-002 的隐含假设）

**决策**：`internal/engine/protocol.go` 的 JSONL 是**内部契约**，只约束我们自己的适配器与 pipeline。上游 `whisper-cli` 不会说这个协议；适配器负责：
`spawn whisper-cli` → 解析 stderr 进度行 → 读 `-oj` 产出的 JSON → 翻译成内部 `Frame` → 供 pipeline 消费。

**理由**
- ADR-002 写的是"引擎经 stdin/stdout JSONL 驱动"，这对**自研引擎**成立，但对上游 CLI 不成立。若不澄清，实现时会误以为要改造 whisper.cpp
- 翻译层放在适配器里，换来上游可替换（未来换 faster-whisper / 换版本）而不动 pipeline

**代价**
- 进度来自 stderr 文本，版本间格式可能变 → 锁版本 + golden 解析测试
- 多一层文件 IO（JSON 输出落盘再读），30 分钟音频约 100 KB，可接受

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| fork whisper.cpp 加 JSONL 输出 | 维护上游 diff，得不偿失 |
| 自己写 CGO 绑定直接调 whisper API | 引入 cgo，扩大构建面（约束 §1.3） |
| 解析 `--output-srt` 而非 JSON | srt 无置信度字段，丢信息 |

## 4. 接口 / 数据 / 安全 / 兼容 / 迁移 / 可观测 / 回滚

### 4.1 接口（对内部）
```go
// internal/pipeline (扩展，不破坏现有实现)
type Engine interface {
	Transcribe(ctx context.Context, audioPath, outPath string, opts TranscribeOptions, onSeg func(Segment)) error
	Separate(ctx context.Context, in, vocals, instr string, opts SeparateOptions) error
}

// internal/engine (新)
type Runtime struct {
	ID       string           // registry runtime id
	Root     string           // <workdir>/runtimes/whisper/<version>-<backend>/
	Exe      string           // 绝对路径
	Backend  registry.Backend
	Version  string
}

type Whisper struct { rt Runtime; model ModelRef; log io.Writer }
func (w *Whisper) Probe(ctx context.Context) (ProbeResult, error)   // 跑一次 --help + 版本行
func (w *Whisper) Transcribe(...) error                             // 实现 pipeline.Engine
```

`TranscribeOptions` 增加：`ModelPath`（绝对路径，由 registry+workdir 解析）、`Threads`、`GPULayers`。

### 4.2 数据（落盘契约）
```
<workdir>/runtimes/whisper/<version>-<backend>/
  whisper-cli.exe  *.dll  VERIFIED.json      # {sha256, source_url, installed_at, our_commit}
<workdir>/models/whisper/<model-id>/
  ggml-<name>.bin  VERIFIED.json
<workdir>/cache/demux/<video_id>/
  audio16k.wav                                # pcm_s16le 16k mono（whisper 硬要求）
```
`VERIFIED.json` 是 provenance 的一环：`meta.json` 只引用 runtime/model id + sha256 前 12 位，完整值在这里。

### 4.3 安全
- 下载仍然只走 `internal/downloader`（sha256 强校验 + 原子落盘 + 路径穿越防护）
- 引擎进程以**参数数组**启动（不拼 shell 字符串），模型路径与输入路径不经过 shell 解释
- `ffmpeg` 只接受本地文件路径，不接受 URL（阻断"用 ffmpeg 当下载器"的越界用法）
- 子进程超时与取消必须生效（`exec.CommandContext`），防孤儿占用显存

### 4.4 兼容 / 迁移
- registry 从"全 tbd" → "实测值"：旧 workdir 里若有 `models.lock.json`，以 lock 为准（已装不动），新装走新值
- `--engine synthetic` 语义保留 → 已有脚本不破
- `meta.json` 增加 `runtime` 字段（可选，读旧文件不报错）

### 4.5 可观测
- 每次 spawn 记 `engine.spawn{pid, exe, backend, model, sha12}`、`engine.exit{code, elapsed_ms}`
- 进度事件 ≤5 Hz（沿用 ARCHITECTURE §4 的节流约定）
- 失败必带 `code`（`ENGINE_MISSING` / `ENGINE_SPAWN` / `ENGINE_TIMEOUT` / `ENGINE_OOM` / `ENGINE_BAD_OUTPUT`）

### 4.6 回滚
单 commit 可回滚到合成引擎：`--engine synthetic` 始终可用，且 `registry.json` 的改动是可逆数据。无数据库迁移。

## 5. 里程碑（依赖顺序）

| # | 里程碑 | 文件/模块 | 验收 | 回滚 |
|---|---|---|---|---|
| M1 | 运行时自建流水线（**三变体**：cpu/vulkan/cuda12） | `.github/workflows/runtime.yml`、`scripts/runtime-build.ps1` | AC-01/02/03 | 删 workflow |
| M2 | registry 实测回填 | `internal/registry/data/registry.json`、`docs/MODEL-REGISTRY.md` | AC-04 | revert 数据 |
| M3 | ffmpeg 运行时 + demux | `internal/engine/ffmpeg.go` | AC-05 | revert |
| M4 | whisper 适配器 | `internal/engine/whisper.go`、`internal/engine/progress.go` | AC-06/07/08 | revert（仍可用 synthetic） |
| M5 | CLI 接线与降级 | `cmd/stemscribe/cli.go`、`internal/resource` 接线 | AC-09/10/11 | 默认值改回 synthetic |
| M6 | 真 e2e + 文档 | `scripts/smoke-test.ps1 -Real`、`docs/UPSTREAM-RESEARCH.md`、README | AC-12 | revert |

## 6. Definition of Ready / Done

**DoR**
- PRD 就绪、需求由 owner 明确下达（「继续吧」承接上一轮提议）✅
- AC 逐条可验证 ✅
- 目标文件清单明确、与 CHG-0007/0003 不冲突 ✅
- 未决问题 Q-1…Q-4 有默认处置，不阻塞评审 ✅

**DoD**
- AC-01…AC-12 全绿，证据写入 `TRACEABILITY.md`
- registry 中**不再有任何 `tbd: true`** 的 whisper 运行时/模型条目（实测回填）
- `go build/vet/test` 全绿；`--engine synthetic` 仍可用
- `TRACEABILITY.md` §3 的实测表填真值（RTF、峰值显存、包体积）
- README 的"真实推理"状态从 scaffold 改为 v0.1 可用；`docs/ROADMAP.md` 勾掉 CHG-0002

## 7. 评审与交付门控

| 门 | 条件 | 谁批 |
|---|---|---|
| SPEC_READY | 本文档完整可评审 | agent 自评 |
| **SPEC_APPROVED** | owner 明确批准（**不得自动批准**） | owner |
| 实现 | 批准后开工 | — |
| **commit** | AC 全绿 + 证据齐 | owner |
| **push** | commit 后单独授权 | owner |

> 本轮只产出契约，**未写任何实现代码、未 commit、未 push**。

## 8. 追溯与证据计划

| AC | 判据（能红） | 验证命令 | 证据落点 |
|---|---|---|---|
| AC-01 | CI 产出 `whisper-cli.exe`（cpu）且能打印版本 | `runtime.yml` 内 `.\whisper-cli.exe --help` 退出码 0 | Actions 日志 |
| AC-02 | vulkan 变体产物**不依赖任何 CUDA DLL** | `dumpbin /dependents` 或 `grep -i cuda` 于依赖清单 0 命中 | `scripts/runtime-verify.ps1` 输出 |
| AC-03 | release 附 `SHA256SUMS`，且与 registry 一致 | 脚本比对 release 资产 hash vs registry | `TRACEABILITY.md` §3 |
| AC-04 | registry 无 `tbd:true`（whisper 部分） | `go test ./internal/registry -run TBD` 断言 | CI |
| AC-05 | mp4 → 16k mono s16 wav，时长误差 ≤100ms | `ffprobe` 对比源/产物时长 | 脚本输出 |
| AC-06 | 真转写：段数>0、`start_ms` 严格递增、含音频中真实词 | `scripts/smoke-test.ps1 -Real` | 日志 |
| AC-07 | 适配器实现 `pipeline.Engine`（编译期断言） | `var _ pipeline.Engine = (*Whisper)(nil)` | 编译 |
| AC-08 | 取消后 ≤2s 内引擎退出、无孤儿进程 | 取消测试 + `tasklist` 断言 | CI |
| AC-09 | Vulkan 不可用 → 落 CPU 且事件含 `reason` | 改名运行时目录后重跑 | 日志 |
| AC-10 | 缺运行时/模型时一次列全缺失项（不是逐条报错） | `run` 于空目录 | 截图/日志 |
| AC-11 | `tbd:true` 条目 UI 显示未发布且不可安装 | 前端断言（可后置到 CHG-0005 联调） | 手工 |
| AC-12 | 30s 真实语音端到端：产物 5 类文件齐 + 内容来自音频 | `scripts/smoke-test.ps1 -Real` | 日志 |

## 9. 独占文件清单

| 范围 | 归属 |
|---|---|
| `internal/engine/**`（含 `whisper.go`、`ffmpeg.go`、`progress.go`） | CHG-0002 |
| `internal/registry/data/registry.json`（whisper/ffmpeg 条目） | CHG-0002 |
| `.github/workflows/runtime.yml`、`scripts/runtime-{build,verify}.ps1` | CHG-0002 |
| `cmd/stemscribe/cli.go`（引擎接线部分） | CHG-0002 |
| `docs/{MODEL-REGISTRY,UPSTREAM-RESEARCH}.md`（运行时章节） | CHG-0002 |
| `THIRD-PARTY-NOTICES.md`（whisper.cpp / ffmpeg 许可） | CHG-0002 |

不触碰：`internal/pipeline/**`（端口不变）、`internal/provider/**`、`frontend/**`、`tasks/CHG-000{1,7}/**`。

> 与 `tasks/README.md` 现有登记冲突：那里 CHG-0002 写的是 `internal/engine/**` + `internal/resource/**`。
> `internal/resource` 本 CHG **只接线不改逻辑**；若需改预算算法，另开 CHG-0006。
