# CHG-0018 · SPEC — 分离模型族与 Demucs 嵌入（设计稿）

> 立项设计。实施顺序与出口判据见 PRD 的 M1–M6；M1 未通过则本 SPEC 的 torch 部分冻结，
> 只交付 ONNX 模型扩容（MDX/VR 之外的 ONNX 存量模型）。

## 1. 运行时设计

### 1.1 为什么是「一个 venv、两种设备档」

`audio-separator` 的 MDX-Net 走 ONNX Runtime，VR/Demucs/MDX23C/Roformer 走 PyTorch；
两者都在同一个包内，依赖已共同安装（torch 本来就是 audio-separator 的依赖，只是 PyPI
给的是 CPU 版）。因此不引入第二套 venv，只把 torch 轮子换成 CUDA 版：

| 档位 | 包 | 实测/预计体积 |
|---|---|---|
| CPU（现状） | `audio-separator` + `onnxruntime` + `audioread` | 1.0 GB（CHG-0017 实测） |
| CUDA（CHG-0017） | 上述 + `onnxruntime-gpu==1.21.1` + nvidia cu12 轮子 | 3.7 GB（实测） |
| CUDA + torch 架构（本 CHG） | 上述，把 `torch` 换成 `torch==2.5.1+cu121` | 约 6.1 GB（3.7 − 0.2 + 2.45，实施时实测） |

CPU 档也可以跑 torch 架构（VR/Demucs 在 CPU 上可用，只是慢），所以「架构」和「设备」
是两张正交的表：架构决定模型文件与代码路径，设备决定同一个 venv 里哪套库生效。
这避免了「装 Demucs 才能用 VR」这类无意义的组合约束。

### 1.2 安装与探测

- `separatorPackages(accel)`：CUDA 档追加
  `torch==2.5.1+cu121`（`--index-url https://download.pytorch.org/whl/cu121`，失败回退阿里云
  pytorch-wheels 镜像；两者都不可达则安装失败并给出明确错误，不降级到 CPU 冒充 CUDA）。
- 安装顺序：先卸 `torch`，再装 CUDA 轮子；避免 pip 解析回 CPU 版。
- 探针扩展（`separatorProbeScript`）：

  ```
  torch.zeros(1, device="cuda").sum().item()   -> TORCH:CUDA
  torch.zeros(1).sum().item()                  -> TORCH:CPU
  ```

  `INSTALLED.json` 增加 `torch_backend`（`cuda12` / `cpu` / `absent`）与 `torch_reason`；
  现有 `accel`（ONNX provider）保持不变。两个探针都必须真实执行成功才记录对应值。
- 升级判定：`accel` 或 `torch_backend` 与目标档位不一致 → 原地升级（同一逻辑已在 CHG-0017）。

### 1.3 helper 设备自证

- ONNX 架构：沿用 CHG-0017 的强制 `onnx_execution_provider` + session provider 记录。
- torch 架构：CUDA torch 装上后，上游 `setup_torch_device` 会自己打开 CUDA 分支；
  helper 额外在 `load_model` 后断言 `model_run` 的参数设备：
  `next(model.parameters()).device.type`，不一致则以 `reason` 记录并降级为 CPU 运行。
- 结果 JSON 扩展：

  ```json
  {
    "backend": "cuda12",
    "runtime": "torch",
    "providers": ["torch.cuda"],
    "stems": {"vocals": "...", "drums": "...", "bass": "...", "other": "..."},
    "requested": "cuda12",
    "available": ["CUDAExecutionProvider", "CPUExecutionProvider", "torch.cuda"],
    "reason": ""
  }
  ```

  `backend` 仍表示设备（cuda12/cpu），`runtime` 表示推理栈（onnx/torch），二者不得混淆。

## 2. 注册表扩展

### 2.1 Schema 增量（向后兼容）

`Model` 增加：

| 字段 | 类型 | 说明 |
|---|---|---|
| `architecture` | `mdx` / `vr` / `demucs` / `mdxc` / `roformer` | 决定运行时路径与参数面 |
| `stems` | 已有，扩展为 2/4/6 | 产物轨数的事实来源（ADR-019） |
| `params` | `{"segment_size": true, "shifts": true, "batch_size": false, ...}` | UI 可开放的参数 |
| `weights_manifest` | 可选 | 多文件模型（Demucs 的 YAML + N 个 `.th`）的说明字段 |

校验新增：`architecture` 必须与文件后缀一致（Demucs 必须含 `.yaml` + `.th`，
MDX 必须 `.onnx`，torch 架构不能只给 `.onnx`）；`stems` 必须含 `vocals`；
非商用许可必须在 `license` 里显式写 `nonCommercial: true`。

### 2.2 候选清单（2026-11-26 由 hf-mirror tree API 实测存在；体积来自 API，sha256 待实测）

来源统一为 `https://hf-mirror.com/lainlives/audio-separator-models/resolve/main/<file>`。

| 模型 | 架构 | 轨 | 文件 | API 体积 |
|---|---|---|---|---|
| Demucs v4 htdemucs | demucs | 4 | `htdemucs.yaml` + `955717e8-8726e21a.th` | 21 B + 84,141,911 B |
| Demucs v4 htdemucs_ft | demucs | 4 | `htdemucs_ft.yaml` + 4×`.th` | 149 B + 4×84,141,271 B |
| Demucs v4 htdemucs_6s | demucs | 6 | `htdemucs_6s.yaml` + `5c90dfd2-34c22ccb.th` | 21 B + 54,996,327 B |
| BS-Roformer-SW | roformer | 2 | `BS-Roformer-SW.yaml` + `.ckpt` | 4,653 B + 699,412,152 B |
| MDX23C-8KFFT-InstVoc_HQ | mdxc | 2 | `MDX23C-8KFFT-InstVoc_HQ.ckpt` + config | 448,101,203 B + config |
| UVR-MDX-NET Inst HQ 3 | mdx | 2 | `UVR-MDX-NET-Inst_HQ_3.onnx` | 66,759,214 B |
| 5_HP-Karaoke-UVR | vr | 2 | `5_HP-Karaoke-UVR.pth` | 126,782,699 B |
| UVR-DeEcho-DeReverb | vr | 2 | `UVR-DeEcho-DeReverb.pth` | 223,650,277 B |

> 实施时先剔除许可证不允许或质量不达标的模型；目标 ≥8 条中允许包含现有
> `uvr-mdx-kim-vocal-2` 以外的模型。**每条都要下载后实测 sha256 才能写进 registry**，
> 未实测的条目保持 `tbd:true`，install 拒绝安装（规则不变）。

### 2.3 模型安装与工作目录

```
<workdir>/models/separator/<model-id>/
  htdemucs_ft.yaml
  f7e0c4bc-ba3fe64a.th ...（4 个）
  VERIFIED.json
```

`files[].dest` 全部平铺到该目录（Demucs 的 YAML 按文件名引用同目录的 `.th`）；
`status`/`find` 逻辑从「单文件」放宽为「清单全在」。

## 3. 流水线

### 3.1 接口

```go
// 分离产物按模型声明的轨数返回；vocals 必须存在，其余键来自 registry.stems。
Separate(ctx, inPath, outDir string, opts SeparateOptions) (map[string]string, error)
```

`pipeline.SeparateOptions` 增加 `Shifts`、`BatchSize`；`SegmentSecs`/`OverlapSecs` 变为真正
传给 audio-separator 的参数（通过 helper CLI 参数 `--segment/--overlap/--shifts/--batch`）。

### 3.2 bundle 与转写

- `audio/` 落 `vocals.wav`、`instrumental.wav`（若模型产出）、`drums.wav`、`bass.wav`、
  `other.wav`、`guitar.wav`、`piano.wav`（按模型实际轨数）。
- `vocals16k.wav` 仍由 vocals 转出，转写阶段输入契约不变。
- `meta.provenance.engines.separate` 增加 `stems: [...]` 与 `runtime: onnx|torch`；
  `backend` 仍是实际设备。
- 失败语义：某轨缺失视为分离失败（不得静默少轨）；分离失败不拖垮已有逐字稿的规则不变。

### 3.3 导出

导出按声部选择：默认导出 vocals + instrumental（2 轨模型）或全部轨（4/6 轨模型）；
UI 允许勾选；磁盘预检把轨数与 44.1 kHz 立体声体积计入（30 min ≈ 316 MB/轨）。

## 4. UI / 预设

- 模型中心：每条模型显示架构、轨数、单文件/总下载体积、显存估算、许可（NC 标红）；
  安装/升级进度沿用现有作业条。
- 预设（设置页）：
  - 播客 / 口播 → `uvr-mdx-kim-vocal-2`（默认，保持 60 分体验）
  - 音乐 → `htdemucs_ft`（4 轨）
  - 卡拉 OK → `5_HP-Karaoke-UVR` 或 Roformer Karaoke（许可允许时）
- 切换模型只改 settings + 确保模型已装，不需要重装运行时。

## 5. 参数、显存与优化

1. **参数映射**（audio-separator arch 配置）：

   | 参数 | MDX | Demucs | Roformer |
   |---|---|---|---|
   | `segment_size` | `mdx_params.segment_size` | `demucs_params.segment_size` | — |
   | `overlap` | `mdx_params.overlap` | `demucs_params.overlap` | — |
   | `shifts` | — | `demucs_params.shifts` | — |
   | `batch_size` | — | — | `roformer_params.batch_size` |

2. **显存校准**：每次成功运行记录峰值显存（已有 NVML 采样），写 `config/calibration.json`；
   预检用实测值而非 registry 静态值；没有实测时回退 registry 估算并标注「估算」。
3. **热启动**：现在每次分离都要重新 import + 加载模型（实测 120 s 素材里约 8–12 s）。
   设计一个常驻 helper 进程（JSON-lines 协议，一次加载多次分离），把多集批量的启动成本摊薄；
   单任务模式保持一次性进程，二者结果必须逐样本一致。
4. **半精度**：Demucs 支持 `fp16` 推理；实现后必须用相关性 + 抽听证明质量不降，失败就不发布。
5. **降级顺序**：缩小 segment → 降低 shifts/overlap → 换小模型 → 最后拒绝；每一步写日志与 meta。

## 6. 调度与首装体积

- 任务队列仍保证分离与转写不并发（单 GPU）。
- 首装组合（GPU）：whisper cuda 574 MB + large-v3 2.9 GB + ffmpeg 68 MB + 分离运行时 6.1 GB
  + 模型（Demucs ft 约 337 MB / Roformer 约 700 MB）≈ 10 GB 量级。模型中心必须逐项显示，
  不允许只给一个总数。
- 只要人声分离的用户可只装 CUDA ONNX 档（3.7 GB）+ MDX 模型（64 MB），不装 torch 档。

## 7. 基准与验收方法

- 素材：`speech-120s.wav`（人声播客）、`music-120s.wav`（含鼓/贝斯）、`episode-30min.wav`。
- 指标：冷/热启动耗时、RTF、峰值显存（nvidia-smi 1 Hz）、峰值 RSS、输出与 CPU 基线相关性、
  抽听记录（owner 或双人盲听，记录结论）。
- 每次优化必须同素材 A/B，禁止跨素材比较；结果写 `docs/SEPARATION-BENCH.md` 并入 CHG 验证节。
- 验收口径：AC-04 的「≥4×」按 30 min 素材、同 venv 只换设备档位计算（与 CHG-0017 方法一致）。

## 8. 预研证据（复现命令）

```bash
# 权重清单（API 已实测 2026-11-26）
curl -sL "https://hf-mirror.com/api/models/lainlives/audio-separator-models/tree/main?recursive=true"

# torch CUDA 轮子可达性（实测 200，2,449,331,371 B）
curl -sIL "https://download.pytorch.org/whl/cu121/torch-2.5.1%2Bcu121-cp312-cp312-win_amd64.whl"

# 上游架构事实（源码）
sed -n '120,140p' .../audio_separator/separator/architectures/demucs_separator.py
grep -n "torch_device" .../audio_separator/separator/architectures/vr_separator.py
```

## 9. 风险与回退

- **M1 失败**（torch CUDA 轮子装不上或 Demucs 无法在 GPU 跑通）→ 停止 torch 部分，
  只交付 ONNX 架构扩容（MDX 新模型 + VR 若可转 ONNX），并在 CHG 中记录失败原因与已试方案；
  禁止用「安装成功」冒充「GPU 可用」（探针必须真实张量运算）。
- **权重不可达或 sha256 不符** → 该模型不登记，文档记录原因；不换源凑数。
- **许可不明** → 视为不可分发，不登记。
- **性能不达标** → 记录实测，不上「预计」；必要时缩小范围（先交付 Demucs 4 轨，6s 后续）。

## 10. 任务拆分（对应 PRD M1–M6）

| 任务 | 内容 | 验收 |
|---|---|---|
| T1 | torch cu121 安装 + 张量探针 + INSTALLED.json 扩展 | 本机 TORCH:CUDA；CPU 档 TORCH:CPU；单测覆盖判定 |
| T2 | Demucs 模型下载/校验/加载（htdemucs_ft） | 4 轨输出 + nvidia-smi 采样 |
| T3 | 注册表 schema + 8 条模型实测入表 | 每条 sha256 可复算；install 拒绝 tbd |
| T4 | 多轨流水线 + bundle/meta | 4/6 轨端到端，转写输入不变 |
| T5 | UI 模型中心 + 预设 + 许可提示 | 截图与 i18n 检查 |
| T6 | 参数映射 + 校准 + 热启动 | 参数生效表；热/冷启动对比 |
| T7 | 基准、文档、ADR-039 | 文档更新 + Checkfix |
