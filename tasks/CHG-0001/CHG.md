# CHG-0001 · 变更记录

> 契约主体见 `SPEC.md`（目标/判据/风险）、`PRD.md`（产品面）、`TRACEABILITY.md`（映射与实测登记）。
> 本文件只记**状态流转与决策增补**，不重复上面内容。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-17 | proposed | 立项：新建仓库骨架与架构契约 | agent |
| 2026-11-17 | **approved** | 主人指令「先新建目录，然后写 CHG，我去新建 GitHub 仓库」→ 视为立项授权 | owner |
| — | in_progress | A 阶段（文档契约）交付 | agent |
| — | — | B 阶段（可构建骨架 + CI 双绿）待开工 | — |
| — | closed | 待 AC-01…AC-09 全绿后关闭 | — |

## A 阶段交付清单（本轮）

| 文件 | 作用 |
|---|---|
| `docs/DECISIONS.md` | 8 条 ADR（决策/理由/代价/被否决方案） |
| `docs/ARCHITECTURE.md` | 分层、进程模型、IPC 契约、目录布局 |
| `docs/PIPELINE.md` | 阶段 DAG、事件协议、checkpoint、错误模型 |
| `docs/MODEL-REGISTRY.md` | registry schema、下载校验、后端变体、许可 |
| `docs/RESOURCE-SCHEDULER.md` | 显存预算、LRU、分段、降级链（对应主人点名的 CUDA/内存缓存） |
| `docs/LLM-PROVIDERS.md` | provider 抽象、能力探测三层、参数面 |
| `docs/UI-DESIGN.md` | taste 基因落地（暗色瑞士平面 × 单一聚光） |
| `docs/UPSTREAM-RESEARCH.md` | 四个参考工具的机制对照与复用点（含实测证据） |
| `docs/ROADMAP.md` | CHG-0001…0006 排布与里程碑 |
| `docs/schema/*.json` | 三份 JSON Schema（registry / provider / bundle-meta） |
| `tasks/README.md` | CHG 流程与编号规则 |
| `README.md` / `README.zh-CN.md` | 仓库门面 |
| `AGENTS.md` | 项目内 agent 约定（与全局 AGENTS.md 叠加生效） |
| `LICENSE` / `CONTRIBUTING.md` / `SECURITY.md` / `CHANGELOG.md` / `.gitignore` / `.editorconfig` | 开源发布基座 |

## 决策增补（A 阶段讨论中新增，需回溯到 DECISIONS.md）

| # | 增补 | 理由 |
|---|---|---|
| N-1 | 项目名从候选 `Sonoglyph` / `Auralith` / `Klangwerk` 改为 **Stemscribe** | 前三个均在 GitHub / 域名 / PyPI 撞名（实测搜索），`Stemscribe` 无同名项目且语义 = stem separation + scribe |
| N-2 | 首发只保证 Windows x64，但引擎层与路径层不写死 `\` | 未来 macOS 只需换壳与引擎二进制 |
| N-3 | 润色阶段强制保留时间戳锚点，丢失则降级为原文 | 防止 LLM 改写导致档案失去可回溯性 |
| N-4 | registry 的 `sha256` 字段在 A 阶段允许 `null` + `TBD` | 不编造校验值；CHG-0002 实测后回填 |

## B 阶段跨界授权（跨 CHG 独占目录）

**理由**：CHG-0001 的 B 阶段判据 AC-05…AC-15 横跨引擎、流水线、LLM、界面、调度五个模块，
必须在本阶段把**可编译、可测试、可构建**的骨架一次性立起来，否则 AC 无法逐条变绿。
因此 B 阶段对下列目录做**一次性骨架建设**，不做功能实现；后续 CHG 在其上继续，不重复造地基。

| 目录 | 名义归属 | B 阶段承诺范围 | 后续 CHG 接续 |
|---|---|---|---|
| `internal/engine/**` | CHG-0002 | 仅协议编解码 + 进程监管骨架 + 单测 | CHG-0002 接真实二进制接入与降级 |
| `internal/resource/**` | CHG-0006 | 仅显存预算器 + LRU 决策 + 四分支单测（AC-12） | CHG-0006 接 NVML 实测与并发控制 |
| `internal/pipeline/**` | CHG-0003 | 仅阶段 DAG + checkpoint + 对齐修复 + 注入式引擎单测（AC-10） | CHG-0003 接真实 ffmpeg/whisper 路径 |
| `internal/store/**` | CHG-0003 | 仅 bundle 路径 + meta 原子读写 + 恢复判定 | CHG-0003 接 SQLite/FTS 与导出 |
| `internal/provider/**` | CHG-0004 | 仅协议适配 + 能力合并 + 分块 + mock 测试（AC-11） | CHG-0004 接真实端点与长文分块调优 |
| `internal/vault/**` | CHG-0004 | 仅 DPAPI 加解密 + 单测（AC-13） | CHG-0004 接密钥生命周期与迁移 |
| `frontend/**` | CHG-0005 | 仅设计 token + 四页骨架 + 构建通过（AC-06） | CHG-0005 接完整交互与波形 |

**影响面**：不改变任何 AC 的归属；B 阶段只提供「能构建 + 能单测」的最小实现，功能完整性仍由各自 CHG 的 AC 判定。

## 下一步（B 阶段开工顺序，不可跳步）

1. `wails init` + Next.js static export 打通（**首日必验白屏风险**）
2. `go build` + `pnpm build` + `wails build` 三绿 → 落 `ci.yml`
3. `scripts/verify-contract.sh` + `secrets-scan` + `redact-scan`
4. 故意注入错误验红 → 还原验绿 → 回填 `TRACEABILITY.md` AC-08
5. `release.yml` + 打 `v0.0.1-rc1` 验附件
6. 关闭 CHG-0001，开 CHG-0002
