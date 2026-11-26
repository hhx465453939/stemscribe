# CHG-0018 · PRD — 分离模型扩容与 Demucs 深度嵌入（立项）

## 背景

CHG-0017 让 MDX-Net（ONNX）分离真正跑上了 CUDA，owner 验收「能用了，基本 60 分达到了」，
并把下一阶段定为一个开发立项：

> 「你把这种相关的模型都纳入进来吧，扩增起来……后面的事情，就是 demucs，
> 模型权重和项目适配做一个最大化优化嵌入开发立项就行了。」

现状（源码级事实）：

| 项 | 现在 | 缺口 |
|---|---|---|
| 分离架构 | 只有 **MDX-Net**（`Kim_Vocal_2.onnx`，ONNX Runtime，2 轨） | VR / Demucs / MDX23C / Roformer 全未接入 |
| 推理后端 | ONNX 路径 CUDA 已打通（CHG-0017）；torch 路径是 **CPU 版 torch** | torch 架构（VR / Demucs / MDX23C / Roformer）即便装了 CUDA 也只能跑 CPU |
| 分轨能力 | 固定 vocals + instrumental | 4 轨（drums/bass/other/vocals）、6 轨（+guitar/piano）没有数据模型与产物路径 |
| 模型元数据 | 2 条模型；`stems` 字段已存在但只用于校验 | 架构、显存、参数支持面、许可展示、权重来源都没有结构化登记 |
| 参数 | `SegmentSecs` 只是占位，未参与计算 | segment / shifts / overlap / batch 对用户不可用 |

`audio-separator 0.48.0` 本身已内置四类架构的实现与模型清单（源码实测）：
`mdx_separator.py`（ONNX）、`vr_separator.py`（torch）、`demucs_separator.py`（torch）、
`mdxc_separator.py`（torch），模型清单在 `models-scores.json`（含 Demucs v4 / MDX23C /
Roformer / VR / MDX-Net 各族）。缺的是本项目的运行时、注册表、流水线和 UI 把它们变成产品能力。

## 预研证据（2026-11-26，不是假设）

1. **Demucs 权重在允许的下载源上可取**：`hf-mirror.com/lainlives/audio-separator-models`
   仓库（已在用，Kim Vocal 2 就来自这里）经 HF tree API 实测包含
   `htdemucs.yaml`、`htdemucs_ft.yaml`、`htdemucs_6s.yaml` 与 `.th` 权重
   （htdemucs 84,141,911 B；htdemucs_ft 四件各 84,141,271 B；6s 54,996,327 B），
   以及 `BS-Roformer-SW.ckpt`（699,412,152 B）、`MDX23C-8KFFT-InstVoc_HQ.ckpt`（448,101,203 B）、
   VR `.pth`、更多 MDX-Net `.onnx`。**体积来自仓库 API**；sha256 必须在实施阶段逐文件实测。
2. **torch CUDA 轮子可达**：`download.pytorch.org` 实测 200，`torch-2.5.1+cu121-cp312-cp312-win_amd64.whl`
   `Content-Length` = 2,449,331,371 B。CUDA 12.1 与本机驱动 566.36、现有 ORT/whisper CUDA 12 同代。
3. **torch 架构的设备门控是上游设计**：VR/MDXC/Demucs/Roformer 都经 `torch.load … .to(torch_device)`；
   PyPI 的 Windows torch 是 CPU 版，所以这些架构当前必然跑 CPU —— 与 CHG-0017 修掉的 ONNX
   门控是同一类问题，需要在 helper 层显式指定并自证设备。

## 目标（可判定）

- **G1 Demucs 可用**：`htdemucs` / `htdemucs_ft` / `htdemucs_6s` 至少三个模型可在 GPU 上完成分离，
  输出 4 轨（6s 为 6 轨），CPU 可降级且显性标注。
- **G2 模型扩容**：注册表新增 ≥8 个可安装模型，覆盖 MDX-Net / VR / MDX23C / Roformer / Demucs
  五族；每条都有实测 sha256、体积、许可、轨数与显存估算。
- **G3 产品嵌入**：模型中心按「架构 / 轨数 / 显存 / 体积 / 许可」展示并可一键安装；
  流水线按模型声明的轨数出产物；转写仍只读 `vocals16k.wav`。
- **G4 参数真正生效**：segment / shifts / overlap / batch 对支持的架构生效，非法值拒绝，
  并给出显存—速度—质量对照表。
- **G5 设备自证沿用**：任何架构、任何运行，`engine.log` / `meta.json` / capabilities 三处
  一致报告实际设备；降级必有原因。
- **G6 性能与体积有实测**：同素材 CUDA vs CPU 加速比、峰值显存、安装体积、模型体积全部实测入档；
  目标 CUDA 分离相对 CPU ≥4×（30 min 素材）。

## 需求

- **R1 运行时双设备档**：CUDA 档 = `onnxruntime-gpu` + **CUDA 12 torch 轮子**（替换 CPU 版），
  CPU 档 = 现有 CPU 依赖。两种档位都有探针记录（ORT provider + `torch.cuda` 张量实测）。
- **R2 单 venv 承载多架构**：MDX（ONNX）与 torch 架构共用一个 Python 环境；不引入第二套依赖树。
- **R3 权重来源合规**：只从 hf-mirror 允许的仓库下载；每条 registry 记录都必须有实测 sha256，
  未实测的条目保持 `tbd:true` 且安装被拒绝（沿用既有硬规则）。
- **R4 模型清单结构化**：registry 增加架构、轨数、参数支持面、许可；Demucs/Roformer/MDX23C 的
  YAML/config 与权重一起作为 `files[]` 登记并校验。
- **R5 多轨流水线**：分离阶段按 `stems` 输出；bundle 落全部声部 + `vocals16k.wav`；
  `meta.provenance.engines.separate` 记录模型、设备与轨数。
- **R6 UI**：模型中心与参数页展示架构/轨数/显存/许可；提供「播客（人声+伴奏）」「音乐（4 轨）」
  「卡拉 OK（人声消除）」预设；非商用许可模型必须显式标注、默认不进预设。
- **R7 参数映射**：MDX 的 `segment_size`、Demucs 的 `segment_size/shifts/overlap`、
  Roformer 的 `batch_size` 映射到 audio-separator 配置并有边界校验。
- **R8 调度**：显存预算改用实测值；分离与转写不并发；Demucs 峰值显存超过预算时
  按参数降级（segment 缩小 → shifts 降低），最后才拒绝运行。
- **R9 基准**：固定素材（语音 2 min、音乐 2 min、整集 30 min）+ 指标（冷/热启动耗时、RTF、
  峰值显存、RSS、与 CPU 输出相关性、抽听结论），结果写 `docs/SEPARATION-BENCH.md`。
- **R10 无回归**：默认仍是 MDX-Net Kim Vocal 2（人声分离质量与耗时不得变差）；
  CHG-0017 的 CUDA/降级/自证行为对全部架构成立；Checkfix 全绿。

## 非目标

- 不做云端分离、不做模型训练/微调、不做模型蒸馏。
- 不做 ensemble（多模型加权融合）与分段级质量打分——留作后续 CHG。
- 不改变 bundle 的转写输入契约（始终是分离后的 `vocals16k.wav`）。
- 不镜像/转存第三方权重到本项目仓库或 Release（只登记来源 + 校验值）。

## 验收

| AC | 判据 |
|---|---|
| AC-01 | `htdemucs_ft` 在 RTX 3060 上 GPU 分离成功，`nvidia-smi` 采样显存/利用率上升；输出 4 轨 |
| AC-02 | `htdemucs_6s` 输出 6 轨，UI 与 bundle 一一对应，不丢轨 |
| AC-03 | 至少 8 个新模型进入 registry，sha256 全部实测；未实测条目安装必须被拒绝 |
| AC-04 | 30 min 音乐素材 CUDA 分离相对 CPU ≥4×；120 s 素材给出冷/热启动两档数字 |
| AC-05 | `SegmentSecs` 改动使显存/耗时单调变化（实测表，非推断） |
| AC-06 | 全链四处设备一致（helper JSON / engine.log / meta / capabilities），任一架构降级都带 reason |
| AC-07 | 模型中心展示架构/轨数/显存/许可；非商用模型标注且默认不选 |
| AC-08 | 现有 MDX 默认路径回归：同素材耗时与输出相关性不劣于 CHG-0017 基线 |
| AC-09 | 文档更新（MODEL-REGISTRY / PIPELINE / RESOURCE-SCHEDULER / GAP-ANALYSIS / 新 ADR-039）与 Checkfix 全绿 |

## 风险与代价

| 风险 | 影响 | 处置 |
|---|---|---|
| CUDA torch 轮子 2.45 GB | CUDA 档安装体积 3.7 → 约 6.1 GB | 档位可选：只要 MDX 的用户不装；模型中心显示实测体积 |
| torch 轮子下载源（download.pytorch.org / 镜像）在目标网络不可达 | Demucs 无法安装 | 安装前可达性探测，失败给出明确错误；不放宽 sha256 |
| 权重许可因模型而异（部分仅限非商用） | 合规风险 | 每条登记许可 + 链接；NC 模型默认不选，UI 显式提示 |
| Demucs 显存高于 MDX（特别是 ft + shifts） | 低显存机器 OOM | 显存预算 + segment/shifts 降级；最后才拒绝 |
| L4/Roformer 大模型（~700 MB）下载慢 | 首次体验差 | 断点续传（已有）；模型中心显示体积与预计时间 |
| audio-separator 上游变动（模型清单、YAML 格式） | 安装/加载失败 | 锁 0.48.x；YAML/清单按现有 UVR 元数据方式内嵌；升级单独 CHG |

## 里程碑（实施阶段）

| 阶段 | 内容 | 出口判据 |
|---|---|---|
| M0 | 立项 | 本 PRD + SPEC 通过 |
| M1 | 可行性收口 | torch CUDA 轮子装上；`htdemucs_ft` 在本机 GPU 跑通并有采样证据 |
| M2 | 运行时与安装器 | CUDA/CPU 档各自探针通过；INSTALLED.json 记录 torch 设备；升级路径可逆 |
| M3 | 注册表与下载 | ≥8 个模型 sha256 实测入表；YAML/权重校验可重放 |
| M4 | 多轨流水线与 UI | 4/6 轨 bundle、meta、模型中心与预设可用 |
| M5 | 参数与优化 | segment/shifts/batch 生效；基准表与 `SEPARATION-BENCH.md` |
| M6 | 文档与验收 | AC-01..AC-09 全绿，Checkfix 通过 |
