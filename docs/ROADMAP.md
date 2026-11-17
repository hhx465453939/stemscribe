# 路线图

> CHG 编号即变更契约编号（`tasks/CHG-<id>/`）。**未立项的条目一律不进实现**。
> 依赖关系与并行规则见 `tasks/CHG-0001/TRACEABILITY.md` §4。

## 里程碑

| 版本 | 目标 | 判据 | 对应 CHG |
|---|---|---|---|
| **v0.0.1** | 骨架可构建、CI 双绿、可出安装包 | `wails build` 出 exe；tag 出 zip/setup；AC-01…AC-09 全绿 | CHG-0001 |
| **v0.1.0** | 端到端可用：链接 → 分离 → 转写 → 润色 → 导出 | AC-10…AC-15 全绿；PRD §9 演示脚本 9 步全过 | CHG-0002/0003/0004/0005 |
| **v0.2.0** | Agent 集成与检索 | MCP 工具可用；跨集 FTS 检索；上下文包导出 | CHG-0006 |
| **v0.3.0** | 批量与质量增强 | 批量队列；说话人分离；多语言 | 待立项 |
| **v1.0.0** | 稳定发布 | 无已知 P0 缺陷；文档齐全；有 3 位以上外部用户反馈 | 待立项 |

## CHG 队列（按依赖排序）

### CHG-0002 · 引擎与运行时（核心风险最高）

| 项 | 内容 |
|---|---|
| 交付 | `internal/engine/{protocol,supervisor,whispercpp,separator,ffmpeg}`、`internal/registry`、`internal/downloader`、`internal/resource` |
| 关键任务 | ① whisper.cpp 多后端二进制接入与 `probe` ② audio-separator 运行时方案定稿（Python-embed vs 纯 ONNX）③ NVML 探测与预算器 ④ 下载器（分片/续传/sha256/原子写）⑤ registry 实测填 sha256 |
| 风险 | 体积预算、CUDA 变体匹配、Python 运行时打包 |
| 完成判据 | `probe` 在 cuda/vulkan/cpu 三后端均返回正确；下载 3 个真实模型并校验通过；`RESOURCE-SCHEDULER.md` §9 全部实测回填 |

### CHG-0003 · 流水线

| 项 | 内容 |
|---|---|
| 交付 | `internal/pipeline/*`、`internal/store/*`、`internal/pipeline/source`（B 站 + 本地） |
| 关键任务 | ① 9 阶段 DAG 与状态机 ② checkpoint + 指纹校验续跑 ③ align 修复（跨段句子/幻觉/低置信度）④ 导出 srt/vtt/md ⑤ bundle 读写 ⑥ SQLite 队列与 FTS |
| 完成判据 | AC-10/AC-15 绿；强杀重启续跑验证通过；30 min 真实音频端到端成功 |

### CHG-0004 · LLM 层（可与 0002/0003 并行）

| 项 | 内容 |
|---|---|
| 交付 | `internal/provider/*`、`internal/vault/*`、`prompts/*`、`known-models.json` |
| 关键任务 | ① openai_compat + anthropic 适配器 ② 刷新模型 + 三层能力合并 ③ 参数投影与置灰 ④ SSE 流式 ⑤ 长文分块与锚点守恒 ⑥ DPAPI vault |
| 完成判据 | AC-11/AC-13 绿；对真实 Ollama 与一家线上 provider 各跑通一次润色 |

### CHG-0005 · 界面

| 项 | 内容 |
|---|---|
| 交付 | `frontend/**`（Next.js 15 static export + Tailwind + 设计 token） |
| 关键任务 | ① 设计 token 与基础组件（按 `UI-DESIGN.md`）② 向导 ③ 看板 ④ 单集详情（波形+播放器+四个 Tab）⑤ 模型中心 ⑥ Provider 设置（参数表 + 置灰说明） |
| 完成判据 | PRD §9 演示脚本前 7 步人工走通；`UI-DESIGN.md` §11 反模式清单 0 命中 |

### CHG-0006 · 调度强化与 Agent 集成

| 项 | 内容 |
|---|---|
| 交付 | `internal/mcpserver/*`、`resource` 的并发控制、`store` 的 FTS 查询层 |
| 关键任务 | ① MCP stdio + http 传输 ② 三个工具 ③ 并发预算 ④ 上下文包生成 |
| 完成判据 | 在 Claude/Codex 任一 client 中注册成功并完成一次带时间戳的检索问答 |

## 明确不排期的想法（避免"画大饼"）

| 想法 | 为什么不排 |
|---|---|
| 手机端 / Web 版 | 需要服务端与存储架构，与本地优先定位冲突 |
| 模型微调 | 与本项目"推理编排"定位无关 |
| 视频剪辑 | 另属一个产品 |
| 云端转写加速 | 违背 N-05 隐私底线 |
| 自动发布到播客/公众号 | 合规与版权风险 |

## 排期原则

1. **不写"X 天"**：本项目的进度以 AC 绿否为准，不以日期为准。
2. **风险前置**：CHG-0002 是最大不确定源（体积/CUDA/许可），先做，失败则触发 ADR-002 的备选路径（纯 ONNX C++ 分离器）。
3. **可发布性持续成立**：每完成一个 CHG 都必须保持"可构建 + CI 绿 + 可出包"，不允许长期红着主干。
