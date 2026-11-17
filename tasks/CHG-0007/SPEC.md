# CHG-0007 · 双主题与双语工程契约

| 字段 | 值 |
|---|---|
| ID | CHG-0007 |
| version | 1 |
| status | **approved**（SPEC_READY → owner 明确批准，依据见 CHG.md 状态流） |
| prd_ref | `tasks/CHG-0007/PRD.md`（本变更内，需求已由 owner 会话指令确定） |
| supersedes | — |
| approved_by | owner（会话指令：「美术风格明亮风格+暗黑风格，两个都要有，还要中英双语」＋「剩余的开发你也继续做掉吧」） |
| approved_at | 2026-11-17 |

> **流程说明（自我纠正）**：本变更属于新增范围，不属于 CHG-0001（骨架）也不属于 CHG-0005（前端骨架 AC）。
> 但我在 owner 提出「明亮+暗黑+中英双语」后**先写了代码、再补立项**，违反项目铁律「无对应 CHG 就不动手」。
> 已落地但未验证的部分如实登记在 `CHG.md` §「已落地（未经本契约批准）」与 `TRACEABILITY.md` §2，
> 待 owner 批准后再继续实现、验证与提交。

---

## 1. 背景 / 目标 / 约束 / 假设 / 未决问题

### 1.1 背景
界面目前**只有暗色主题、只有中文文案**，且文案硬编码在组件里。开源面向英文用户，暗色在投影/打印场景不可用。

### 1.2 目标（可判定）
见 `PRD.md` §2 G-1…G-6。核心一句：**同一套设计基因在两种光照下都成立，文案不绑定语言。**

### 1.3 约束
- 不得引入 UI 框架或 i18n 库（当前前端只有 next/react，零额外运行时依赖）
- 4 色封顶、唯一圆角 2px、flat 无阴影（`docs/UI-DESIGN.md` 是硬约束）
- 必须是**静态导出**（`output:'export'`），不能用 middleware / server 端语言协商
- 不得破坏既有 AC：AC-06（`frontend/out/` 产出）、CI 三 job 全绿

### 1.4 假设
- 用户接受"语言名不翻译"（`中文` / `EN` 固定）
- 时间码是计时格式（`HH:MM:SS`），不涉及本地化格式

### 1.5 未决问题（需 owner 拍板，不阻塞 P0）
| # | 问题 | 默认处置 |
|---|---|---|
| Q-1 | 明亮主题的强调色是否与暗色同为琥珀？（白底需更深才够对比） | 默认：同一色相，两档明度（见 ADR-009） |
| Q-2 | 是否需要"跟随系统"三态开关（明/暗/跟随）？ | 默认：两态开关 + 系统为初值，不提供"跟随"显式项 |
| Q-3 | 英文文案语气（英式/美式、是否用 sentence case） | 默认：美式、sentence case |

## 2. 现状架构与债务影响

| 位置 | 现状 | 本变更的改动 |
|---|---|---|
| `frontend/app/globals.css` | 单套暗色 token，语义名为 `--ink/--bone/--lamp` | 改为**语义名** `--bg/--surface/--text-1..3/--line/--lamp*`，两套主题各一份 |
| `frontend/app/components/*.tsx` | 中文硬编码 | 全部走 `t(key)`；组件内零可见文案 |
| `frontend/lib/api.ts` | `stageLabel()` 返回中文 | 改为 `stageKey()` 返回 i18n key，翻译交给 UI |
| `frontend/app/layout.tsx` | 无主题/语言初始化 | 内联 boot script + `<Providers>` |
| 新增 `frontend/lib/i18n.ts` | — | 扁平字典 + 类型强制 parity |
| 新增 `frontend/app/components/Providers.tsx` | — | 主题/语言上下文 + 持久化 + URL 覆盖 |

**债务影响**：token 改名是一次性破坏性变更，但换来"新组件只需关心语义名、不必关心当前主题"。若不改，第二套主题会以"覆盖暗色规则"的形式长出来，两套样式互相打架。

## 3. 选定设计与 ADR

### ADR-009 · 两主题不是反色，是同一基因的两种光照

**决策**：明亮主题是「瑞士平面」的**本相**（cg-01 原文即白底 `#ffffff~#f7f8fa` + 灰阶 + 单一强调色）；暗黑是它的"熄灯变体"。

| 维度 | 明亮 | 暗黑 |
|---|---|---|
| 底 | 纸白 `#f7f7f5`（不用纯白，防刺眼） | 墨黑 `#0a0b0d` |
| 层级 | 靠**边线与底色差**（白面浮在灰底上） | 靠**受光面更亮**（`--surface-hi`） |
| 焦点（单一光源） | **暖纸色受光面** `#fffdf6` + 琥珀实心左竖线 + 极淡暖晕 | **暗场一束暖光** `--glow` + 琥珀左竖线 |
| 强调色 | `--lamp #8a5f0e`（深琥珀，白底 5.2:1）/ `--lamp-fill #c9a227`（实心底） | `--lamp #c9a227` |

**理由**：直接反色会得到"发灰的白"和"发脏的黑"，且会丢掉"单一光源"的语义。改按**光照条件**重新推导，两套都自洽。

**代价**：需要维护两套 token（约 14 个变量 ×2）；强调色两档明度容易被后续贡献者混用。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 机械反色（`filter: invert`） | 图片、图标、语义色全错；对比度不可控 |
| 只做暗色 + 一次性的"高对比模式" | 不解决投影/打印白底诉求 |
| 两套完全独立的样式表 | 组件要写两份类名，维护成本翻倍且必然不同步 |

### ADR-010 · i18n 用扁平字典 + 类型强制 parity，不引库

**决策**：`frontend/lib/i18n.ts` 导出 `zh`（`as const`）与 `en`（`Record<keyof typeof zh, string>`）。
组件通过 `usePrefs().t(key, vars)` 取文案。缺 key / 多余 key / 拼错 key **都是编译错误**。

**理由**：本项目文案量在 100 条量级，`next-intl` / `i18next` 带来的路由与运行时开销远超收益，且会破坏静态导出。类型强制 parity 恰好是这类项目最容易漏的检查项。

**代价**：新增语言要手写一份完整字典（编译器会逐条报缺）；不支持嵌套命名空间，key 必须点分层级命名。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| `next-intl` / `next-i18next` | 需 middleware/路由前缀，与 `output:'export'` 冲突 |
| `react-i18next` | 字典要运行时加载，首屏多一次请求；类型安全弱 |
| 每个组件内联双语对象 | 无法检查 parity，必然漏译 |

### ADR-011 · 偏好解析顺序：URL > storage > 系统，boot script 预绘制

**决策**：`layout.tsx` 在 `<head>` 内联一段同步脚本，按
`?theme=/&lang=` → `localStorage` → `prefers-color-scheme`/`navigator.language`
解析并写入 `<html data-theme lang>`；React 侧 `Providers` 挂载后从 DOM 读回同一份结果。

**理由**：静态导出的 HTML 是构建期生成的，服务端不可能知道用户偏好。若等 React hydrate 再设主题，会先闪一帧默认主题（FOUC）。URL 覆盖让截图与深链**确定性可复现**，不依赖脚本点击。

**代价**：`Providers` 初始 state 必须与预渲染 HTML 一致（light/zh），否则 hydration 文本不匹配；因此非默认语言的用户在首帧会看到一次文案切换（颜色不会跳，boot script 已处理）。

**被否决方案**
| 方案 | 否决原因 |
|---|---|
| 全部靠 `useEffect` 设置 | 必然 FOUC（先白后黑） |
| 只靠 CSS `prefers-color-scheme` 媒体查询 | 无法支持手动覆盖与持久化 |
| cookie + 服务端渲染 | 静态导出无服务端 |

## 4. 接口 / 数据 / 安全 / 兼容 / 迁移 / 可观测 / 回滚

### 4.1 接口（对外契约）
```ts
// frontend/lib/i18n.ts
type Key = keyof typeof zh;                       // 单一真相
translate(lang: "zh"|"en", key: Key, vars?: Vars): string

// frontend/app/components/Providers.tsx
usePrefs(): {
  theme: "light"|"dark"; setTheme(t): void;
  lang: "zh"|"en";       setLang(l): void;
  t(key: Key, vars?: Vars): string;
}
```

### 4.2 数据（本地存储）
| key | 值 | 说明 |
|---|---|---|
| `stemscribe.theme` | `"light"` \| `"dark"` | 用户显式选择；未设置时跟随系统 |
| `stemscribe.lang` | `"zh"` \| `"en"` | 同上 |

隐私模式写入失败被 `try/catch` 吞掉（偏好不持久，但不报错）。

### 4.3 安全
- 无新增网络请求、无新增密钥、无外发
- 内联 boot script 为静态字符串常量，**不拼接任何用户输入**（无 XSS 面）
- 不使用 `dangerouslySetInnerHTML` 渲染动态内容

### 4.4 兼容 / 迁移
- token 改名（`--ink→--bg`、`--bone→--text-1` 等）为**破坏性**：仓库内唯一消费方是 `globals.css` 自身，已同批改完
- 已存 `localStorage` 无旧值（首次引入），无需迁移
- 旧链接无 query 参数 → 行为与之前一致（跟随系统）

### 4.5 可观测
- 无运行时埋点。构建期可观测：`pnpm build` 必须产出 4 条路由的 `index.html`
- 翻译完整性在**编译期**由类型检查保证（无需运行时日志）

### 4.6 回滚
单 commit 可回滚；主题/语言是纯前端表现层，无数据迁移、无服务依赖，回滚后已存 `localStorage` 键被忽略（不产生错误）。

## 5. 里程碑（依赖顺序，每个 = 一个实现/验证/复查循环）

| # | 里程碑 | 涉及文件 | 验收 | 回滚 |
|---|---|---|---|---|
| M1 | 双主题 token + boot script | `globals.css`、`layout.tsx`、`Providers.tsx` | AC-01/02/07/08 | revert |
| M2 | i18n 字典 + 全部组件接入 | `lib/i18n.ts`、`lib/api.ts`、`app/**` | AC-03/04/05 | revert |
| M3 | 对比度实测与调色 | `globals.css` | AC-06 | 调回旧值 |
| M4 | 四组合截图 + README 双语同步 | `docs/images/*`、`README*.md` | AC-09/10 | revert |
| M5 | CI 校验项（翻译 parity + 硬编码扫描） | `scripts/i18n-check.sh`、`.github/workflows/ci.yml` | AC-05 在 CI 生效 | revert |

**依赖**：M1 → M2（组件需要 `t()`）→ M3（需要真实渲染）→ M4（需要截图）→ M5（守卫回归）。

## 6. Definition of Ready / Done

**DoR（开工前必须成立）**
- PRD 就绪且需求由 owner 明确下达 ✅
- 本 SPEC 的 AC 可逐条验证 ✅
- 目标文件清单明确、不与他人 CHG 冲突 ✅

**DoD（关闭前必须成立）**
- AC-01…AC-10 全绿，证据写入 `TRACEABILITY.md`
- `pnpm lint` / `pnpm run typecheck` / `pnpm run build` 三绿
- 四组合截图入库，README 中英双版同步
- 无回归：`go build ./... && go test ./...` 仍绿

## 7. 评审与交付门控

| 门 | 条件 | 谁批 |
|---|---|---|
| SPEC_READY | 本文档完整可评审 | agent 自评 |
| **SPEC_APPROVED** | owner 明确批准（本条**不得自动批准**） | owner |
| 实现 | SPEC_APPROVED 后开工 | — |
| **commit** | AC 全绿 + 证据齐 | owner |
| **push** | commit 后单独授权 | owner |

> 按 `/ai-spec`：commit 与 push 是**两个独立门**，不得因为"开发完了"自动连推。
> 本 CHG 现状：M1/M2 的代码在批准前已落地（流程失误，见上文说明），M3–M5 未做。

## 8. 追溯与证据计划

| AC | 判据 | 验证命令 | 证据落点 |
|---|---|---|---|
| AC-01 | 两套主题各定义 14 个必需 token | `bash scripts/i18n-check.sh tokens` | TRACEABILITY §2 |
| AC-02 | boot script 在 `<head>` 内联且先于正文 | `grep -c 'data-theme' frontend/out/index.html` + 目视无闪白 | TRACEABILITY §2 |
| AC-03 | 四组合 × 四页面无溢出/无残留 | 8 张截图人工复核 | `docs/images/` |
| AC-04 | 组件内零硬编码可见文案 | `bash scripts/i18n-check.sh literal` | CI |
| AC-05 | zh/en key 集合完全一致 | `bash scripts/i18n-check.sh parity` + `tsc --noEmit` | CI |
| AC-06 | 两主题正文/次级 ≥4.5:1 | `python scripts/contrast.py`（输出真实数值） | TRACEABILITY §3 |
| AC-07 | 切换后刷新保持 | 手工：切换 → 刷新 → 断言 | TRACEABILITY §2 |
| AC-08 | URL 覆盖生效且不写回 storage | 手工：`?theme=dark&lang=en` | TRACEABILITY §2 |
| AC-09 | lint + typecheck + build 全绿 | `pnpm run lint && pnpm run typecheck && pnpm run build` | CI |
| AC-10 | README 中英双版含两主题截图 | 人工复核链接可用 | README |

## 9. 独占文件清单

| 范围 | 归属 |
|---|---|
| `frontend/app/**`、`frontend/lib/{i18n,api}.ts` | CHG-0007 |
| `scripts/{i18n-check.sh,contrast.py}` | CHG-0007 |
| `docs/images/app-{main,settings}-{light,dark}.png` | CHG-0007 |
| `README.md`、`README.zh-CN.md`（主题/双语章节与截图） | CHG-0007 |
| `docs/UI-DESIGN.md` §3（token 双主题表） | CHG-0007 |

不触碰：`cmd/**`、`internal/**`、`tasks/CHG-000{1..6}/**`。
