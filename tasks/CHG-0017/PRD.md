# CHG-0017 · PRD — 分离阶段必须真正使用 GPU

## 背景（owner 2026-11-26 实测反馈）

拖入真实视频开跑后，**UVR5 分离阶段把 CPU 打满，显卡 CUDA 占用为零**；此前
CHG-0016 只验证了转写阶段的 CUDA，没有验证分离阶段。owner 的原话：
「分离音轨一直在把 CPU 打满，但是我的显卡 CUDA 那块就没有运行」。

## 根因（已实证，不是猜测）

1. 运行时安装的是 `audio-separator[cpu]`（`internal/runctl/separator_install.go`），
   装出来的 onnxruntime **只有 `CPUExecutionProvider`**；实测
   `ort.get_available_providers()` 返回 `['AzureExecutionProvider', 'CPUExecutionProvider']`。
2. 装的是 PyPI 的 `torch 2.15.0+cpu`，`torch.cuda.is_available() == False`。
   audio-separator 只有在 `torch.cuda.is_available()` 为真时才启用 CUDA provider
   （`separator.py: setup_torch_device → configure_cuda`），因此连 CUDA 分支都进不去。
3. 分离产物 JSON 不携带任何设备信息，`BuildMeta` 把分离后端**硬编码为 "cpu"**，
   engine.log 不记录 provider —— 运行结束后无法回答「这次到底用的什么设备」。

## 目标

在装有可用 NVIDIA GPU 的机器上，**分离阶段默认使用 CUDA**；不可用时必须显性降级并
记录原因，不允许静默跑 CPU。任何一次运行都能从日志与 meta 回答用了哪个设备。

## 需求

- R1 分离运行时按硬件选择 CUDA 或 CPU；CUDA 版自带所需 CUDA 库（不要求用户预装 toolkit）。
- R2 每次分离自证设备：helper 解析真实 ONNX session 的 provider，写入结果 JSON。
- R3 设备事实贯穿全链：engine.log、meta.provenance.engines.separate.backend、
  `/api/capabilities.separation_backend` 三处一致，不一致即缺陷。
- R4 已安装的 CPU 运行时可原地升级为 CUDA：模型中心对「装的是 CPU、机器能跑 CUDA」
  的行提供「升级到 CUDA」，不得只显示「已校验」。
- R5 用 nvidia-smi 采样 + 同素材计时给出前后对比证据，不用「应该更快」代替测量。

## 非目标

- 不切换分离模型（当前仍是 MDX-Net `Kim_Vocal_2.onnx`；Demucs 从未接入过，
  registry 和代码里都没有 Demucs 权重。要加是另一个 CHG）。
- 不改分段/半精度等音质参数。
- 不为无 GPU 机器安装 CUDA 依赖（CPU 版照旧，只是明确标注 CPU）。

## 风险与代价

- CUDA 版分离运行时下载体积显著增大（onnxruntime-gpu + cudnn/cublas/cufft 轮子），
  首次安装时间变长；必须在模型中心显示实测体积。
- GPU 分离与 whisper 转写共享显存；分离完成后 ONNX session 随进程退出释放。
