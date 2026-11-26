# CHG-0018 · CHG Log

## 变更摘要

在 CHG-0017（MDX-Net 分离真正跑上 CUDA）验收后，owner 定的下一阶段：
**把分离模型族扩起来，并把 Demucs / 模型权重 / 项目适配做成最大化的深度嵌入**。
截至 2026-11-27：M1–M4 与 M6 参数面已落地并实测；M5（模型中心展示）已实现、待截图核对；
M6 的显存校准/热启动与长期基准文档未做，如实列在「下一步」。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-26 | proposed | owner 指示立项（「后面的事情，就是 demucs，模型权重和项目适配做一个最大化优化嵌入开发立项」）；完成预研与 PRD/SPEC | agent |
| 2026-11-26 | in_progress | owner「开工」；M1 可行性实测通过（见下节），进入 M2 | agent |
| 2026-11-26 | in_progress | M2 torch 运行时档跑通并取证（就地升级 + 生产 venv Demucs + MDX 回归） | agent |
| 2026-11-27 | in_progress | M3 8 条模型入库、M4 多轨全链路、M6 参数面全部实测通过；M5 模型中心展示实现 | agent |

## 预研结论（2026-11-26 实测）

1. 权重可得且合规：`hf-mirror.com/lainlives/audio-separator-models` 已含 htdemucs/htdemucs_ft/
   htdemucs_6s 的 YAML 与 `.th`、BS-Roformer、MDX23C、VR `.pth`、更多 MDX-Net `.onnx`。
   体积来自仓库 API；sha256 实施时逐文件实测（未测不登记、install 拒绝）。
2. torch CUDA 轮子可达：实测选定 `torch-2.14.1+cu126-cp312-cp312-win_amd64.whl`
   （SJTU PyTorch 镜像，2,602,768,778 B，sha256 `97e3bc87…115a4`），
   `torchvision-0.29.1+cu126` 6,111,898 B（sha256 `ab045bfb…b96`）。
   **修正立项初稿**：2.5.1+cu121 会违反 torchvision 0.29.2 的 `torch>=2.14.0` 约束，
   且本机驱动 566.36 同时支持 cu126；改用 2.14.1+cu126。

## M1 可行性实测（2026-11-26，详见 tmp/evidence/chg0018/M1-FEASIBILITY.md）

| 项 | 结果 |
|---|---|
| CUDA torch | 2.14.1+cu126，`torch.cuda.is_available()==True`，CUDA 张量 matmul 成功 |
| 权重 | htdemucs_ft.yaml + 4×84,141,271 B `.th`，5 个 sha256 与 audio-separator 发布的 `model-checksums.json` 全部一致 |
| GPU 分离 | 120 s 素材 50.48 s（RTF 0.42），峰值显存 3794 MiB，GPU util 100%，4 轨 |
| CPU 基线 | 30 s 素材 124.36 s（同素材 GPU 17.54 s，**7.09×**） |
| 数值一致性 | Vocals 与 CPU 基线 corr 0.999973 / SNR 42.7 dB |
| 离线装载 | `separator_meta/download_checks.json`（CHG-0017 已嵌入）即可让 audio-separator 不触网 |

结论：torch 部分可行，PRD/SPEC 的 M2–M6 不再冻结。

## M2 运行时 torch 档（2026-11-26，详见 tmp/evidence/chg0018/M2-RUNTIME-TORCH.md）

| 项 | 结果 |
|---|---|
| 触发 | registry `architecture` 需要 torch 且模型已装 → 安装时带 torch 档；装 torch 模型后自动补齐 |
| torch 索引 | SJTU（实测 7.7 MB/s）→ 官方 → 阿里云，`STEMSCRIBE_TORCH_INDEX` 可覆盖；全失败即报错，不退回 CPU 冒充 CUDA |
| 就地升级 | 38.67 s（uv 缓存），INSTALLED.json `torch_backend=cuda`，venv 7,243 MB |
| 生产 venv Demucs | 30 s 素材 17.14 s，`SEP_TORCH_DEVICE cuda`，4 轨 |
| MDX 回归 | `backend=cuda12`，CUDA provider 仍激活，两轨正常 |
| 探针拆分 | ONNX 会话与 torch 必须分进程探测，否则 torch 报 cudnn WinError 127（见证据） |

3. 架构事实：MDX-Net 走 ONNX（CHG-0017 已 GPU）；VR/Demucs/MDX23C/Roformer 走 torch，
   PyPI 的 Windows torch 是 CPU 版——这正是这些架构现在必然跑 CPU 的原因。

## M3 模型族入库（2026-11-27，详见 tmp/evidence/chg0018/M3-MODELS.md）

| 项 | 结果 |
|---|---|
| 入库 | 8 条 separator：Kim / MDX Voc FT / MDX Inst HQ3 / MDX Karaoke（mdx）· 5_HP-Karaoke（vr）· htdemucs / htdemucs_ft / htdemucs_6s（demucs，4/4/6 轨） |
| 校验 | 每条都过三道：`Content-Length` 对体积、全文件 sha256 对 audio-separator 发布的 `model-checksums.json`（243 条）、MDX/VR 还要求尾部 10 MB 的 MD5 在 `*_model_data.json` 内 |
| 移除 | `uvr-vr-hp2-vocals`：尾部 MD5 不在上游参数表（内嵌与镜像副本逐字节一致，30 条），加载必抛 `Unsupported Model File`；原条目 sha256 为 tbd、从未装成功 |
| 不入库 | `UVR-DeEcho-DeReverb`：校验全过，但产物是「No Reverb」两轨，不是 vocals，超出分离契约；Roformer/MDX23C 体积大、参数面未开放、许可未证实，本轮不做 |
| 许可 | Demucs 权重 CC-BY-NC-4.0（`nonCommercial: true`，依 facebookresearch/demucs#327）；UVR 系权重标 MIT + note 说明上游未单独声明权重许可 |

## M4 多轨流水线（2026-11-27，详见 tmp/evidence/chg0018/M4-PIPELINE.md）

| 项 | 结果 |
|---|---|
| 端口 | `Separate` 返回 `map[stem]path`；`requireStems` 缺轨即失败；`MarkDoneWith` 按真实产物记账 |
| 端到端 | MDX 2 轨、htdemucs 4 轨、htdemucs_6s 6 轨、htdemucs_ft 4 轨全部跑通；`meta.provenance` 记 `runtime=onnx/torch` + `stems` |
| 设备 | Demucs 三次运行 `engine.log` 均为 `backend=cuda12 runtime=torch device=cuda`，非自述 |
| 安装 | 多文件模型「清单全在」才算已装；入口按架构选（Demucs 交 YAML）；`VERIFIED.json` 记 `files[]` |
| 转写 | 输入仍是 `vocals16k.wav`，契约未变（4 轨集 11 段） |
| 恢复 | 改参数 → `resume: separate: parameters changed` 重跑；原样重跑全部 skipped |

## M6 参数面（2026-11-27，详见 tmp/evidence/chg0018/M6-PARAMS.md）

| 项 | 结果 |
|---|---|
| 支持矩阵 | mdx：segment/overlap/batch；vr：batch；demucs：segment(秒)/overlap/shifts；mdxc：segment/overlap/batch；roformer：0.48 无参数面 |
| 链路 | CLI `--segment/--overlap/--shifts/--batch/--fp16` → 按 registry `params` 校验 → helper 以完整默认字典为底覆盖 → 日志记实际生效值 |
| 实测 | `arch=demucs params=map[overlap:0.5 shifts:1]`；MDX 传 `--shifts` 在启动前被拒（`has no shifts setting`） |
| fp16 | 仅 torch+cuda 生效，默认关闭（质量未测量，不设默认） |
| 未做 | 显存校准写 `calibration.json`、热启动常驻 helper、`docs/SEPARATION-BENCH.md` |

## 待定的关键决策（默认值已写进 SPEC，owner 不反对即按此执行）

| 决策 | 默认 |
|---|---|
| 运行时形态 | 单 venv 双设备档；CUDA 档按需加 CUDA torch（实测 7,243 MB），CPU 档不变 |
| 默认模型 | 仍 `uvr-mdx-kim-vocal-2`（人声分离），模型中心可一键切换；预设见 SPEC §4 |
| 新模型数量 | 8 条；许可/质量不过关的剔除（HP2、DeEcho、Roformer/MDX23C 本轮不入） |
| 4/6 轨导出 | 默认全导，UI 可勾选 |
| 非商用许可模型 | 登记但默认不选，模型中心标红（`tag--nc`）+ 悬停说明 |

## ADR-043（提案）

> 编号说明：原稿写 ADR-039，与 CHG-0015 已用的 ADR-039 冲突；040/041/042 也已被占用，顺延为 043。

**分离模型族扩展：单一运行时 + 设备/架构正交登记 + 逐模型实测**

- 不为每个模型建 venv；设备（cuda12/cpu）与推理栈（onnx/torch）分开登记、分别自证。
- 模型能力（轨数、参数面、许可）在 registry 上，不在全局开关（延续 ADR-019）。
- 任何模型、任何设备，`engine.log` / meta / capabilities 三处必须一致；降级必须写原因。
- 权重只从 hf-mirror 允许仓库取；sha256 未实测不登记，install 拒绝；MDX/VR 还要求
  尾部 MD5 在钉住的参数表内——「能下载」不等于「能加载」。

## 下一步

1. M5 收尾：模型中心的架构/轨数/非商用标记已落地，截图与双主题核对。
2. M6 剩余：显存校准、热启动常驻 helper、`docs/SEPARATION-BENCH.md`。
3. T7：Checkfix 全绿、redact/secrets 扫描、按里程碑提交；push 待 owner 明确授权。
