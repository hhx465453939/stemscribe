# CHG-0001 · Repository Bootstrap & Architecture Contract

| 字段 | 值 |
|---|---|
| ID | CHG-0001 |
| status | **approved** |
| approved_at | 2026-11-17 |
| approved_by | owner（会话指令直接授权立项） |
| type | bootstrap / architecture contract |
| scope | 全仓库（Phase 0 → Phase 1 骨架） |
| owner_agent | primary |
| supersedes | — |

---

## 1. 背景与真实问题

### 1.1 现状（实测）

主人当前的人工流水线由四个独立 Windows 程序串成，靠手工搬文件：

| 环节 | 现用工具 | 实测位置 / 关键产物 |
|---|---|---|
| 视频获取 | bilibili-video-downloader（Tauri/Rust 客户端） | `E:\software\bilibili-video-downloader\*.exe` + 自带 ffmpeg |
| 人声分离 | Ultimate Vocal Remover (UVR) | `F:\software\Ultimate Vocal Remover\`，内含 `models/{MDX_Net_Models,VR_Models,Demucs_Models}`、`torch`、`onnxruntime` |
| 语音转写 | Buzz | `E:\software\Buzz\_internal\`，内含 `faster_whisper-1.0.3`、`ctranslate2`、`torch-2.2.1+cu121`、`onnxruntime-1.17.1`、`whisper.dll` |
| 语音转写（备） | WhisperDesktop（Const-me/Whisper） | `F:\software\WhisperDesktop\ggml-large-v3.bin`（3.1 GB）、`Whisper.dll`、`lz4.txt`（压缩权重缓存） |
| 文本润色 | 手工粘贴到任意 LLM / 智能体 | 无本地留痕，提示词无版本 |

本机算力实测：`NVIDIA GeForce RTX 3060 12 GB`、驱动 566.36、`CUDA 12.7`（`OrayIddDriver` 为远控虚拟显示适配器，非计算设备）。

### 1.2 痛点是"链路断裂"，不是"缺功能"

四个工具单独都能用，问题全部出在接缝上：

1. **模型散落**：UVR 模型在 `F:\...\models`，Whisper 权重在 `F:\...\WhisperDesktop`，Buzz 的 CTranslate2 权重另存一处，换机器/重装等于重配。
2. **手工搬运**：下载好的 mp4 要人工拖进 UVR，导出 vocals.wav 再拖进转写器，产物命名靠人。
3. **不可复现**：同一集视频两次跑，用的模型版本、分离强度、转写参数无人记录，结论无法回溯。
4. **润色无契约**：提示词在聊天窗口里，输出不进档案，模型能力（上下文窗口、推理深度）靠记忆猜。
5. **无法批量**：UP 主 200 集视频要重复 200 次手工操作。

### 1.3 真实目标（字面背后）

主人要的不是"再写一个下载器"或"再包一层 UVR 界面"，而是：

> 一条**可复现、可批量、产物可直接喂给 agent 讨论**的本地音视频文字化流水线，装在一个 Windows 客户端里，模型与运行时由 app 自己下载安装到工作目录，全流程离线可用，LLM 润色作为可选增值环节可自由换端点。

## 2. 目标（可判定终态）

| # | 终态 | 判定方式 |
|---|---|---|
| G-1 | 从「一条 B 站链接」到「带时间戳的结构化文字档案」一次点击完成，全程无人工搬文件 | e2e：输入 URL → 产出 bundle 全部 5 类文件 |
| G-2 | 模型与推理运行时由 app 下载、校验、安装到用户指定工作目录，可离线迁移（整目录拷走即用） | 断言：workdir 自包含，`runtimes/` + `models/` 拷贝到新机后无需重下 |
| G-3 | GPU 加速自动择优并可在显存不足时安全降级，不因 OOM 中途炸掉任务 | 单测覆盖 4 条降级分支；3060 12G 上跑 30 分钟音频不 OOM |
| G-4 | LLM 润色为可选阶段：可关、可换端点、模型列表可刷新、能力参数（含上下文窗口/推理深度/多模态/function call）可见可改 | mock provider 端到端测试 + UI 字段清单比对 |
| G-5 | 每份产物带完整 provenance（模型版本、参数快照、提示词 hash），可复现 | bundle 内 `meta.json` 字段校验 |
| G-6 | 开源可发布：CI 绿、tag 出安装包、仓库无敏感信息 | GitHub Actions 双 workflow 跑通；脱敏扫描 0 命中 |

## 3. 非目标（本轮明确不做）

- ❌ 不做视频编辑/剪辑/字幕时间轴精修 UI（只输出 srt/vtt，不做可视化调轴器）
- ❌ 不做云端转写/云端分离（隐私底线：音视频不出本机；仅润色阶段允许用户主动外发文本）
- ❌ 不做 macOS / Linux 首发（架构留跨平台位，首发只保证 Windows x64）
- ❌ 不做模型训练/微调（只做推理编排）
- ❌ 不做账号体系、云同步、多人协作
- ❌ 不内置任何 API Key，不提供代付额度

## 4. 架构决策（结论先行，细节见 `docs/DECISIONS.md`）

| ADR | 决策 | 代价（提前算） |
|---|---|---|
| ADR-001 | 桌面壳用 **Wails v2**（Go + WebView2），前端 Next.js 15 `output:'export'` 静态产物 | Windows 构建需 MSYS2/GCC；前端放弃 SSR（桌面无需）；Wails 生态小于 Tauri |
| ADR-002 | **Go 只做编排，不碰张量**：推理交给外置引擎进程（whisper.cpp 可执行 / Python embeddable + audio-separator），stdin-stdout JSONL 协议 | 需自建协议层与进程监管；首次需下载运行时 |
| ADR-003 | 模型与运行时按 **Ollama 模式** 分发：内置 registry + 分片断点续传 + sha256 校验 + 原子落盘（`.part` → rename） | 首次使用需联网下载 1–3 GB；需维护 registry；国内需镜像源配置 |
| ADR-004 | **资源预算调度器**：NVML 读显存 → VRAM 预算 → 模型 LRU 常驻 → 分段流式处理 → 四级降级链（CUDA→Vulkan→DirectML→CPU） | 调度复杂度高；需真实硬件测量而非拍脑袋 |
| ADR-005 | LLM 层统一抽象：**OpenAI-compatible 为主 + Anthropic 原生补充**，能力信息三层来源（内置能力库 / `/v1/models` / 用户手改覆盖） | 能力库需持续维护；未知模型默认保守值可能截断长文 |
| ADR-006 | 产物为**自包含 bundle**：`meta / source / audio / text / runs`，阶段级 checkpoint 可幂等续跑 | 磁盘占用（视频原文件默认保留，可配置为软链或删除） |
| ADR-007 | Agent 集成走**本地 MCP server**（stdio + HTTP）暴露 `search_transcripts / get_transcript / get_context_pack` | 需维护 MCP 协议版本；额外进程 |
| ADR-008 | 密钥走 **Windows DPAPI**（`CryptProtectData`），明文不落盘、不写日志 | 仅限 Windows 用户态；跨机迁移需重新输入 |

**为什么不是 Electron / Tauri**：Electron 打包 150 MB+ 且其 Node 主进程与 Go 后端职责重叠；Tauri 用 Rust 壳则 Go 只能做 sidecar，多一层进程与打包链，与「Go 是后端主语言」的要求背离。Wails 让窗口、IPC、子进程管理、文件系统落在同一个 Go 进程里，是这个技术栈组合下最短路径。

## 5. 交付物

### 5.1 A 阶段（本轮，纯契约与骨架文档）

```
stemscribe/
├── README.md  README.zh-CN.md  LICENSE  CHANGELOG.md
├── AGENTS.md  CONTRIBUTING.md  SECURITY.md
├── .gitignore  .editorconfig
├── docs/{ARCHITECTURE,PIPELINE,MODEL-REGISTRY,RESOURCE-SCHEDULER,
│         LLM-PROVIDERS,UI-DESIGN,DECISIONS,UPSTREAM-RESEARCH,ROADMAP}.md
├── docs/schema/{model-registry.schema.json,provider.schema.json,bundle-meta.schema.json}
└── tasks/CHG-0001/{SPEC.md,PRD.md,TRACEABILITY.md,CHG.md}
```

### 5.2 B 阶段（CHG-0001 关闭前必须落地的最小可构建骨架）

```
cmd/stemscribe/main.go        Wails 入口 + 单实例 + 崩溃日志
internal/{registry,downloader,engine,pipeline,provider,vault,resource,store}/
frontend/                     Next.js 15 + Tailwind + 设计 token
build/                        wails.json、windows manifest、图标
.github/workflows/{ci.yml,release.yml}
scripts/{dev.ps1,smoke-test.ps1}
```

## 6. 验收判据（每条都能红）

| AC | 判据 | 验证方式 |
|---|---|---|
| AC-01 | 5.1 列出的文件全部存在且非空 | `scripts/verify-contract.sh` 逐项 `test -s` |
| AC-02 | CHG 三件套齐全，SPEC 含 ≥12 条 AC，`status` 非 draft | 脚本 grep 字段 |
| AC-03 | `docs/DECISIONS.md` 覆盖 8 个 ADR，每个含「决策/理由/代价/被否决方案」四段 | grep 段落标题，缺段即红 |
| AC-04 | 三份 JSON Schema 通过 `ajv validate` 自身校验；registry 至少 3 条真实条目，`sha256` 允许 `null` 但必须带 `TBD` 标记 | `pnpm schema:check` |
| AC-05 | Go 骨架 `go build ./...` 成功、`golangci-lint run` 0 issue | CI job `go` |
| AC-06 | 前端 `pnpm build` 产出 `frontend/out/`，`pnpm lint` 0 error | CI job `web` |
| AC-07 | `wails build` 产出 exe，启动 3 秒内存活且日志出现 `engine ready` | `scripts/smoke-test.ps1` |
| AC-08 | 故意注入一处 Go 语法错误 → CI 变红；还原后变绿（记录两次 run 链接） | 人工验证并写入 TRACEABILITY |
| AC-09 | 推 `v0.0.1-rc1` tag → release 附件含 `Stemscribe-*-portable.zip`、`*-setup.exe`、`SHA256SUMS.txt` | 查 GitHub Release |
| AC-10 | 无 GPU 的 CI 上跑 30 秒测试音频 → `transcript.json` 段数 > 0 且 `start` 严格递增 | `go test ./internal/pipeline -run E2E` |
| AC-11 | 对 `httptest` mock OpenAI 服务完成模型刷新 + 一次润色，断言请求体含 temperature/max_tokens、SSE 流式解析正确 | `go test ./internal/provider` |
| AC-12 | 显存预算器单测覆盖「充足 / 不足 / 卸载后可容 / 全不可用」四分支，覆盖率 ≥ 90% | `go test -cover ./internal/resource` |
| AC-13 | 安装目录与配置目录 `grep -r "sk-"` 0 命中；密钥落盘字节不含明文（DPAPI 密文） | `scripts/secrets-scan.ps1` + 单测 |
| AC-14 | 开源脱敏扫描 0 命中（真实 IP/域名/用户名路径/密钥/桶名）；`git log --all -S` 无敏感串 | `scripts/redact-scan.sh` |
| AC-15 | 全流程 e2e：30 秒合成音频 → 分离 → 转写 → 润色（mock）→ 导出，bundle 5 类文件齐备 | `scripts/smoke-test.ps1 -Full` |

## 7. 独占文件清单（多 agent 并行防冲突）

| 范围 | 归属 |
|---|---|
| `docs/**`、`tasks/**`、`README*`、`AGENTS.md` | CHG-0001 A 阶段（已交付） |
| `cmd/**`、`internal/registry/**`、`internal/downloader/**`、`build/**`、`.github/**` | CHG-0001 B 阶段 |
| `internal/engine/**`、`runtimes/**` 清单 | CHG-0002（引擎与运行时） |
| `internal/pipeline/**`、`internal/store/**` | CHG-0003（流水线） |
| `internal/provider/**`、`internal/vault/**` | CHG-0004（LLM 层） |
| `frontend/**` | CHG-0005（界面） |
| `internal/resource/**`、`internal/mcpserver/**` | CHG-0006（调度与 Agent 集成） |

> 规则：任何 agent 不得跨 CHG 改他人独占目录；确需跨界的，先在该 CHG 的 `CHG.md` 写「跨界理由 + 影响面」，再动。

## 8. 风险与回滚

| 风险 | 概率 | 影响 | 缓解 |
|---|---|---|---|
| Wails + Next.js static export 集成摩擦（路由/资源路径） | 中 | 阻塞 B 阶段 | 先用 `basePath:''` + `assetPrefix` 相对路径；保底降级为 Vite+React（保留 Go 后端） |
| Windows 上打包 Python 运行时体积失控 | 高 | 安装包 > 1 GB | 运行时按需下载、不随包；embeddable + 精简依赖；ONNX-only 路径优先 |
| UVR 模型许可不允许再分发 | 中 | 无法内置 registry | 只存元数据 + 官方源直链，不镜像权重；UI 明示许可 |
| whisper.cpp CUDA 预编译二进制与本机驱动不匹配 | 中 | GPU 不可用 | 探测 `cudaVersion ≥ 12.x` 再选 CUDA 变体；失败自动落 Vulkan/CPU 并提示 |
| B 站下载接口变动 | 高 | 环节 1 断 | 引擎抽象 + 下载器可切换（内置 yt-dlp 兼容层作为备选） |
| 12 GB 显存跑 large-v3 爆内存 | 中 | 转写失败 | 默认 `medium`/量化 `q5_0`；大模型需 `-ngl` 分层 + 分段 |

**回滚**：A 阶段纯文档，`git revert` 即回到空仓库；B 阶段每个模块独立 tag（`v0.0.x`），故障模块可单独 revert 不影响契约。

## 9. 依赖与前置

- Go ≥ 1.23、Node ≥ 20、pnpm ≥ 9、MSYS2（Wails Windows 构建）
- WebView2 Runtime（Win10/11 常见自带）
- NVIDIA 驱动 ≥ 525（CUDA 12 路径）；无 GPU 时 CPU 路径必须可用
- GitHub Actions：`windows-latest`（构建）、`ubuntu-latest`（lint/契约校验）
