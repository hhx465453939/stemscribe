# 变更契约（CHG）目录

每个 CHG = 一份**可执行契约**，不是任务清单。
代码改动前必须读到成立状态的 CHG；没有 CHG 就不许动手（见 `AGENTS.md`）。

## 编号与登记

| ID | 标题 | status | 独占范围 | 依赖 |
|---|---|---|---|---|
| CHG-0001 | Repository Bootstrap & Architecture Contract | **approved** | `docs/**`、`tasks/**`、根目录文档、`cmd/**`、`internal/{registry,downloader}`、`build/**`、`.github/**` | — |
| CHG-0002 | Engines & Runtimes | **proposed**（SPEC_READY，待 owner 批准） | `internal/engine/**`、`internal/registry/data/registry.json`（whisper+ffmpeg 条目）、`.github/workflows/runtime.yml`、`scripts/runtime-*`、`cmd/stemscribe/cli.go`（引擎接线）。**不包含** `internal/resource/**`（仅接线不改逻辑，算法改动走 CHG-0006）；人声分离已移出到 CHG-0003 | 0001 |
| CHG-0003 | Pipeline & Store | planned | `internal/pipeline/**`、`internal/store/**` | 0002 |
| CHG-0004 | LLM Layer | planned | `internal/provider/**`、`internal/vault/**`、`prompts/**` | 0001（可与 0002/0003 并行） |
| CHG-0005 | Frontend | planned | `frontend/**` | 0003（需 IPC 契约冻结） |
| CHG-0006 | Scheduling & Agent Integration | planned | `internal/mcpserver/**`、`internal/resource/**`（并发部分） | 0003 |
| CHG-0007 | Theming & Localization（双主题 + 中英双语） | **approved** | `frontend/app/**`、`frontend/lib/{i18n,api}.ts`、`scripts/{i18n-check.sh,contrast.py}`、`docs/images/app-*`、`README*.md`、`docs/UI-DESIGN.md` §3/§14 | — |
| CHG-0008 | Runtime Options & Backend Compatibility（参数控制面 + 后端矩阵） | **approved** | `internal/engine/**`、`internal/registry/**`、`internal/resource/**`、`internal/pipeline/**` 与 `internal/store/**`（仅参数快照，跨界已登记）、`cmd/stemscribe/**`、`frontend/app/**`、`scripts/param-defaults-check.ps1`、`docs/{UI-DESIGN,RESOURCE-SCHEDULER,MODEL-REGISTRY}.md` | 0002 |
| CHG-0010 | Run Control & Live Progress（单击运行 + 实时进度） | **approved** | `internal/runctl/**`、`internal/webapi/**`、`cmd/stemscribe-gui/**`、`cmd/stemscribe/{cli,shell}.go`、`internal/webui/**`（仅加 ServeHandler）、`frontend/app/**`、`frontend/lib/{i18n,api}.ts`、`scripts/build.ps1`、`docs/{ARCHITECTURE,PIPELINE,UI-DESIGN}.md`、`tasks/README.md` | 0008 |
| CHG-0011 | Provisioning（从界面安装模型与运行时） | **approved** | `internal/runctl/install.go`、`cmd/stemscribe/install.go`、`internal/webapi/handler.go`（仅新增 install 路由）、`frontend/app/models/page.tsx`、`frontend/lib/{api,i18n}.ts`、`docs/{MODEL-REGISTRY,UI-DESIGN}.md`、`tasks/README.md` | 0002、0010 |
| CHG-0012 | Multi-file Intake & Run Queue（多文件录入 + 串行队列） | **approved**（M3 拖入已由 CHG-0015 交付） | `internal/runctl/queue.go`、`internal/webapi/handler.go`（queue 路由）、`cmd/stemscribe-gui/main_gui.go`、`frontend/app/components/NewTask.tsx`、`frontend/lib/{api,i18n}.ts`、`docs/{PIPELINE,UI-DESIGN}.md`、`tasks/README.md` | 0010 |
| CHG-0013 | UX Essentials（审计 P0：参数生效/输出目录/浏览/打开产物） | **approved** | `internal/config/config.go`、`internal/runctl/run.go`、`internal/webapi/handler.go`、`cmd/stemscribe-gui/main_gui.go`、`frontend/app/components/NewTask.tsx`、`frontend/lib/{api,i18n}.ts`、`docs/{UX-AUDIT,UI-DESIGN}.md`、`tasks/README.md` | 0010、0011、0012 |
| CHG-0014 | Transcript Quality Baseline（阶段 A1+A2：可用模型 + 句级切分） | **closed**（2026-11-26，AC 全绿） | `internal/registry/data/registry.json`、`internal/webapi/handler.go`、`frontend/app/components/NewTask.tsx`、`frontend/lib/{api,i18n}.ts`、`internal/pipeline/pipeline.go` + `align_merge_test.go`（**跨界**，归 CHG-0008）、`scripts/verify-lines.py`、`docs/{GAP-ANALYSIS,PIPELINE}.md`、`tasks/README.md` | 0002、0010、0012、0013 |
| CHG-0015 | Core Pipeline（强制 UVR5 分离 + 最大模型 + 拖入即跑 + txt） | **closed**（2026-11-26，AC 有证据；AC-05 真实鼠标拖拽留人工复核） | `internal/engine/{separator.go,separator_helper.py,separator_meta/**,locate.go,ffmpeg.go}`、`internal/runctl/{run.go,helpers.go,catalog.go,install.go,separator_install.go,queue.go,service.go}`、`internal/config/config.go`、`internal/registry/{registry.go,data/registry.json}`、`internal/pipeline/{pipeline.go,transcript.go}`、`internal/store/bundle.go`、`internal/webapi/handler.go`、`cmd/stemscribe/{cli.go,install.go}`、`cmd/stemscribe-gui/main_gui.go`、`frontend/app/components/NewTask.tsx`、`frontend/lib/{api,i18n}.ts`、`docs/{PIPELINE,MODEL-REGISTRY,RESOURCE-SCHEDULER,ARCHITECTURE,GAP-ANALYSIS}.md`、`docs/schema/model-registry.schema.json`、`tasks/README.md` | 0002、0010、0011、0012、0013 |
| CHG-0016 | CUDA Runtime & Desktop Essentials（CUDA 真用上 + 拖入区/开始/停止/输出目录/打开目录） | **closed**（2026-11-26，AC 全绿、Checkfix 绿，已推送 86a99ba） | `internal/registry/**`、`internal/engine/**`、`internal/runctl/**`、`internal/webapi/**`、`internal/resource/**`（仅 GPUName）、`internal/store/**`（仅 EngineInfo 填值）、`cmd/stemscribe-gui/**`、`cmd/stemscribe/**`、`frontend/**`、`.github/workflows/runtime.yml`、`.github/scripts/stage-runtime.ps1`、`scripts/{build.ps1,build-runtime-cuda.ps1}`、`docs/{UI-DESIGN,PIPELINE,MODEL-REGISTRY,ARCHITECTURE,GAP-ANALYSIS}.md`、`tasks/CHG-0015/CHG.md`（仅 ADR-040 注记）、`tasks/README.md` | 0010、0011、0012、0013、0015 |

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
