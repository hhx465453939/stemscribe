# TRACEABILITY · CHG-0008

> 无对应关系必须写 `TBD` 并说明由哪个里程碑回填；实测数据只登记真跑出来的值。

---

## 1. 需求 → 判据 → 验证 → 归属

| 需求（PRD） | 目标 | AC | 验证方式 | 交付文件 | 里程碑 |
|---|---|---|---|---|---|
| F-01 参数面 | G-1 | AC-01、AC-02 | `go test ./internal/engine -run TestSpecs` + `scripts/param-defaults-check.ps1` | `internal/engine/params.go` | M1 |
| F-02 语言双维度 | G-2 | AC-03 | 适配器单测断言 args 含 `-l/-dl/-tr` | `internal/engine/params.go`、`whisper.go` | M1/M2 |
| F-03 模型语言校验 | G-2 | AC-05 | 表驱动冲突单测 | `internal/engine/params.go` `Validate` | M1 |
| F-04 stem 目标 | G-3 | AC-08、AC-09 | registry 断言 + UI 手工 | `registry.json`、`frontend` | M3/M6 |
| F-05 registry stems | G-3 | AC-08 | `go test ./internal/registry` | `internal/registry/registry.go`、`data/registry.json` | M3 |
| F-06 后端矩阵 | G-4 | AC-06 | `go test ./internal/resource` | `internal/resource/scheduler.go` | M4 |
| F-07 降级可见/可锁 | G-4 | AC-07 | 单测 + 真跑（删运行时目录） | `internal/resource`、`cmd/stemscribe/wire.go` | M4 |
| F-08 参数持久化 | G-5 | AC-04 | 断言 `meta.json.provenance.params_snapshot` | `internal/store`、`internal/pipeline`、`cmd` | M5 |
| F-09 冲突检测 | G-2 | AC-05 | 四类冲突全拦 | `internal/engine/params.go` | M1 |
| F-10 CLI 等价物 | G-1 | AC-03、AC-10 | `stemscribe run --help` 覆盖 | `cmd/stemscribe/cli.go` | M5 |
| G-6 双主题双语 | G-6 | AC-10 | `i18n-check.sh` + 截图 | `frontend/**`、`lib/i18n.ts` | M6 |

## 2. 交付物 → 判据 → 状态

| # | 交付物 | 判据 | 状态 |
|---|---|---|---|
| 1 | `tasks/CHG-0008/PRD.md` | 三件套 | ✅ 已交付 |
| 2 | `tasks/CHG-0008/SPEC.md` | 三件套 | ✅ 已交付 |
| 3 | `tasks/CHG-0008/TRACEABILITY.md`（本文件） | 三件套 | ✅ 已交付 |
| 4 | `tasks/CHG-0008/CHG.md` | 流程记录 | ✅ 已交付 |
| 5 | `internal/engine/params.go` + 测试 | AC-01/02/03/05 | ⬜ M1 |
| 6 | `internal/engine/whisper.go` 全参数接入 | AC-03 | ⬜ M2 |
| 7 | registry `stems` 字段 + 数据 | AC-08 | ⬜ M3 |
| 8 | `internal/resource` 后端矩阵 | AC-06/07 | ⬜ M4 |
| 9 | 参数快照与阶段失效 | AC-04 | ⬜ M5 |
| 10 | 前端参数面板 | AC-09/10 | ⬜ M6 |
| 11 | `scripts/param-defaults-check.ps1` | AC-02 | ⬜ M1 |
| 12 | 文档同步（UI-DESIGN / RESOURCE-SCHEDULER / MODEL-REGISTRY） | AC-10 | ⬜ M7 |

## 3. 实测数据登记（禁止编造）

### 3.1 已确认事实（来源：本机 `whisper-cli.exe --help`，1.9.5）

| 事实 | 值 |
|---|---|
| 默认识别语言 | `-l en` |
| 默认线程 / 处理器 | `-t 4` / `-p 1` |
| 默认束搜索 / 候选 | `-bs 5` / `-bo 5` |
| 默认温度 / 增温 | `-tp 0.00` / `-tpi 0.20` |
| 默认上下文 / 最大段长 | `-mc -1` / `-ml 0` |
| 默认阈值 | `-nth 0.60` / `-et 2.40` / `-lpt -1.00` / `-wt 0.01` |
| flash attention | `-fa true`（默认开） |
| 翻译只到英文 | `-tr --translate  translate from source language to english` |
| 支持输入格式 | flac / mp3 / ogg / wav |
| VAD 参数 | `--vad -vm -vt 0.50 -vspd 250 -vsd 100 -vmsd FLT_MAX -vp 30 -vo 0.10` |
| 输出格式 | `-otxt -ovtt -osrt -olrc -owts -ocsv -oj -ojf` |

### 3.2 分离模型族（来源：本机 UVR 安装目录枚举）

| 族 | 证据文件 | stems |
|---|---|---|
| MDX-Net（ONNX） | `UVR-MDX-NET-Inst_HQ_3.onnx`（名字含 **Inst**） | vocals / instrumental |
| VR（PyTorch） | `1_HP-UVR.pth`、`UVR-DeNoise-Lite.pth` | vocals / instrumental |
| Demucs | `v3_v4_repo/` | drums / bass / other / vocals |
| ONNX Runtime | `onnxruntime/capi`（UVR 自带 C API） | 佐证 MDX 走 ONNX 可行 |

### 3.3 待实测回填

| 项 | 谁填 | 里程碑 |
|---|---|---|
| Vulkan 相对 CPU / CUDA 的实际加速比 | 真跑计时 | M4 |
| AMD 机器上 Vulkan 是否初始化成功 | 需 AMD 硬件 | TBD（本机只有 3060，无法验证，如实登记） |
| `-bs`/`-bo` 对耗时与质量的影响曲线 | 真跑 | M2 |
| 参数快照对 `params_hash` 的实际影响面 | 单测证明 | M5 |

## 4. 缺口登记（诚实）

| 缺口 | 影响 | 计划 |
|---|---|---|
| **分离推理引擎不在本 CHG**（ADR-021） | "instrumental 分离"选项可配但**不能真跑**；引擎未就绪时必须禁用并说明 | CHG-0009 |
| DirectML 不支持 | 老 Intel 核显可能只能 CPU | 不做（上游无后端），矩阵如实标注 |
| 非英文目标翻译 | `-tr` 只能到英文 | 两段式方案记入 CHG-0009 |
| AMD 硬件未实测 | Vulkan 路径在 AMD 上的真experimental 可用性只有代码保证 | 需要 AMD 机器验证，登记为 TBD |

## 5. 里程碑状态

| 里程碑 | 状态 |
|---|---|
| M1 参数规格表 + 校验 | ⬜ |
| M2 引擎全参数接入 | ⬜ |
| M3 registry stems | ⬜ |
| M4 后端矩阵 | ⬜ |
| M5 参数快照 | ⬜ |
| M6 UI 参数面板 | ⬜ |
| M7 文档同步 | ⬜ |
