# TRACEABILITY · CHG-0002

> 规则：无对应关系必须写 `TBD` 并说明由哪个里程碑回填；「实测数据」只登记真跑出来的数值。
> 本 CHG 尚未开工，因此**除契约文档外全部为 ⬜**——这不是遗漏，是真实状态。

---

## 1. 需求 → 判据 → 验证 → 归属

| 需求（PRD） | 目标 | AC | 验证方式 | 交付文件 | 里程碑 |
|---|---|---|---|---|---|
| F-01 自建运行时 | G-2 | AC-01、AC-02 | `runtime.yml` 构建日志 + `runtime-verify.ps1` | `.github/workflows/runtime.yml`、`scripts/runtime-build.ps1` | M1 |
| F-02 运行时校验 | G-2 | AC-03、AC-04 | 资产 hash vs registry 比对 | `internal/registry/data/registry.json` | M1/M2 |
| F-03 模型下载 | G-3 | AC-04 | 下载 + sha256 + `.part` → rename 观察 | `internal/downloader`（复用，不改） | M2 |
| F-04 ffmpeg 运行时 | G-5 | AC-05 | `ffprobe` 对比源/产物时长 | `internal/engine/ffmpeg.go` | M3 |
| F-05 引擎适配器 | G-1 | AC-07 | 编译期接口断言 | `internal/engine/whisper.go` | M4 |
| F-06 后端选择 | G-4 | AC-09 | 去掉 Vulkan 运行时后重跑 | `cmd/stemscribe/cli.go`、`internal/resource`（接线） | M5 |
| F-07 降级可见 | G-4 | AC-09 | 事件日志含 `reason` | `internal/engine/whisper.go` | M5 |
| F-08 真实 e2e | G-1 | AC-06、AC-12 | `smoke-test.ps1 -Real` | `scripts/smoke-test.ps1` | M6 |
| F-09 合成引擎退位 | G-1 | AC-12 | 默认路径不含 synthetic 字样 | `cmd/stemscribe/cli.go` | M5 |
| F-10 未发布不可装 | G-3 | AC-11 | registry 断言 + UI 手工 | `internal/registry`、`frontend`（后置联调） | M2 |
| 取消/孤儿进程 | G-6 | AC-08 | 取消后 `tasklist` 断言 | `internal/engine/whisper.go` | M4 |
| 缺失项一次列全 | G-1 | AC-10 | 空目录下 `run` | `cmd/stemscribe/cli.go` | M5 |

## 2. 交付物 → 判据 → 状态

| # | 交付物 | 判据 | 状态 |
|---|---|---|---|
| 1 | `tasks/CHG-0002/PRD.md` | 三件套 | ✅ 已交付 |
| 2 | `tasks/CHG-0002/SPEC.md` | 三件套 | ✅ 已交付 |
| 3 | `tasks/CHG-0002/TRACEABILITY.md`（本文件） | 三件套 | ✅ 已交付 |
| 4 | `tasks/CHG-0002/CHG.md` | 流程记录 | ✅ 已交付 |
| 5 | `.github/workflows/runtime.yml` | AC-01/02 | ⬜ M1 |
| 6 | `scripts/runtime-build.ps1` / `runtime-verify.ps1` | AC-02/03 | ⬜ M1 |
| 7 | registry 实测回填（whisper + ffmpeg） | AC-04 | ⬜ M2 |
| 8 | `internal/engine/ffmpeg.go` | AC-05 | ⬜ M3 |
| 9 | `internal/engine/whisper.go` + `progress.go` | AC-06/07/08 | ⬜ M4 |
| 10 | `cmd/stemscribe/cli.go` 引擎接线 + 缺失项一次列全 | AC-09/10/12 | ⬜ M5 |
| 11 | `scripts/smoke-test.ps1 -Real` 分支 | AC-12 | ⬜ M6 |
| 12 | `THIRD-PARTY-NOTICES.md`（whisper.cpp MIT / ffmpeg LGPL） | 合规 | ⬜ M6 |
| 13 | `docs/MODEL-REGISTRY.md` 运行时章节 + `docs/UPSTREAM-RESEARCH.md` 实测对照 | 文档同步 | ⬜ M6 |

## 3. 实测数据登记（禁止编造）

### 3.1 已确认的事实（有来源）

| 事实 | 来源 | 日期 |
|---|---|---|
| 模型官方下载源为 `https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-*.bin` | HF 仓库页（`ggml-tiny.bin` 直链已核实） | 2026-11-17 |
| whisper.cpp 的 Vulkan 后端产物运行时**不需厂商 SDK**，NVIDIA/AMD/Intel 通用 | 上游构建说明 + 社区 Vulkan 构建实践 | 2026-11-17 |
| CUDA 后端需要 CUDA Toolkit 构建，且运行时依赖 CUDA DLL | 上游构建说明 | 2026-11-17 |
| `whisper-cli` 只接受 **16 kHz 16-bit WAV** | 上游 README | 2026-11-17 |
| 二进制名为 `whisper-cli.exe` | 上游 README / 构建产物 | 2026-11-17 |
| 无 GPU 时 CPU 路径可用（本机实测：`probe` 返回 `cpu` 或 `vulkan` 的理由串） | 本机 `stemscribe probe` | 2026-11-17 |

### 3.2 待实测回填（**不得猜**）

| 项 | 谁填 | 里程碑 |
|---|---|---|
| cpu / vulkan 两个 zip 的**实测字节数** | `runtime-verify.ps1` | M1 |
| cpu / vulkan 两个 zip 的**实测 sha256** | 同上 | M1 |
| Vulkan SDK 在 CI 的安装耗时 | Actions 日志 | M1 |
| whisper.cpp 构建耗时（两变体） | Actions 日志 | M1 |
| 量化档位文件是否存在（Q-1） | `HEAD` 探测脚本 | M1 |
| Vulkan 构建的 DLL 依赖清单（确认无 CUDA） | `dumpbin` | M1 |
| 各模型档位的实测字节数 + sha256 | 下载后实测 | M2 |
| 30 分钟音频：`medium` 的 RTF 与峰值显存 | `probe` + NVML 采样 | M4 |
| `tiny` / `small` / `medium` 在 CPU 上的 RTF | 同上 | M4 |
| Vulkan 相对 CPU 的加速比 | 同上 | M4 |
| 适配器层额外开销（JSON 落盘+读取） | 计时 | M4 |
| 真实 30s 语音样本的生成方式与 hash（Q-3） | 生成脚本 | M6 |

## 4. 缺口登记（诚实）

| 缺口 | 影响 | 计划 |
|---|---|---|
| **人声分离不在本 CHG** | v0.1 的「人声分离」勾选框需置灰并说明"运行时未就绪" | CHG-0003 |
| CUDA 后端缺失 | NVIDIA 用户拿不到 CUDA 级性能 | 后续可选资产（ADR-013 同流程） |
| 无 DirectML | 老 Intel 核显可能只有 CPU 可用 | 不做（上游无成熟后端） |
| 模型量化档位命名未逐一核实 | registry 可能引用不存在的文件 | M1 阻塞项 Q-1 |
| Vulkan SDK 若导致 CI 超时 | 只能自建 CPU 变体 | Q-2 的降级路径已写明 |
| 真语音样本来源 | 不能把版权音频塞进仓库 | Q-3：TTS/合成 + 仅存 hash |
| AC-11（UI 不可安装态）依赖前端 | 本 CHG 只到后端契约与数据 | 与 CHG-0005 联调时收口 |

## 5. 里程碑状态

| 里程碑 | 状态 | 证据 |
|---|---|---|
| M1 运行时自建流水线 | ⬜ 未开始 | 需 SPEC 批准 |
| M2 registry 实测回填 | ⬜ 未开始 | 依赖 M1 产物 |
| M3 ffmpeg + demux | ⬜ 未开始 | — |
| M4 whisper 适配器 | ⬜ 未开始 | 依赖 M2 的真实 sha256 |
| M5 CLI 接线与降级 | ⬜ 未开始 | 依赖 M4 |
| M6 真 e2e + 文档 | ⬜ 未开始 | 依赖 M5 |

## 6. 与既有 CHG 的关系

| CHG | 关系 |
|---|---|
| CHG-0001 | 提供 `internal/downloader`（续传/校验/原子落盘）与 `pipeline.Engine` 端口 → 本 CHG 直接复用，不改 |
| CHG-0003 | 接收本 CHG 移出的「人声分离」；本 CHG 的 `Separate` 返回 `ErrNotImplemented` |
| CHG-0006 | 本 CHG 只**接线** `internal/resource.SelectBackend`，不改预算算法；算法改动静走 CHG-0006 |
| CHG-0007 | 无交集（纯前端主题/i18n） |
