# CHG-0017 · SPEC — GPU 分离（v1）

## 1. 安装层

`InstallSeparatorRuntime`（`internal/runctl/separator_install.go`）：

| accel | pip 包 |
|---|---|
| `cuda12` | `audio-separator` + `onnxruntime-gpu` + `nvidia-cublas-cu12` + `nvidia-cudnn-cu12` + `nvidia-cufft-cu12` + `nvidia-curand-cu12` + `nvidia-cuda-runtime-cu12` |
| `cpu` | `audio-separator` + `onnxruntime` |

- 选择：`SeparatorDesiredAccel()` = `cuda12` 当 `nvidia-smi` 可用且驱动 ≥ minDriverMajor，否则 `cpu`。
- 两种 ONNX Runtime 发行包互斥：安装前先卸载另一支，避免 provider 取决于安装顺序。
- 安装后**真跑一次会话探针**（Identity 小模型 + `CUDAExecutionProvider`），
  只有 session 实际激活 CUDA 才把 `accel=cuda12` 写进 `INSTALLED.json`；
  否则写 `accel=cpu` + `accel_reason`，安装仍可用但状态诚实。
- 已安装但 `accel` 与目标不一致 → 原地重装（同一 venv，只换包）。

## 2. 执行层

`internal/engine/separator_helper.py`：

- `--accel auto|cuda12|cpu`。
- 导入 onnxruntime 前把 `site-packages/nvidia/*/bin` 前插到 `PATH`（Windows DLL 解析）。
- `cuda12`：确认 `CUDAExecutionProvider` 在编译期 provider 列表里；在 `Separator`
  构造后显式设置 `onnx_execution_provider`（Windows torch 为 CPU 版，上游门控不可用）。
- 包装 `onnxruntime.InferenceSession` 记录每个会话实际激活的 provider。
- 结果 JSON 增加 `backend/provider/requested/available/reason`；请求 CUDA 却落到 CPU
  时向 stderr 打 `WARNING`。

## 3. 记录层

- `separatorResult` 解析 `backend`；`Separator.LastBackend` 保存本次实际设备。
- `engine.log` 每次分离写 `separate backend=... providers=... requested=...`；
  降级时追加 `WARN`。
- `BuildMeta` 不再硬编码 `"cpu"`，由 `separatorBackend(eng)` 提供；
  未跑分离则留空（不猜）。
- `/api/capabilities` 增加 `separation_backend`；CPU 装机时 reason 追加
  `· CPU only (<reason>)`。

## 4. UI

- 模型中心：运行时行显示 `CUDA` / `CPU` 标签；已装 CPU 而机器可跑 CUDA 时出现
  「升级到 CUDA」按钮（复用既有 install 接口，安装器负责升级）。
- 文案双语（i18n 键 `models.upgradeCuda`）。

## 5. 验收

| AC | 判据 |
|---|---|
| AC-01 | 当前机器 `SeparatorDesiredAccel()=="cuda12"`，安装后 `INSTALLED.json.accel=="cuda12"` 且 provider 探针 session 含 CUDA |
| AC-02 | 同 120 s 素材分离：nvidia-smi 采样显示分离期间显存上升 / GPU 利用率上升，CPU 版对照显存不变 |
| AC-03 | 分离结果 JSON、engine.log、meta `separate.backend`、capabilities 四处均为 `cuda12` |
| AC-04 | 计时：CUDA 分离耗时显著低于 CPU 基线（记录实测秒数与倍率） |
| AC-05 | 降级诚实：请求 `cuda12` 而 provider 不可用时，JSON/日志标 `cpu` + reason，不静默 |
| AC-06 | 模型中心对 CPU 装机显示「升级到 CUDA」，点击后升级为 CUDA（acces 记录变化） |
| AC-07 | Go 单测覆盖：包选择互斥、meta 记录真实 backend、未跑则留空 |
| AC-08 | 全量 Checkfix 绿 |
