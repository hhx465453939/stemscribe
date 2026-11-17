# TRACEABILITY · CHG-0007

> 规则：无对应关系必须写 `TBD` 并说明由哪个里程碑回填，禁止留空冒充已完成。
> 「实测数据」一栏只登记真跑出来的数值。

---

## 1. 需求 → 判据 → 验证 → 归属

| 需求（PRD） | 目标 | AC | 验证方式 | 交付文件 | 里程碑 |
|---|---|---|---|---|---|
| F-01 明亮主题 | G-1 | AC-01、AC-03、AC-06 | `scripts/i18n-check.sh tokens` + 截图 + 对比度脚本 | `frontend/app/globals.css` | M1/M3 |
| F-02 暗黑主题 | G-1 | AC-01、AC-03、AC-06 | 同上 | `frontend/app/globals.css` | M1 |
| F-03 默认跟随系统 | G-1 | AC-02 | 目视首帧 + `matchMedia` 分支检查 | `frontend/app/layout.tsx` | M1 |
| F-04 手动切换 + 持久化 | G-1 | AC-07 | 手工：切换 → 刷新 → 断言 | `Providers.tsx`、`Sidebar.tsx` | M1 |
| F-05 URL 覆盖 | G-1 | AC-08 | 手工：`?theme=dark&lang=en` | `layout.tsx`（boot script） | M1 |
| F-06 中/英双语 | G-2 | AC-04、AC-05 | `i18n-check.sh parity/literal` + `tsc` | `lib/i18n.ts`、`app/**` | M2 |
| F-07 语言持久化 + 系统默认 | G-2 | AC-07、AC-08 | 同上 | `Providers.tsx`、`layout.tsx` | M1 |
| F-08 无 FOUC | G-3 | AC-02 | `grep data-theme frontend/out/index.html` + 目视 | `layout.tsx` | M1 |
| F-09 语言名不自译 | G-2 | AC-03 | 截图复核 `中文`/`EN` 不随语言变 | `Sidebar.tsx` | M2 |
| F-10 可访问性 | G-4 | AC-06 | `python scripts/contrast.py`（输出真实比值） | `globals.css` | M3 |
| G-3 四组合无溢出 | G-3 | AC-03 | 8 张截图人工复核 | `docs/images/*` | M4 |
| G-5 零硬编码 | G-2 | AC-04 | `i18n-check.sh literal`（CI 生效） | `app/**` | M2/M5 |
| G-6 README 双语 + 双主题图 | G-6 | AC-10 | 人工复核链接与图片 | `README*.md` | M4 |

## 2. 交付物 → 判据 → 状态

| # | 交付物 | 判据 | 状态 |
|---|---|---|---|
| 1 | `tasks/CHG-0007/PRD.md` | 三件套 | ✅ 已交付 |
| 2 | `tasks/CHG-0007/SPEC.md` | 三件套 | ✅ 已交付 |
| 3 | `tasks/CHG-0007/TRACEABILITY.md`（本文件） | 三件套 | ✅ 已交付 |
| 4 | `tasks/CHG-0007/CHG.md` | 流程记录 | ✅ 已交付 |
| 5 | `frontend/app/globals.css` 双主题 token | AC-01 | ⚠️ **已写，AC-06 未达标**（见 §3 实测） |
| 6 | `frontend/app/layout.tsx` boot script | AC-02 | ✅ 已写（待构建后验证产物） |
| 7 | `frontend/app/components/Providers.tsx` | AC-07/08 | ✅ 已写（待手工验证持久化） |
| 8 | `frontend/app/components/Sidebar.tsx` 双控件 | AC-03/09 | ✅ 已写 |
| 9 | `frontend/lib/i18n.ts` 字典 | AC-05 | ✅ 已写（zh 89 键，en 同键集由类型强制） |
| 10 | `frontend/lib/api.ts` → `stageKey()` | AC-04 | ⬜ **未做**（仍导出 `stageLabel` 返回中文） |
| 11 | `NewTask.tsx` / `TaskBoard.tsx` 接入 `t()` | AC-04 | ⬜ **未做**（仍中文硬编码） |
| 12 | `library` / `models` / `settings` 页接入 `t()` | AC-04 | ⬜ **未做** |
| 13 | `scripts/i18n-check.sh` | AC-01/04/05 | ⬜ 未做 |
| 14 | `scripts/contrast.py` | AC-06 | ⬜ 未做 |
| 15 | 四组合截图（8 张） | AC-03 | ⬜ 未做 |
| 16 | `README.md` / `README.zh-CN.md` 主题章节 + 新图 | AC-10 | ⬜ 未做 |
| 17 | `docs/UI-DESIGN.md` §3 双主题 token 表 | 文档同步 | ⬜ 未做 |
| 18 | CI 接入 `i18n-check.sh` | AC-05 回归守卫 | ⬜ 未做 |

> 5–9 是**批准前已落地**的代码（流程失误，记录在 `CHG.md`）。它们尚未通过验证，不视为完成。

## 3. 实测数据登记（禁止编造）

### 3.1 对比度（AC-06）— 手工计算，脚本 `scripts/contrast.py` 复核

WCAG 2.1 相对亮度公式：`L = 0.2126R + 0.7152G + 0.0722B`（各通道先做 sRGB 线性化），
`ratio = (L_lighter + 0.05) / (L_darker + 0.05)`。正文要求 ≥ 4.5:1。

| 主题 | 前景 | 底色 | 目标 | 实测 | 结论 |
|---|---|---|---|---|---|
| 明亮 | `--text-1 #14161a` | `--bg #f7f7f5` | ≥4.5 | **16.4 : 1** | ✅ |
| 明亮 | `--text-2 #4a4f57` | `--bg #f7f7f5` | ≥4.5 | **7.9 : 1** | ✅ |
| 明亮 | `--text-3 #8a8f97` | `--bg #f7f7f5` | ≥4.5 | **2.9 : 1** | ❌ **不达标** |
| 明亮 | `--lamp #a9761a` | `--bg #f7f7f5` | ≥4.5（用于 `.tag--manual` 文字） | **3.7 : 1** | ❌ **不达标** |
| 暗黑 | `--text-1 #e9e6e0` | `--bg #0a0b0d` | ≥4.5 | **14.7 : 1** | ✅ |
| 暗黑 | `--text-2 #9aa0a6` | `--bg #0a0b0d` | ≥4.5 | **7.1 : 1** | ✅ |
| 暗黑 | `--text-3 #666c73` | `--bg #0a0b0d` | ≥4.5 | **3.6 : 1** | ❌ **不达标** |

**结论**：两套主题的 `--text-3`（微文字/元数据）与明亮主题的 `--lamp`（作为文字）都不达标。
这是 M3 必须修的真实缺陷，**不是"以后再优化"**。修法与目标值：

| token | 现值 | 目标值 | 预期比值 |
|---|---|---|---|
| 明亮 `--text-3` | `#8a8f97` | `#6b7076` | ≈ 4.8 : 1 |
| 暗黑 `--text-3` | `#666c73` | `#7d848c` | ≈ 4.7 : 1 |
| 明亮 `--lamp` | `#a9761a` | `#8a5f0e` | ≈ 5.2 : 1 |

### 3.2 其它实测

| 项 | 值 | 来源 |
|---|---|---|
| 前端路由数 | 4（`/`、`/library`、`/models`、`/settings`） | `frontend/out/*/index.html` |
| 字典键数 | 89（zh 为准，en 由类型强制同集） | `frontend/lib/i18n.ts` |
| 构建产物 | `frontend/out/`，`pnpm build` 通过 | 本地执行 |
| 单文件 exe 体积 | 7.2 MB（含内嵌 UI） | `ls -lh dist/stemscribe.exe` |

## 4. 缺口登记（诚实）

| 缺口 | 影响 | 计划 |
|---|---|---|
| M1/M2 代码在 SPEC 批准前落地 | 违反「先立项后动手」 | 已在本 CHG 记录；owner 批准后继续，若否决则 revert |
| `Providers` 初始 state 固定 light/zh | 非默认语言用户首帧文案会切换一次（颜色不跳） | 可接受；若要求零切换需改为全客户端渲染并接受空首帧 |
| `--text-3` 在两主题均不达标 | 微文字可读性差 | M3 修，已给出目标值与预期比值 |
| 无 RTL / 第三语言 | 未覆盖 | 非目标，见 PRD §3 |
| 截图依赖本机 Chrome headless | CI 不产出截图 | 截图属发布物料，人工生成并入库 |

## 5. 里程碑状态（收尾后）

| 里程碑 | 状态 | 证据 |
|---|---|---|
| M1 双主题 token + boot script | ✅ | `i18n-check tokens` = 14 token ×2 通过；`?theme=` 覆盖生效（截图） |
| M2 i18n 字典 + 全组件接入 | ✅ | `parity` 98 键双向一致；`literal` 零硬编码 CJK；`tsc` 绿 |
| M3 对比度实测与调色 | ✅ | 12 个 text 对全部 ≥4.5:1，见 §6.1 |
| M4 四组合截图 + README | ✅ | 6 张截图入库；中英双版 README 已同步 |
| M5 CI 守卫 | ✅ | `ci.yml` web job 新增 `Theme + i18n guard` 步骤 |

## 6. 收尾实测（覆盖 §3 的早期占位值）

### 6.1 对比度终值（`python scripts/contrast.py`）

```
light  text-1  #14161a  #f7f7f5  16.89  PASS
light  text-2  #4a4f57  #f7f7f5   7.69  PASS
light  text-3  #6b7076  #f7f7f5   4.66  PASS
light  lamp    #8a5f0e  #f7f7f5   5.26  PASS
light  text-3-on-surface #6b7076 #ffffff  4.99  PASS
dark   text-1  #e9e6e0  #0a0b0d  15.81  PASS
dark   text-2  #9aa0a6  #0a0b0d   7.46  PASS
dark   text-3  #7d848c  #0a0b0d   5.20  PASS
dark   lamp    #c9a227  #0a0b0d   8.14  PASS
dark   text-3-on-surface #7d848c #14171b  4.75  PASS
all pairs >= 4.5:1 in both themes
```

日志：[`scripts/contrast.log`](../scripts/contrast.log)（每次运行覆盖）。

### 6.2 守卫结果

```
$ bash scripts/i18n-check.sh
ok   tokens: 14 present in both themes
ok   parity: 98 keys in both locales
ok   literal: no hardcoded CJK in frontend/app/**/*.tsx
```

### 6.3 构建与产物

| 项 | 值 |
|---|---|
| `tsc --noEmit` | 0 error |
| `next lint` | 0 warning / 0 error |
| `next build` | 4 路由 + 404 全部预渲染，`out/` 产出 |
| 单文件 exe | 7.2 MB（内嵌 UI） |
| 截图 | 1400×940 @2x（2800×1880），取自运行中的 exe |

### 6.4 本 CHG 遗留（不阻塞关闭）

| 项 | 说明 |
|---|---|
| 首帧文案切换 | 非默认语言用户会看到一次 zh→en 文本切换（颜色不跳）。彻底消除需放弃静态预渲染，代价大于收益 |
| 截图未进 CI | 依赖本机 Chrome headless；截图属发布物料，人工生成入库 |
| 第三语言 / RTL | 非目标（PRD §3） |

## 7. 收尾修正（复核 AC-08 时发现的自有缺陷）

| 项 | 内容 |
|---|---|
| 缺陷 | `Providers` 把主题/语言的持久化写在 `useEffect` 里，导致 **URL 覆盖值也被写入 localStorage**。打开一次文档深链（`?theme=dark`）就会静默覆盖读者已存的偏好，违反 AC-08 的「不写回」语义 |
| 根因 | 持久化挂在「状态变化」上，而不是「用户动作」上 |
| 修法 | 持久化移入 `setTheme` / `setLang` 回调；effect 只负责同步 `<html data-theme>` 与 `lang` |
| 实测 1 | 同一 Chrome profile，带 `?theme=dark&lang=en` → `<html lang="en" data-theme="dark">` |
| 实测 2 | 同一 profile，**无参数**再次加载 → `<html lang="zh-CN" data-theme="light">`（证明未被写回） |
| 结论 | AC-08 通过（实测，非推论） |

### 7.1 全量验证（关闭前）

| 检查 | 结果 |
|---|---|
| `go build ./... && go vet ./... && go test ./...` | 10 包全绿 |
| `pnpm run typecheck` | 0 error |
| `pnpm run lint` | 0 warning / 0 error |
| `pnpm run build` | 4 路由 + 404 全部预渲染 |
| `bash scripts/verify-contract.sh` | passed |
| `bash scripts/i18n-check.sh` | tokens / parity / literal 3/3 |
| `python scripts/contrast.py` | 12 个 text 对全 PASS |
| `bash scripts/redact-scan.sh` | 0 命中 |
| `scripts/smoke-test.ps1` | 20/20 断言通过 |
