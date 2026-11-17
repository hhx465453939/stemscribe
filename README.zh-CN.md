# Stemscribe · 声轨抄录

**把任意视频变成一份可归档、可检索、可对话的文字稿——全程跑在你自己的显卡上。**

面向 Windows 的本地优先流水线：视频 → 音频 → 文字 → 润色。不上传媒体、不开账号、产物可复现。

[English →](README.md)

---

## 为什么做这个

零件都已经有了：B 站下载器、UVR 提人声、Whisper 转写、LLM 润色。
缺的从来不是零件，而是**接缝**——而所有工作量都在接缝上。

| 现在 | 用 Stemscribe |
|---|---|
| 4 个软件，文件靠手拖 | 1 次点击，阶段化流水线 |
| 模型散在 `E:\`、`F:\` 各处 | 全部装在**一个可整体搬走的工作目录** |
| 「上次用的哪个模型？」 | 每份产物记录引擎版本、模型指纹、参数快照 |
| 180 集要手工重复 180 次 | 批量队列，断点续跑，失败单条重试 |
| 稿子躺在聊天窗口里 | 结构化档案，可 grep、可备份、可直接交给 agent |

## 它做什么

```
来源 ──► 下载 ──► 抽轨 ──► 分离 ──► 转写 ──► 对齐 ──► 润色(可选) ──► 导出
(链接/                    (人声/伴奏)  (whisper.cpp  (句子    (任意 LLM)   (档案+字幕)
 本地文件)                              GGML，GPU)   修复)
```

几个关键取舍：

- **本地优先**：音视频字节永不出本机；只有**文本**可能发往 LLM，且必须由你显式开启。
- **显存自己管**：NVML 读真实显存 → 预算 → 模型 LRU 常驻 → 分段流式 → 四级降级（CUDA→Vulkan→DirectML→CPU），降级原因会明确告诉你，不静默。
- **模型自己装**：内置 registry 声明「去哪下、多大、sha256」，下载支持断点续传 + 校验 + 原子落盘。装好的模型可整目录拷到另一台机器。
- **LLM 随你换**：填 URL + API Key → 点「刷新」→ 拉模型列表 → 逐模型配参数。上下文窗口（常见 1M）、输出上限、推理深度、多模态、function call、采样参数，全部可见可改；端点不支持的字段会**置灰并说明原因**，不隐藏。
- **可复现**：每集一个自包含目录；改参数只让依赖它的阶段失效，不整集重跑。

## 安装

> 状态：**v0.0.x 骨架阶段**，可用流水线在 v0.1.0 落地。进度见 [`docs/ROADMAP.md`](docs/ROADMAP.md)。

- 免安装 portable zip，不需要管理员权限
- 需要 WebView2 运行时（Win10 1803+ / Win11 自带）
- 没有 NVIDIA 显卡也能用（CPU 路径始终可用）
- **构建未签名**：SmartScreen 会警告。请用 release 附带的 `SHA256SUMS.txt` 校验

## 从源码构建

```powershell
# 前置：Go ≥1.23、Node ≥20、pnpm ≥9、MSYS2（Wails 需要 GCC）、WebView2 运行时
pnpm install
pnpm --dir frontend install
.\scripts\dev.ps1           # 开发：wails dev + next dev
.\scripts\build.ps1         # 产出 build/bin/Stemscribe.exe
.\scripts\smoke-test.ps1    # 端到端冒烟（合成音频，不联网）
```

CI 跑在 `windows-latest`；推 tag（`v0.1.0-rc1`）自动出安装包与 portable zip。

## 界面

暗色瑞士网格 + 单一光源——是仪表，不是后台面板。
设计基因、token、组件规范与反模式清单：[`docs/UI-DESIGN.md`](docs/UI-DESIGN.md)。

## 架构一览

| 层 | 选型 |
|---|---|
| 壳 | Wails v2（Go + WebView2），单文件约 25 MB |
| 前端 | Next.js 15 静态导出 |
| 推理 | 外置引擎进程，JSONL 协议（`whisper.cpp` / `audio-separator` / `ffmpeg`） |
| 模型 | registry 声明 + 按需下载 + sha256 校验 + 原子安装 |
| 调度 | NVML 显存预算 → LRU 常驻 → 分段流式 → 四级降级 |
| LLM | OpenAI-compatible + Anthropic 适配器，能力三层解析 |
| Agent | 本地 MCP server（检索 / 取原文 / 上下文包） |

详见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)；决策记录见 [`docs/DECISIONS.md`](docs/DECISIONS.md)。

## 文档

| 文档 | 内容 |
|---|---|
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | 分层、进程模型、IPC 契约、错误模型 |
| [`docs/PIPELINE.md`](docs/PIPELINE.md) | 阶段 DAG、幂等续跑、对齐修复、失败矩阵 |
| [`docs/MODEL-REGISTRY.md`](docs/MODEL-REGISTRY.md) | registry schema、下载校验、许可合规 |
| [`docs/RESOURCE-SCHEDULER.md`](docs/RESOURCE-SCHEDULER.md) | 显存预算、LRU、分段、内存/IO 策略 |
| [`docs/LLM-PROVIDERS.md`](docs/LLM-PROVIDERS.md) | provider 抽象、能力解析、参数投影 |
| [`docs/UI-DESIGN.md`](docs/UI-DESIGN.md) | 设计基因、token、组件、反模式 |
| [`docs/UPSTREAM-RESEARCH.md`](docs/UPSTREAM-RESEARCH.md) | 从 UVR / Buzz / WhisperDesktop / 下载器借鉴了什么 |
| [`tasks/CHG-0001/`](tasks/CHG-0001/) | 立项变更契约（SPEC / PRD / TRACEABILITY） |

## 致谢

Stemscribe 做的是编排，功劳在别人：

- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) — MIT
- [Ultimate Vocal Remover](https://github.com/Anjok07/ultimatevocalremovergui) — 分离模型体系与分段实践
- [audio-separator](https://github.com/nomadkaraoke/python-audio-separator) — ONNX 分离运行时
- [Buzz](https://github.com/chidiwilliams/buzz) — 16 kHz 流水线与 compute type 实践
- [Const-me/Whisper](https://github.com/Const-me/Whisper) — 单文件分发与模型常驻
- [bilibili-video-downloader](https://github.com/lanyeeee/bilibili-video-downloader) — 桌面打包参考
- [FFmpeg](https://ffmpeg.org/) — LGPL/GPL，运行时下载

**本项目不再分发任何第三方模型权重。** registry 只记录去哪取、怎么校验。

## 许可

MIT，见 [`LICENSE`](LICENSE)。第三方声明见 `THIRD-PARTY-NOTICES.md`（随首个运行时集成加入）。

> 遵守所在平台的服务条款与版权法规。请只处理你有权处理的素材。
