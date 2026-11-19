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

| 里程碑 | 状态 | 证据 |
|---|---|---|
| M1 参数规格表 + 校验 | ✅ | 37 specs / 8 groups；`TestSpecsCoverage`、`TestSpecsUnique`、`TestDefaultsMatchSpecs`、`TestValidateConflicts` 全绿 |
| M2 引擎全参数接入 | ✅ | `whisper.go` 改用 `Params.Args()`；真视频 run 恢复（segments 2，内容为音频原话） |
| M3 registry stems | ✅ | `Model.Stems` + 校验强制 separator 声明 ≥2 stem 且含 vocals；`uvr-vr-hp2-vocals` 的 URL/体积经 HTTP HEAD 实测（63,454,827 B） |
| M4 后端矩阵 | ✅ | AMD/Intel → vulkan；DirectML 永不选用并给出原因；`TestSelectBackendAMD/Intel/NoVulkanLoader/NoGPU` 全绿 |
| M5 参数快照 | ✅ | `meta.json.provenance` 实测：`engines` 为 whisper.cpp/1.9.5（不再是 synthetic）、`params_snapshot: {beam_size:1}`、独立 hash；AC-04 由 `TestPlanInvalidatesTranscribeWhenParamsChange` 锁死 |
| M6 UI 参数面板 | ✅ | `/params` 路由 9.37 kB；截图 `docs/images/app-params-dark.png`；`i18n-check` 108 键双向对齐、零硬编码 |
| M7 文档同步 | ✅ | `UI-DESIGN §7` 增运行参数页；`RESOURCE-SCHEDULER §5` 降级链改为 `cuda12 → vulkan → cpu` 并声明 DirectML 不在链上（与 ADR-018 对齐，原文含 directml 属文档撒谎）；`MODEL-REGISTRY §2.1` 增 `stems` 字段语义与陷阱 |

## 6. 本轮修掉的真缺陷（续 5.4 计数）

| # | 缺陷 | 根因 |
|---|---|---|
| 1 | 真转写回归为 `exit 10` | `cli.go` 硬写 `VAD: true`（旧引擎不读该字段，是空操作），接线后成了静默打开的坏开关，而 `Validate()` 从未被调用 |
| 2 | provenance 自称为 synthetic | `buildMeta` 写死引擎名，真实 whisper.cpp 产物在元数据里擞谎 |
| 3 | `-oj` 被写成 `if !OutJSON` | 逻辑取反，默认路径根本不请求 JSON |
| 4 | 先填默认值再合并旧参数 | `InputLanguage=en` 压掉调用方的 `zh` |
| 5 | 零值 `WhisperParams` 读作 FlashAttn=false | 每次运行静默关掉 flash attention |
| 6 | stage 失败却打 `✓` | `emit` 忽略 `ev.OK`，把失败画成成功 |
| 7 | 适配器丢弃引擎 stderr | 只能看到 `exit 10`；已落 `logs/engine.log` 并在错误里附引擎原话 |
| 8 | 参数失效机制整段是死代码 | checkpoint 里记的是 `HashParams(stage)`（只由阶段名派生的常量），`Plan` 又恒传 `nil`——改 `beam_size` 不可能失效 |
| 9 | `prepareAudio` 每次运行无条件重写 `audio/16k.wav` | 该文件的指纹是 (size, mod_time)，重写刷新 mtime 后 `demux` 的指纹恒不匹配：demux 永久重跑、下游连带重跑，还掩盖了参数比较（`Plan` 在第一个失效阶段就返回） |
| 10 | 计划先算、`disabled` 后改 | 第 199 行才把 `disabled[separate]=true`，计划用的是旧映射，于是永远停在 separate、走不到 transcribe 的参数比较 |
| 11 | 空参数哈希造成假失效 | 写路径缺值会回退到 `HashParams(stage)`，读路径拿原始空串，两边永不等——同参数每次都说“参数变了” |

### 6.1 AC-04 的真实运行证据

```
同参重跑   → （无 resume 行，全部阶段复用）
改 beam=8 → resume: transcribe: parameters changed
```

这一对输出是 AC-04 的判据：只验证“改了会失效”不够，必须同时验证“没改不会误失效”，
否则一个永久失效的实现也能让前半句看起来是对的。

## 7. 仍未绿的 AC（不得隐去）

| AC | 状态 | 正交原因 | 交付时如何补 |
|---|---|---|---|
| AC-08（Demucs=4） | ⚠️ 部分 | 已入库 VR 两轨（`uvr-vr-hp2-vocals`，URL/体积经 HEAD 实测）；Demucs 四轨条目的真实下载地址未能验证——`lj1995/VoiceConversionWebUI` 下 MDX 路径均 404，hf-mirror 的 `/api/models` 返回空，**不能编造 URL** | 找到可验证的 Demucs 权重直链后加入；校验规则已就位，加条目即自动生效 |
| AC-09（未就绪选项置灰并给原因） | ❌ 未交付 | 静态导出页面无法在构建期得知运行时硬件/模型状态；需要 GUI 桥（Wails binding）或 shell 侧状态端点，属 CHG-0005 窗口层 | 在 `shell`/`gui` 暴露 `engine-status.json`，前端据此禁用并显示原因 |

> AC-09 目前**没有**以假实现充数：参数页不假装知道可用性，也不静默忽略不可用项。
> 宁可显式标为未交付，也不做一个看不出真假的置灰。
