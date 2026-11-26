# CHG-0017 · CHG Log

## 变更摘要

修一个 owner 用真视频一跑就发现的硬伤：**UVR5 分离阶段完全跑在 CPU 上，显卡零负载**。
CHG-0016 只验证了转写阶段的 CUDA，把"CUDA 可用"误当成"整条流水线在 GPU 上"。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-26 | draft | owner 实测反馈（CPU 打满 / CUDA 不动）触发；根因定位 | agent |
| 2026-11-26 | approved | owner 明确要求修好（「你他妈的上来没有用显卡去计算」），按 PRD/SPEC 执行 | owner |
| 2026-11-26 | in_progress | 安装器双变体、helper 设备自证、meta/capabilities/UI 贯通，验证中 | agent |
| 2026-11-26 | verified | 实测全绿：干净安装 accel=cuda12、显存/利用率上升、120 s 与 30 min 计时、全链路四处 cuda12、降级诚实、Checkfix 绿 | agent |
| 2026-11-26 | closed | owner 验收（「能用了，基本60分达到了」），随 6d474bc 推送 | owner |

## 根因（实测）

1. 旧安装器固定装 `audio-separator[cpu]`，`onnxruntime` 只有
   `['AzureExecutionProvider', 'CPUExecutionProvider']`。
2. PyPI 的 Windows `torch 2.15.0+cpu`：`torch.cuda.is_available() == False`，
   于是 audio-separator 的 `setup_torch_device` 直接走 "No hardware acceleration"，
   连 CUDA provider 分支都不会进。
3. helper 结果 JSON 无设备字段，`BuildMeta` 把分离后端写死 `"cpu"`，engine.log 不记 provider
   —— 跑完无法自证。
4. 2026-11-26 20:06 实测 120 s 素材 CPU 分离：GPU 利用率仅桌面背景 3–13%，
   显存恒定 2139–2154 MiB（分离进程零显存），产物 21.1 MB × 2，耗时约 60 s。

## 实现

- 安装器（`separator_install.go`）：按硬件选 CUDA/CPU 变体；CUDA 变体 = `audio-separator`
  + `onnxruntime-gpu==1.21.1` + nvidia 的 cublas/cudnn/cufft/curand/cudart 轮子；
  两支 ONNX Runtime 互斥；装后用小模型真跑一次 CUDA 会话才记 `accel=cuda12`；
  CPU 装机可在模型中心一键升级。
- **版本钉死的原因（实测）**：`onnxruntime-gpu 1.32.0` 的 provider DLL 依赖
  `cublasLt64_13.dll`，报 "Require cuDNN 9.* and CUDA 13.*"；CUDA 13 需要 580+ 驱动，
  本机 566.36 与目标用户群都不满足。1.21.1 是最后一批 CUDA 12 构建。
- helper：导入 onnxruntime 前把 `site-packages/nvidia/*/bin` 加进 PATH；显式设置
  `onnx_execution_provider`（绕开 torch 门控，MDX 推理本身是纯 ONNX）；包装
  `InferenceSession` 记录真实 provider；结果 JSON 带 `backend/providers/reason`。
- 记录：`LastBackend` → `engine.log` 每次写 `separate backend=… providers=…`；
  meta 的 `separate.backend` 来自实际报告，未跑则留空；capabilities 增加
  `separation_backend`，CPU 装机写明 `· CPU only (<reason>)`。
- UI：模型中心显示 CUDA/CPU 标签，CPU 装机出现「升级到 CUDA」。

## 新增发现（验证过程中暴露）

- **干净 venv 缺 `audioread`**：`audio-separator` 的 `uvr_lib_v5` 在 import 阶段就要它，
  但包元数据已不再声明（librosa 1.x 移除）。原地升级的旧 venv 里恰好残留了该包，掩盖了问题；
  用空 workdir 从零安装才暴露。两个变体都已显式加 `audioread`，并有单测。
- `onnxruntime-gpu 1.21.1` 的探针模型必须钉 `ir_version=9`：新版 onnx 默认写 IR 14，
  ORT 1.21 只支持到 10，探针会在建会话前报 "Unsupported model IR version"。

## 验证（2026-11-26 实测，证据 `tmp/evidence/chg0017/`）

| 项 | 结果 |
|---|---|
| 安装（CUDA 变体） | `INSTALLED.json.accel="cuda12"`，探针 session `[CUDAExecutionProvider, CPUExecutionProvider]` |
| 干净安装（空 workdir） | 从零安装同样 `accel=cuda12`，并用该 venv 真跑分离成功 |
| GPU 占用（120 s 素材） | nvidia-smi 显存 2372 → **4670 MiB**，利用率峰值 **97–100 %**；CPU 版显存恒定 2372 MiB、利用率 ≤8 % |
| 计时（120 s 素材） | CPU **76.3 s** → CUDA **25.1 s**（**3.0×**） |
| 计时（30 min 整集） | CPU **18 min 58 s**（历史基线）→ CUDA **3 min 51 s**（231 s，RTF 0.63 → 0.13，**4.9×**） |
| 输出正确性 | GPU/CPU 两条分轨逐样本 corr = 1.000000，RMS 差 −94 dBFS（float32 累加误差量级） |
| 全链路 | `engine.log` / `meta.json` / `/api/capabilities` 三处均为 `cuda12`（见 TRACEABILITY） |
| 降级诚实 | 只有 CPU 的干净 venv 以 `--accel cuda12` 运行：`backend=cpu` + reason，stderr 两条 WARNING，退出码 0 |
| 吞吐影响 | 30 min 一集端到端由 ~24 min 降到 ~9 min（分离 3:51 + large-v3 转写 4:47） |
| Checkfix | `go build/vet/test -race`、`pnpm lint/typecheck/build`、`verify-contract`、`i18n-check`、`contrast`、`redact-scan`、`secrets-scan` 全绿（`tmp/evidence/chg0017/checkfix-*.log`、`redact.log`、`secrets.log`） |
