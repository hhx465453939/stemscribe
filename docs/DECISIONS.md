# 架构决策记录（ADR）

> 每条四段式：**决策 / 理由 / 代价 / 被否决方案**。缺一段视为不合格（CHG-0001 AC-03 会红）。
> 编号不复用；推翻旧决策时新增 ADR 并标注 `supersedes`。

---

## ADR-001 · 桌面壳：Wails v2（Go + WebView2）

**决策**：用 Wails v2 作为 Windows 桌面壳，Go 为后端主进程，前端为 Next.js 15 的**静态导出**产物（`output:'export'`），由 WebView2 加载本地文件。单进程 + 单 exe 分发。

**理由**
- 用户指定 Go 为后端语言；Wails 让窗口、IPC、子进程监管、文件系统、GPU 探测全部落在同一个 Go 进程里，**没有跨语言 IPC 与 sidecar 打包**。
- 产物 ~15–25 MB（对比 Electron 150 MB+），符合"本地 AI 工具应轻薄"的定位（PRD N-02 ≤40 MB）。
- IPC 通过编译期生成的 bindings（Go 方法 ↔ TS 函数），**不监听任何本地端口**，默认无网络攻击面。
- Windows 上 WebView2 由系统提供（Win10 1803+ / Win11 自带），无需随包分发浏览器内核。

**代价**
- Windows 构建需要 MSYS2/GCC（WebView2 loader 走 cgo），CI 多一步 `setup-msys2`。
- 前端无法使用 SSR/RSC/Server Actions——桌面客户端本不需要，但意味着 Next.js 只能当"路由 + 构建工具"用。
- Wails 生态显著小于 Tauri/Electron，遇到疑难需读源码；v3 尚未稳定，锁 v2 主线。
- 同时存在 Go 与 Node 两套工具链，贡献者环境准备成本略高。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| Electron + Next.js + Go sidecar | 包体 150 MB+、内存常驻高；Node 主进程与 Go 后端职责重叠，两套进程监管 |
| Tauri 2 + Go sidecar | 壳层用 Rust，Go 降级为被监管的子进程；多一层 JSON-RPC 与打包链，与"Go 是主语言"背道而驰 |
| Wails v3（alpha） | 未稳定，开源项目不宜押注 alpha 的桌面壳 |
| 纯 Web（本地起 http 服务） | 用户明确要 Windows 客户端；且浏览器沙箱拿不到显存信息与进程控制 |

---

## ADR-002 · 引擎边界：Go 只做编排，不碰张量

**决策**：所有神经网络推理交给**外置引擎进程**，Go 通过 stdin/stdout 的 **JSONL 协议**驱动：
- 转写引擎：`whisper.cpp`（GGML/GGUF 权重，预编译多后端二进制：CUDA / Vulkan / DirectML / CPU）
- 分离引擎：Python embeddable 运行时 + `audio-separator`（ONNX Runtime 后端，跑 MDX-Net / VR / Demucs 系权重）
- 两者都由 app 下载安装到 `<workdir>/runtimes/<engine>/<semver>/`，版本独立、可并行共存

**理由**
- 在 Windows 上把 PyTorch + CUDA 打进安装包是 3–5 GB 的地狱：体积、驱动耦合、杀软误报、CI 时长全部失控。外置 + 按需下载是唯一可持续路径（Ollama 已验证过的模式）。
- 崩溃隔离：引擎 OOM 崩溃只死子进程，主进程捕获退出码后按降级链重启，不会让整个 app 挂掉（PRD F-14）。
- 升级隔离：换 whisper 版本不需要重装 app。
- 语言边界清晰：Go 不承担任何 CGO 张量绑定，`go build` 在纯 Go 环境可过（CI 无需 CUDA）。

**代价**
- 需自建协议层（`internal/engine/protocol`）+ 进程监管（存活探测、僵尸回收、Windows Job Object 防孤儿）。
- 首次使用必须联网下载运行时（约 300–900 MB/引擎），离线场景需用户手动导入离线包。
- 进度上报由引擎自己输出（whisper.cpp 的 `--print-progress` 是 stderr 纯文本，需解析适配）。
- 多一层编码开销（音频经文件传递而非管道流），批量小文件时有可感知延迟。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| CGO 直接绑 `whisper.cpp` | 需要为 4 种后端各出一份带 CGO 的构建，CI 矩阵爆炸；崩溃即带走主进程 |
| 全 Python 后端（faster-whisper + audio-separator） | PyTorch 打包 5 GB+；Python GIL 与进程管理在桌面场景更脆弱 |
| 纯 Go 推理（GGML 重实现） | 工程量与精度风险不可接受 |
| 直接调用已安装的 Buzz/UVR | 依赖用户预装第三方软件，无法开源分发的自包含承诺 |

---

## ADR-003 · 运行时与模型分发：Ollama 模式

**决策**：内置只读 `registry.json`（随 app 发版），描述「运行时」与「模型」两类可下载物；下载器实现**分片并发 + 断点续传 + 末端 sha256 全量校验 + 原子落盘（`.part` → `fsync` → rename）**。所有内容落在用户可选 workdir：

```
<workdir>/
├── config/           settings.json（非密）、providers.json（不含明文 key）
├── runtimes/<engine>/<version>/        引擎二进制 + 依赖
├── models/
│   ├── whisper/<model-id>/             *.gguf / *.bin
│   └── separators/<family>/<model-id>/ *.onnx
├── libraries/<video-id>/               产物 bundle（见 ADR-006）
├── cache/            ffmpeg 转码中间件、波形峰值缓存
├── logs/             结构化 JSONL + 崩溃报告
└── tmp/              原子写的暂存区
```

**理由**
- 「装到工作目录」是主人明确要求，也是"整目录拷走即用"的前提（G-2）。
- sha256 全量校验是唯一能防住"下到一半/镜像投毒/磁盘位翻转"的手段；原子 rename 保证断电后不会留下半截模型被当成可用。
- registry 随 app 发版，避免"配置服务器挂了就不能用"；镜像源通过 settings 覆盖 base_url 实现（国内可用 HF 镜像）。

**代价**
- 首次使用需联网（1–3 GB），必须把"下载 " 做成一等公民 UI（进度、限速、镜像切换、失败重试）。
- registry 需要长期维护（模型上新/下架、sha 变更）；`sha256` 未实测前只能是 `null`+`TBD`，不允许编造。
- 磁盘占用需显式预算与清理 UI（模型动辄 GB 级）。
- 权重许可问题：UVR 系权重许可各异，registry 只存**官方直链 + license 字段**，不做镜像分发。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 模型打进安装包 | 安装包 3 GB+，无法增量更新，CI 无法承载 |
| 用 HuggingFace Hub SDK 直接拉 | 多一层 Python 依赖；无法统一做断点/镜像/原子性 |
| 运行时用 Docker | Windows 桌面分发 Docker 不现实 |
| 直接把模型放 `%APPDATA%` | 违背"工作目录自包含"，用户无法整目录迁移 |

---

## ADR-004 · 资源调度：显存预算 + LRU 常驻 + 分段流式 + 四级降级

**决策**：引入 `internal/resource` 调度器，职责四件：

1. **测量**：经 NVML（`nvml.dll`，随 NVIDIA 驱动安装）读取 `total/free/used`；读不到则回退解析 `nvidia-smi --query-gpu` 输出；再无则判定 `cpu-only`。
2. **预算**：`可用预算 = free_vram - reserve`（`reserve` 默认 1.5 GB，可配置），任何引擎启动前的模型选择必须满足 `model.vram_estimate ≤ 预算`。
3. **常驻与驱逐**：已加载模型按 LRU 缓存，切换任务时不立刻卸载；预算不足时按「最久未用 → 最大占用」双键驱逐，驱逐后重测显存，仍不足则降档模型。
4. **降级链**：`cuda → vulkan → directml → cpu`，每次降级记录 `reason` 并在 UI 显性提示（不允许静默降级）。

音频处理统一**分段流式**：
- 分离：默认 15 s 段 + 2 s 交叉淡化（对应 UVR 的 `segment_size` 思路，调小换显存）
- 转写：VAD 切片 + whisper.cpp `--processors N`，段级结果落盘后才处理下一段
- 中间音频统一规格：分离用 44.1 kHz stereo float32；转写用 16 kHz mono float32（whisper 硬要求）

**理由**
- 12 GB 显存不是无限资源：`large-v3` + 长音频 + 并发的组合必然 OOM。预算先行 + 分段流式是唯一稳态方案。
- "边处理边落盘"让长任务可在任意点中断续跑（F-14），也让进度条是**真实进度**而非假动画。
- 显性降级保护用户知情权：用户看到"已降级为 Vulkan（原因：CUDA 初始化失败：驱动版本不匹配）"才能自己修驱动。

**代价**
- 调度器本身是复杂度来源，必须用单测把 4 条分支钉死（AC-12 覆盖率 ≥90%）。
- `vram_estimate` 在 registry 里是**经验值**，必须允许用户覆盖并在首次运行后自动校准（记录实测峰值回写本地 `calibration.json`）。
- 分段会损失跨段上下文（分离在段边界可能出现轻微不连续；转写在段边界可能切断句子）→ 需要 2 s 重叠 + 句子级合并修复。
- NVML 缺失环境（虚拟机、无驱动）必须能优雅退到 CPU。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 一次性整段处理 | 长音频必 OOM，且无法续跑 |
| 固定并发 1 + 固定模型 | 浪费 12 GB 显存，长任务过慢 |
| 交给用户手动选显存参数 | 违背"默认可用"，用户不知道 OOM 边界在哪 |
| 用 `nvidia-smi` 文本解析作主路径 | 每 200 ms fork 一个进程，开销与稳定性都差 |

---

## ADR-005 · LLM 层：OpenAI-compatible 为主 + 能力三层探测

**决策**：
- **协议**：主路径 `OpenAI-compatible`（`GET /v1/models`、`POST /v1/chat/completions`，含 SSE 流式）；`Anthropic` 作为原生适配器补充（`thinking.budget_tokens` 语义不同）。其余厂商一律通过 OpenAI-compatible 网关接入（one-api / OpenRouter / vLLM / Ollama / LM Studio 均满足）。
- **能力信息三层**：①内置 `known-models.json` 能力库（context window / vision / tools / reasoning 支持）→ ②`/v1/models` 返回的真实 id → ③**用户手改覆盖**（持久化到 provider 配置，优先级最高）。三层都缺时用保守默认（128k / 无 vision / 无 tools）。
- **参数面**：见 PRD §5 全表；不支持的能力字段**置灰并说明**，不隐藏。
- **润色输出契约**：必须保留 `[HH:MM:SS]` 锚点；后置校验发现锚点缺失或段落数骤减 → 该段降级为原文并标 `degraded`（防幻觉改写档案）。

**理由**
- OpenAI-compatible 已成事实标准，一套代码覆盖线上十余家 + 全部本地推理框架，**这是唯一能同时满足"线上 provider"与"自建本地端点"的抽象**。
- 能力信息没有统一来源（多数端点不返回 context window），所以必须"库 + 探测 + 手改"三层兜底，否则用户会在长文上撞 400 错误却不知道为什么。
- 时间戳锚点是档案可用性的底线：丢掉锚点的润色稿对"回跳原片"毫无价值。

**代价**
- `known-models.json` 需要持续维护（模型迭代快），且必然滞后 → 因此手改覆盖必须是第一优先级入口。
- SSE 解析要处理各家差异（`data: [DONE]`、`choices[0].delta` vs Anthropic 的 `content_block_delta`）。
- 未知模型的保守默认可能导致长文被静默截断 → 必须在 UI 上显示"按 128k 估算，本稿约 210k，可能截断"。
- 不做 prompt 注入防护的通用方案（本地工具，用户自负其责），仅对下载内容做"非指令化"包装。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 只支持 OpenAI 官方 | 违背"自定义本地端点"的明确需求 |
| 每家写一个原生适配器 | 维护成本爆炸，收益极低 |
| 用 LiteLLM 之类的 Python 代理 | 多一个 Python 运行时依赖，与 ADR-002 冲突 |
| 只信任 `/v1/models` 返回的能力 | 事实是多数端点根本不返回能力字段 |

---

## ADR-006 · 数据模型：自包含 bundle + 阶段级 checkpoint

**决策**：每集一条记录，落在 `<workdir>/libraries/<video_id>/`：

```
meta.json                  # 源信息、时长、状态机、引擎/模型版本、参数快照、prompt hash
source/video.mp4           # 原片（可配置为仅存链接 / 软链 / 保留）
audio/full.wav             # 44.1k stereo（分离输入）
audio/vocals.wav           # 人声（转写输入）
audio/instrumental.wav     # 伴奏（可选保留）
text/transcript.json       # 段级：[{idx,start,end,text,confidence,tokens?}]
text/transcript.md         # 带时间戳锚点的人读稿
text/transcript.srt/.vtt   # 字幕
text/polished.md           # LLM 润色稿（含 degraded 标记）
text/outline.md            # 大纲（可选）
runs/<run_id>.jsonl        # 事件流（可审计：每阶段开始/结束/参数/耗时/引擎版本）
.state.json                # checkpoint：已完成阶段 + 幂等键 + 文件指纹
```

**理由**
- 目录即契约：用户能直接进资源管理器拿走、备份、用 grep 检索，不需要任何"导出"动作。
- `runs/*.jsonl` 提供可审计性（G-5 复现要求）；`.state.json` 提供幂等续跑（F-14）。
- 阶段级 checkpoint 而非文件级：粒度足够且实现简单（每个阶段完成时原子写 `.state.json`）。

**代价**
- 磁盘占用可观（原片 + 双 WAV）→ 提供"仅保留人声 / 删除原片 / 完成后压缩"三种策略，默认保留原片（用户可能有二次需求）。
- checkpoint 与文件实际状态可能不一致（用户手动删了 vocals.wav）→ 续跑前必须做**文件指纹校验**（size + mtime + 可选 sha），不一致则回退到该阶段起点。
- `video_id` 需稳定生成（B 站 BV 号 / 本地文件 path hash），否则重复导入会建两份。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| SQLite 单一数据库存所有内容 | 音视频无法入库；用户无法直接带走单集 |
| 只存数据库 + 外部文件引用 | 迁移即断链，违背"自包含" |
| 单一巨型 JSON 存全部产品 | 无法流式写、崩溃即全丢 |

---

## ADR-007 · Agent 集成：本地 MCP server

**决策**：内置一个 MCP server（同一 Go 二进制，子命令 `stemscribe mcp`），传输支持 `stdio`（默认）与 `streamable-http`（opt-in，仅 127.0.0.1 + 随机 token）。工具集：
- `search_transcripts(query, limit)` → 跨集 FTS5 检索，返回带 `video_id + 起止时间戳` 的片段
- `get_transcript(video_id, start?, end?)` → 指定区间原文
- `get_context_pack(video_id, budget_tokens?)` → 结构化上下文包（大纲 + 关键段 + 锚点）

**理由**
- 主人原话"之后给 agent 发过去，讨论有些内容"——MCP 是当前 agent 生态的标准接入口（Claude/Codex/Pi 等均支持），比自己发明导出格式更有生命力。
- 返回**带时间戳的原文片段**而非摘要，能让 agent 引用可核查的原话（PRD S3 断言）。

**代价**
- MCP 协议仍在演进，需锁定版本并做兼容层。
- stdio 模式下每个 agent 会话会拉起一个进程（轻量，但需保证只读打开库）。
- HTTP 模式引入本地端口，需 token + 仅回环绑定，且默认关闭。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 只做"导出 Markdown 让用户粘贴" | 无法检索、无法引用、每次重复劳动 |
| 自建 HTTP API 让用户写脚本 | 生态割裂，agent 侧要自己适配 |
| 做 Obsidian 插件 | 绑定单一工具，且 v0.1 不该扩战场 |

---

## ADR-008 · 密钥与配置存储

**决策**：
- 非敏感配置：`<workdir>/config/settings.json`（明文 JSON，可读可改，含 provider 的 base_url/model/参数）
- **密钥**：Windows DPAPI `CryptProtectData`（用户态作用域）加密后存 `config/secrets.bin`；解密仅在内存中，落地不明文、不进日志、不进崩溃报告、不导出到 bundle
- 日志脱敏器：所有日志写入前过滤 `sk-*`、`Bearer *`、`*api_key*` 模式

**理由**
- 桌面单用户场景下 DPAPI 是"零依赖 + 系统级"的最优解（不需要用户设主密码，也不需要引入 SQLCipher）。
- 明文不落盘是底线；日志是密钥最常见的泄漏渠道，必须在写入层拦截而非事后清理。

**代价**
- DPAPI 密文与 Windows 用户账户绑定：换机/重装系统后必须重新输入 key（这点必须写进 FAQ，否则用户会困惑）。
- 导出配置给他人时密钥不会跟随（这是正确行为，但需 UI 说明）。
- 无法在 Linux/macOS 复用（未来需 keyring 适配，属 CHG-0004 的技术债）。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 明文存 `providers.json` | 直接违规 |
| 让用户每次启动输入 | 违背"一键运行"体验 |
| 引入 keytar / 系统钥匙串库 | 额外原生依赖，Windows 上 DPAPI 已够 |
| 自研加密 + 主密码 | 徒增用户负担且更易错 |
