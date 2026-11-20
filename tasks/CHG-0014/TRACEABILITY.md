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
| AC-01 句级切分 | ⬜ | — |
| AC-02 用户值优先 | ⬜ | — |
| AC-03 medium sha256 | ⬜ | 下载中（1.46 GB） |
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

## 4. 缺陷记录

| # | 缺陷 | 根因 | 状态 |
|---|---|---|---|
| 1 | 中文输出字词错误率高 | 模型太小（tiny 为下限） | 本 CHG 修（A1 换 medium） |
| 2 | 长句粘连、无法当字幕 | `max_len` 默认 0 且 UI 未给基线 | 本 CHG 修（A2） |
| 3 | 安装按钮「像没反应」 | 多数模型 sha256 为 `tbd` → 后端异步拒绝，错误只在顶部一行 | 本 CHG 让 medium 可装；行内提示见审计表 P1 |
