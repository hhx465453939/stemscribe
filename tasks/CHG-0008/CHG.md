# CHG-0008 · 变更记录

> 契约主体见 `SPEC.md`、`PRD.md`、`TRACEABILITY.md`。本文件只记状态流转与决策增补。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-18 | proposed | 立项：运行参数控制面与后端兼容矩阵 | agent |
| 2026-11-18 | **approved** | owner：「你把我说的这些事，整体去立项然后开发一下吧」 | owner |
| — | in_progress | M1→M7 | agent |
| — | closed | 待 AC-01…AC-10 全绿 | — |

## 需求原文（逐字保留，便于核对）

> 「decum 这种，其实有很多功能，除了 vocal，还有别的，比如 instrumental 分离，除此以外，whisper，对于 input/output 语言是什么，也需要用户选择吧？你能不能把这几个模型的功能，了解详细，然后在这个应用里面，把运行相关的选项（现在没有选项下的运行过程参数就是默认参数），以一种符合本项目美学的形式，标记出来，让用户随时可以选择或者填写，cuda12runtime 之类的，尽量把兼容性，类似 vulkan，cpu 之类的，也要能够在应用内部做好 fallback 逻辑，哪怕有些用户的 amd 显卡，你给他用 3d 渲染去跑这个，也要能够用起来，这个才是最重要的；你把我说的这些事，整体去立项然后开发一下吧」

## 立项前的调研（不凭记忆）

| 做了什么 | 结果 |
|---|---|
| 从自建 `whisper-cli.exe --help` 取真实参数表 | 拿到全部 40+ 参数与默认值，登记进 TRACEABILITY §3.1 |
| 枚举本机 UVR 安装 | 确认三族分离模型与 `UVR-MDX-NET-**Inst**_HQ_3.onnx`（stem 能力属于模型）；找到 `onnxruntime/capi` |
| 确认 Python 可用性 | `F:\software\Miniconda3\python.exe` = 3.13.5（为 CHG-0009 的运行时选型留依据） |

## 决策增补

| # | 增补 | 理由 |
|---|---|---|
| N-1 | **ADR-016** 参数面以 `paramSpecs` 单表驱动 UI/CLI/文档 | 三处各写一遍必然漂移；单表可被脚本比对默认值（AC-02） |
| N-2 | **ADR-017** 输入语言 / 只探测 / 翻译 三者独立 | 用户口语的"输出语言"含三种诉求；塞进一个下拉会出现"选中文却输出英文" |
| N-3 | **ADR-018** 后端优先"能跑起来"：cuda12→vulkan→cpu；DirectML 如实标不支持 | owner 明确要求 AMD 也要能用；Vulkan 是唯一现实路径 |
| N-4 | **ADR-019** stem 是模型属性 | 模型名里就写着 Inst，说明能力属于模型；全局开关必然错配 |
| N-5 | **ADR-020** 参数快照与阶段失效联动 | 参数不可复现 = 产物不可信（项目重心第 1 条） |
| N-6 | **ADR-021** 分离推理引擎移出本 CHG | ONNX CGO 绑定 + STFT/iSTFT 数值实现是独立工程；硬塞会让两边半成品 |

## 下一步（依赖顺序）

1. M1：`internal/engine/params.go`（`WhisperParams` + `paramSpecs` + `Args()` + `Validate()`）+ 覆盖率与冲突测试
2. M2：`whisper.go` 改为从结构体渲染参数
3. M3：registry 加 `stems`，MDX/VR=2、Demucs=4，并让校验强制分离模型必须有 stems
4. M4：`resource` 后端矩阵补 AMD/Intel → vulkan，降级 reason 落到事件
5. M5：参数进 `params_snapshot`，改参使 transcribe 失效
6. M6：前端参数面板（暗色瑞士网格 × 单一光源，双语）
7. M7：文档同步 + 截图
