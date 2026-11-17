# CHG-0007 · 变更记录

> 契约主体见 `SPEC.md`（目标/判据/ADR/风险）、`PRD.md`（产品面）、`TRACEABILITY.md`（映射与实测）。
> 本文件只记状态流转与决策增补。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-17 | proposed | 立项：双主题 + 双语工程契约（SPEC_READY） | agent |
| 2026-11-17 | **approved** | owner 会话明确指令：「美术风格明亮风格+暗黑风格，两个都要有，还要中英双语」＋「剩余的开发你也继续做掉吧」→ 视为对本 SPEC 的批准 | owner |
| — | in_progress | M1–M5 实现与验证 | agent |
| — | closed | 待 AC-01…AC-10 全绿后关闭 | — |

> 按 `/ai-spec`：`SPEC_READY` ≠ 批准。本行 `approved` 的依据是 owner 的**明确书面指令**（上表引用原文），
> 不是 agent 自行假定。commit 与 push 仍需单独授权（见 SPEC §7）。

## 流程失误登记（自我纠正）

| 事实 | 说明 |
|---|---|
| 违反顺序 | owner 提出需求后，我**先写了 6 个文件**（`globals.css`/`layout.tsx`/`Providers.tsx`/`Sidebar.tsx`/`page.tsx`/`i18n.ts`），然后才补立项 |
| 违反的规则 | 项目 `AGENTS.md`：「无对应 CHG 就不动手，先立项」 |
| 处置 | 不移除已写代码（避免做无用功），但在 `TRACEABILITY.md` §2 标记为「已写、未验证」，不计入完成；owner 否决则该批文件整体 revert |
| 已发现的真实缺陷 | `--text-3` 两主题均 <4.5:1、明亮 `--lamp` 作为文字 3.7:1 —— 已在 `TRACEABILITY.md` §3.1 登记为 M3 必修项 |

## 决策增补

| # | 增补 | 理由 |
|---|---|---|
| N-1 | 明亮主题**不是**暗色的反色，而是同一基因的原相 | 直接反色会得到发灰的白与发脏的黑，并丢掉「单一光源」语义（ADR-009） |
| N-2 | i18n 不引第三方库，用扁平字典 + 类型强制 parity | 100 条量级，库带来的路由/运行时成本远超收益，且会破坏静态导出（ADR-010） |
| N-3 | 语言名固定显示 `中文` / `EN`，不随界面语言翻译 | 把「中文」翻成「Chinese」对不懂英文的人是不可用（已知陷阱） |
| N-4 | 偏好解析顺序 URL > localStorage > 系统，且用 head 内联脚本预绘制 | 静态导出无服务端，只能靠预绘制避免 FOUC（ADR-011） |

## 下一步（依赖顺序）

1. **M3 先修对比度**（已知不达标，优先级高于新功能）→ 用脚本复测并回填 `TRACEABILITY.md` §3.1
2. M2 收尾：`api.ts` 的 `stageKey()` + 剩余 7 个组件接入 `t()`
3. M5 守卫：`scripts/i18n-check.sh`（tokens / parity / literal 三子命令）+ 接入 CI
4. M4 截图：明/暗 × 中/英 四组合，各取 `/` 与 `/settings`
5. README 中英双版同步主题章节与截图；`docs/UI-DESIGN.md` §3 补双主题 token 表
6. 全量验证：`pnpm lint && pnpm run typecheck && pnpm run build` + `go build/test`
7. commit（owner 授权）→ push（单独授权）→ 观察 CI
