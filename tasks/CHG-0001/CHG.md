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

## 下一步（B 阶段开工顺序，不可跳步）

1. `wails init` + Next.js static export 打通（**首日必验白屏风险**）
2. `go build` + `pnpm build` + `wails build` 三绿 → 落 `ci.yml`
3. `scripts/verify-contract.sh` + `secrets-scan` + `redact-scan`
4. 故意注入错误验红 → 还原验绿 → 回填 `TRACEABILITY.md` AC-08
5. `release.yml` + 打 `v0.0.1-rc1` 验附件
6. 关闭 CHG-0001，开 CHG-0002
