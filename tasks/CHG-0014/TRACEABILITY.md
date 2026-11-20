# CHG-0014 · TRACEABILITY

## 1. 需求 → AC → 验证

| 需求 | 来源 | AC | 验证 | 落点 |
|---|---|---|---|---|
| 句级切分生效 | U-2 | AC-01 | 重跑 01 集，统计行宽 | `frontend/app/components/NewTask.tsx`、`internal/webapi/handler.go` |
| 用户值优先 | U-3 | AC-02 | `max_len=0` 与 `=30` 两次对比 | 同上 |
| medium 可装 | U-1 | AC-03 | registry + 真装 | `internal/registry/data/registry.json` |
| 双语 | — | AC-04 | `i18n-check.sh` | `frontend/lib/i18n.ts` |

## 2. 状态

| 项 | 状态 | 证据 |
|---|---|---|
| 实测基线（tiny, 30min, RTF 0.058, 163 段） | ✅ | `docs/GAP-ANALYSIS.md` §1 |
| AC-01 句级切分 | ✅ | 同一集 30 min 视频实测：修复前 163 段 / median 32 / p90 96 / **max 328**；修复后 **283 段 / median 32 / p90 39 / max 40，超 40 字的行 = 0**。根因是 align 合并无上限，不是 `max_len` |
| AC-02 用户值优先 | ⚠️ 部分 | 参数确实到达引擎（`q30` 的 `params_snapshot` 含 `max_len:"30"`，`q0` 正确地不含）；但“生效”被 `align` 合并抵消 |
| AC-03 medium sha256 | ⬜ | 下载中（103.9 MB / 1462.7 MB） |
| AC-04 双语 | ⬜ | — |

## 3. 实测数据回填（禁止编造）

| 项 | 值 | 来源 |
|---|---|---|
| 视频时长 | 29:57 | ffprobe |
| tiny 引擎耗时 | 104.8 s（RTF 0.058） | engine.log |
| tiny 段数 | 163 | 产物 |
| medium 期望字节 | 1,533,763,059 | registry（此前已交叉验证） |
| medium 实测 sha256 | 待下载完成 | 本机计算 |
| 下载吞吐 | ≈190 KB/s（hf-mirror，实测） | curl |
| 60 s 片段 `-ml 0` | 25 段，长度 4/18/10（min/max/median） | 直调 whisper-cli |
| 60 s 片段 `-ml 30 -sow` | **同 25 段，4/18/10** | 同上 |
| 30 min `max_len=30` | 163 段，行宽 median 32 / p90 96 / **max 328** | `scripts/verify-lines.py` |
| 30 min `max_len=0` | **完全相同** 163 / 32 / 96 / 328 | 同上 |
| **align 上限修复后** | **283 段 / median 32 / p90 39 / max 40，超限行 = 0** | 同一集重跑（全新 bundle） |

> **关键推论**：`-ml` 无法解释 328 字的行——whisper 自己每段仅 ~10 字。长度膨胀发生在
> `align`（设计上写着“句子合并”）。所以调 `max_len` 默认值无效，**必须改 `align` 的合并策略**。

## 4. 缺陷记录

| # | 缺陷 | 根因 | 状态 |
|---|---|---|---|
| 1 | 中文输出字词错误率高 | 模型太小（tiny 为下限） | 本 CHG 修（A1 换 medium） |
| 2 | 长句粘连、无法当字幕 | **不是** `max_len` 默认值，而是 `align` 合并无累计上限 | ✅ 已修：`MaxMergedRunes = 40`，实测 max 328 → 40 |
| 3 | 安装按钮「像没反应」 | 多数模型 sha256 为 `tbd` → 后端异步拒绝，错误只在顶部一行 | 本 CHG 让 medium 可装；行内提示见审计表 P1 |
