# CHG-0017 · TRACEABILITY

证据目录：`tmp/evidence/chg0017/`（不入库，`.gitignore` 覆盖 `tmp/`）。
机器：RTX 3060 12 GB / 驱动 566.36 / Windows 10 x64；素材：120 s 44.1 kHz 立体声
（`dist/workdir/libraries/local-2953b5fe884e1134/audio/full.wav`，21,168,078 B）。

## 需求 → 实现 → 证据

| AC | 判据 | 实现 | 证据 |
|---|---|---|---|
| AC-01 | 本机目标 `cuda12`，装完 `INSTALLED.json.accel=="cuda12"` 且探针 session 含 CUDA | `SeparatorDesiredAccel`、`probeSeparatorSession`、`writeSeparatorRecord` | `install-cuda.log`（`accel=cuda12 providers=[CUDAExecutionProvider CPUExecutionProvider]`）；`dist/workdir/runtimes/separator/INSTALLED.json` |
| AC-02 | 分离期间显存/利用率上升；CPU 版不变 | helper 强制 CUDA provider + `record_sessions` 自证 | `cuda.gpu`（基线 2372 MiB → 峰值 4670 MiB，利用率峰值 97–100%）对照 `cpu2.gpu`（显存 2372 MiB 不变、利用率 ≤8%）；30 min 整集 `cuda30.gpu` 峰值 4617 MiB |
| AC-03 | JSON / engine.log / meta / capabilities 四处为 `cuda12` | `Separator.LastBackend` → `engine.log`；`separatorBackend(eng)` → `BuildMeta`；`SeparationBackend` → capabilities | `cuda.out`（`"backend":"cuda12"`）；全链路运行日志与 meta 见下节 |
| AC-04 | CUDA 明显快于 CPU，记录秒数与倍率 | 同素材、同模型、同 venv，仅 `--accel` 不同 | 120 s 素材：CUDA **25,116 ms** vs CPU **76,316 ms**（**3.04×**）；30 min 整集：CUDA **231,076 ms** vs CPU 基线 **18 min 58 s**（**4.9×**） |
| AC-05 | 请求 CUDA 不可用时落 CPU 且带 reason，不静默 | `configure_onnx` reason + helper stderr WARNING + meta 记实际 provider | 干净 CPU venv（`onnxruntime` 只有 Azure/CPU provider）以 `--accel cuda12` 运行：`fallback.out` 里 `"backend":"cpu","requested":"cuda12"` + 完整 reason，stderr 两条 WARNING，退出码 0 |
| AC-06 | CPU 装机显示「升级到 CUDA」，点击后 accel 变化 | `CatalogItem.Upgrade`（`separatorNeedsUpgrade`）+ `models/page.tsx` 按钮 + i18n | 单测 `TestSeparatorNeedsUpgrade`；本机旧记录 `accel=""` 经模型中心同一安装入口（CLI `runtime install`）原地升级为 `accel=cuda12`（`install-cuda.log`）；前端 `lint/typecheck/build` 通过 |
| AC-07 | Go 单测覆盖互斥/真实 backend/未跑留空 | `separator_install_test.go`、`run_test.go` | `go test ./...` 输出 |
| AC-08 | 全量 Checkfix 绿 | — | 见 CHG.md 验证节 |

## 全链路（2026-11-26 实测）

命令：`stemscribe run --input tmp/queue-test/a.mp4 --workdir dist/workdir --model whisper-large-v3 --language zh`
（真实引擎，20.1 s 素材；强制分离；润色按设置为关闭）。

| 位置 | 证据 |
|---|---|
| helper JSON | `cuda.out`：`"backend":"cuda12","providers":[...,"CUDAExecutionProvider"],"requested":"cuda12","reason":""` |
| `engine.log` | `separate backend=cuda12 providers=[CPUExecutionProvider CUDAExecutionProvider] requested="cuda12"` |
| bundle `meta.json` | `provenance.engines.separate = {name:"uvr5", version:"0.48.0", backend:"cuda12", model_id:"uvr-mdx-kim-vocal-2"}`（同时 `transcribe.backend="cuda12"`） |
| `/api/capabilities` | `separation_backend="cuda12"`，`separation_reason="UVR5 via audio-separator 0.48.0 · uvr-mdx-kim-vocal-2 · CUDA"`（`capabilities.json`） |

## 正确性旁证

同一 120 s 素材，CUDA 与 CPU 两侧输出的两条分轨逐样本相关：

| 分轨 | corr | RMS 差 |
|---|---|---|
| vocals | 1.000000 | −94.4 dBFS |
| instrumental | 1.000000 | −94.3 dBFS |

差值在 float32 累加误差量级，说明 GPU 跑的是同一个模型、同一段推理，而不是"动了但结果不对"。

## 新鲜安装验证（不使用缓存环境）

旧 workdir 的 venv 是从 CPU 版原地升级的，会掩盖"缺包"问题。因此另建空 workdir
`tmp/fresh-workdir`，用 `stemscribe runtime install separator-python` 从零安装（`install-fresh-cuda.log`）：

- 安装探针：`separation runtime ready: accel=cuda12 providers=[CUDAExecutionProvider CPUExecutionProvider]`
- 用该 venv 跑 20 s 素材：`fresh.out` → `"backend":"cuda12"`，退出码 0。

这一步暴露并修掉了一个真实缺包：`audio-separator` 的 `uvr_lib_v5` 在 import 时依赖
`audioread`，但其包元数据已不再声明（librosa 1.x 移除了该依赖），干净 venv 会
`ModuleNotFoundError`。两个变体现在都显式安装 `audioread`，并有单测守着。
