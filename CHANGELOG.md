# Changelog

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [SemVer](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Added

- **立项**：`tasks/CHG-0001` 变更契约（SPEC / PRD / TRACEABILITY / CHG），status = approved
- **架构决策** 8 条 ADR：桌面壳 Wails v2 · 引擎外置进程 · Ollama 式模型分发 · 显存预算调度 · LLM Provider 三层能力 · 自包含 bundle · 本地 MCP server · DPAPI 密钥
- **文档**：`ARCHITECTURE` / `PIPELINE` / `MODEL-REGISTRY` / `RESOURCE-SCHEDULER` / `LLM-PROVIDERS` / `UI-DESIGN` / `UPSTREAM-RESEARCH` / `ROADMAP`
- **Schema**：`model-registry.schema.json` / `provider.schema.json` / `bundle-meta.schema.json`
- **设计系统**：暗色瑞士网格 × 单一聚光基因；4 色 token、2px 唯一圆角、flat 层级、等宽数据面
- **上游调研**：UVR / Buzz / WhisperDesktop / bilibili-video-downloader 的机制对照与「不照抄清单」

### Notes

- 本版本**不含可运行代码**。可构建骨架与 CI 见 `docs/ROADMAP.md` 的 CHG-0001 B 阶段。
- registry 中 `sha256` 暂为 `null` + `tbd:true`（待 CHG-0002 实测回填），不编造校验值。

## 版本规划

| 版本 | 内容 |
|---|---|
| v0.0.1 | 骨架可构建、CI 双绿、可出安装包 |
| v0.1.0 | 端到端可用：链接 → 分离 → 转写 → 润色 → 导出 |
| v0.2.0 | Agent 集成（MCP）与跨集检索 |
| v0.3.0 | 批量队列、说话人分离、多语言 |
| v1.0.0 | 稳定发布 |

详见 [`docs/ROADMAP.md`](docs/ROADMAP.md)。
