# CHG-0018 · CHG Log

## 变更摘要

在 CHG-0017（MDX-Net 分离真正跑上 CUDA）验收后，owner 定的下一阶段：
**把分离模型族扩起来，并把 Demucs / 模型权重 / 项目适配做成最大化的深度嵌入**。
本 CHG 目前是**立项**（PRD + SPEC），代码尚未开始。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-26 | proposed | owner 指示立项（「后面的事情，就是 demucs，模型权重和项目适配做一个最大化优化嵌入开发立项」）；完成预研与 PRD/SPEC | agent |
| — | approved | 待 owner 确认范围后开工 M1（可行性收口） | owner |

## 预研结论（2026-11-26 实测）

1. 权重可得且合规：`hf-mirror.com/lainlives/audio-separator-models` 已含 htdemucs/htdemucs_ft/
   htdemucs_6s 的 YAML 与 `.th`、BS-Roformer、MDX23C、VR `.pth`、更多 MDX-Net `.onnx`。
   体积来自仓库 API；sha256 实施时逐文件实测（未测不登记、install 拒绝）。
2. torch CUDA 轮子可达：`torch-2.5.1+cu121-cp312-cp312-win_amd64.whl` = 2,449,331,371 B，
   与现有 CUDA 12 运行时同代，驱动 566.36 满足要求。
3. 架构事实：MDX-Net 走 ONNX（CHG-0017 已 GPU）；VR/Demucs/MDX23C/Roformer 走 torch，
   PyPI 的 Windows torch 是 CPU 版——这正是这些架构现在必然跑 CPU 的原因。

## 待定的关键决策（默认值已写进 SPEC，owner 不反对即按此执行）

| 决策 | 默认 |
|---|---|
| 运行时形态 | 单 venv 双设备档；CUDA 档换 CUDA torch（约 6.1 GB），CPU 档不变 |
| 默认模型 | 仍 `uvr-mdx-kim-vocal-2`（人声分离），预设另给「音乐 4 轨」「卡拉 OK」 |
| 新模型数量 | ≥8 条，许可/质量不过关的剔除，不凑数 |
| 4/6 轨导出 | 默认全导，UI 可勾选 |
| 非商用许可模型 | 登记但默认不选，UI 标红 + 链接 |

## ADR-039（提案）

**分离模型族扩展：单一运行时 + 设备/架构正交登记 + 逐模型实测**

- 不为每个模型建 venv；设备（cuda12/cpu）与推理栈（onnx/torch）分开登记、分别自证。
- 模型能力（轨数、参数面、许可）在 registry 上，不在全局开关（延续 ADR-019）。
- 任何模型、任何设备，`engine.log` / meta / capabilities 三处必须一致；降级必须写原因。
- 权重只从 hf-mirror 允许仓库取；sha256 未实测不登记，install 拒绝。

## 下一步

1. owner 确认 PRD/SPEC 范围（或直接按默认执行）。
2. M1：本机装 CUDA torch，`htdemucs_ft` 跑通并采样显存/计时；失败则 torch 部分冻结，
   只交付 ONNX 模型扩容，并记录失败原因与已试方案。
3. M2–M6 按 SPEC §10 的任务表推进，每个里程碑独立 commit。
