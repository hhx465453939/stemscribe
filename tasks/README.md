# 变更契约（CHG）目录

每个 CHG = 一份**可执行契约**，不是任务清单。
代码改动前必须读到成立状态的 CHG；没有 CHG 就不许动手（见 `AGENTS.md`）。

## 编号与登记

| ID | 标题 | status | 独占范围 | 依赖 |
|---|---|---|---|---|
| CHG-0001 | Repository Bootstrap & Architecture Contract | **approved** | `docs/**`、`tasks/**`、根目录文档、`cmd/**`、`internal/{registry,downloader}`、`build/**`、`.github/**` | — |
| CHG-0002 | Engines & Runtimes | planned | `internal/engine/**`、`internal/resource/**`、`runtimes/**` 清单 | 0001 |
| CHG-0003 | Pipeline & Store | planned | `internal/pipeline/**`、`internal/store/**` | 0002 |
| CHG-0004 | LLM Layer | planned | `internal/provider/**`、`internal/vault/**`、`prompts/**` | 0001（可与 0002/0003 并行） |
| CHG-0005 | Frontend | planned | `frontend/**` | 0003（需 IPC 契约冻结） |
| CHG-0006 | Scheduling & Agent Integration | planned | `internal/mcpserver/**`、`internal/resource/**`（并发部分） | 0003 |
| CHG-0007 | Theming & Localization（双主题 + 中英双语） | **approved** | `frontend/app/**`、`frontend/lib/{i18n,api}.ts`、`scripts/{i18n-check.sh,contrast.py}`、`docs/images/app-*`、`README*.md`、`docs/UI-DESIGN.md` §3/§14 | — |

> `planned` 的 CHG 目录尚未创建。开工时复制 `CHG-0001/` 结构并改写内容，然后把 status 从 `draft` 提到 `approved`。

## 四件套（缺一不算立项）

| 文件 | 作用 | 硬要求 |
|---|---|---|
| `SPEC.md` | 目标、架构决策摘要、交付物、**验收判据**、独占文件、风险回滚 | AC 必须能红；每条给验证方式 |
| `PRD.md` | 用户、场景、功能需求（P0/P1/P2）、交互流程、非功能指标、边界 | 用户故事要具体到可演示 |
| `TRACEABILITY.md` | 需求 ↔ AC ↔ 验证 ↔ 归属映射；实测数据登记；缺口登记 | 无对应关系必须写 TBD，禁止留空 |
| `CHG.md` | 状态流转、决策增补、下一步顺序 | 每次状态变更追加一行，不改历史 |

## 状态机

```
draft ──► proposed ──► approved ──► in_progress ──► closed
                            │                          ▲
                            └────────► rejected         │
                                              （AC 全绿才可关闭）
```

- `draft`：还没想清楚，**不许写代码**
- `approved`：契约成立，可以动手
- `closed`：`SPEC.md` §6 全部 AC 验证通过，`CHG.md` 记录证据

## 写 CHG 的三条底线

1. **AC 必须能红**：说不出「什么情况算失败」= 没有判据 = 必然假绿。
2. **不编造数据**：没实测的数值写 `TBD` 并在 `TRACEABILITY.md` §3 登记待回填项。
3. **独占清单要具体到目录**：否则多 agent 并行必然互相踩踏。

## 与全局规则的关系

本目录是项目级落地，上游约束来自：
- `C:\Users\DamnCheater\AGENTS.md`（全局铁律）
- 本项目 `AGENTS.md`（项目特化）
- `docs/DECISIONS.md`（ADR 是契约的一部分，推翻需新增 ADR）
