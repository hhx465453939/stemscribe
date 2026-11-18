# PRD · CHG-0002 · 真实推理引擎与运行时

> 上游需求来源：owner 会话指令 2026-11-17「可以，继续吧」（承接上一轮提议：开 CHG-0002，把合成引擎换成真 whisper.cpp）

## 1. 问题与用户

| 用户 | 场景 | 现状的痛 |
|---|---|---|
| 想真正用起来的人（主用户） | 把一集 42 分钟访谈丢进客户端，期望拿到逐字稿 | **`run` 跑的是合成引擎**：输出"合成语音片段 1"这类占位文本。整条链路搭好了，但一个字也不是从视频里来的 |
| 有 NVIDIA 卡的人 | 期望用上显卡 | 现在没有任何后端被真正调用；"cuda12 · ready"是写死的字符串 |
| 没有显卡的人 | 期望至少能跑 | CPU 路径理论上可用，但没有任何引擎可执行文件 |

真实需求不是"接一下 whisper"，而是：**让"下载 → 校验 → 安装 → 调用 → 降级 → 出真实文本"这条链在用户机器上第一次就成立**，而且引擎来源可审计。

## 2. 目标

| # | 终态（可判定） |
|---|---|
| G-1 | `stemscribe run --input <真实视频>` 产出的 `transcript.json` 内容来自音频本身，不是占位文本 |
| G-2 | 引擎二进制由**本项目 CI 自建**并以 release 资产发布，附 `SHA256SUMS`；registry 的 sha256 为实测值，不再是 `tbd` |
| G-3 | 模型按需下载、sha256 校验、原子落盘、可断点续传；下载源为官方上游 |
| G-4 | 有 GPU 走 GPU（Vulkan），无 GPU 自动落 CPU，降级原因可见 |
| G-5 | 视频先经 ffmpeg 转成 whisper 要求的 16 kHz 16-bit 单声道 WAV，时长误差 ≤100 ms |
| G-6 | 引擎崩溃/超时/OOM 不炸掉整集：任务失败可重试，已落盘产物保留 |

## 3. 非目标（本 CHG 明确不做）

| 不做 | 理由 |
|---|---|
| ❌ **人声分离（UVR/MDX）** | 需要 Python 运行时或纯 ONNX C++ 分离器，是独立子项目（约 350 MB 运行时决策）。交 CHG-0003。本 CHG 转写**原始音轨** |
| ❌ CUDA 后端 | 产物要带 `cublas64_12.dll`（单文件 130 MB+），CI 需装 CUDA Toolkit；先用自包含的 Vulkan 覆盖"有 GPU 就加速"。CUDA 列为后续可选资产 |
| ❌ DirectML | whisper.cpp 无成熟 DirectML 后端，不做假承诺 |
| ❌ 模型微调 / 自研推理 | 只做编排与集成 |
| ❌ 说话人分离、字级时间戳 | 上游不直接提供，另立项 |

## 4. 功能需求

### P0（本 CHG 必须全部满足）

| ID | 需求 | 说明 |
|---|---|---|
| F-01 | 自建运行时 | CI 构建 whisper.cpp 两个变体：`cpu` 与 `vulkan`（Windows x64），产物打 zip 发 release |
| F-02 | 运行时校验 | 每个 zip 有实测 sha256；registry 条目从 `tbd:true` 变为真实值 |
| F-03 | 模型下载 | 从官方 HF 源下 `ggml-*.bin`，断点续传 + sha256 + 原子落盘（复用 `internal/downloader`） |
| F-04 | ffmpeg 运行时 | 下载静态构建，用它把任意容器/编码转到 `pcm_s16le 16k mono` |
| F-05 | 引擎适配器 | 实现 `pipeline.Engine`：spawn `whisper-cli`、翻译进度、解析 JSON 输出、支持取消 |
| F-06 | 后端选择 | 探测 Vulkan 可用性 → 选 `vulkan`，否则 `cpu`；用户可锁定 |
| F-07 | 降级可见 | Vulkan 初始化失败 → 落 CPU，事件日志与 UI 都必须给原因 |
| F-08 | 真实 e2e | tiny 模型 + 30 秒语音 → 非空、单调递增的段级时间戳 |
| F-09 | 合成引擎退位 | CLI 默认走真引擎；合成引擎仅保留为测试替身（`--engine synthetic` 显式选择） |
| F-10 | 未发布即不可装 | registry 里仍为 `tbd:true` 的条目，UI 显示"未发布"且安装按钮不可用 |

### P1（结构预留，不在本 CHG 实现）

- CUDA 变体（同 M1 流程，多一个构建 job）
- 模型档位自动推荐（按显存预算选 tiny/base/small/medium/large）
- 多线程/多进度条

## 5. 交互

首次运行 `run` 时的**必需前置**必须被显式处理，而不是让用户面对一句 `exec: file not found`：

```
$ stemscribe run --input episode.mp4
缺少运行时：whisper.cpp (vulkan)
  → 安装：stemscribe runtime install whispercpp-vulkan-win-x64
缺少模型：whisper-tiny
  → 安装：stemscribe model install whisper-tiny
缺少 ffmpeg
  → 安装：stemscribe runtime install ffmpeg-win-x64
```

- 缺什么说清楚哪个、怎么装，**一次列全**（不要修一个报下一个）
- 安装动作本身也要能一条命令完成，便于 CI 与脚本
- `--engine synthetic` 仍可用，但帮助文本要写明"不读取音频，仅用于链路测试"

## 6. 验收演示脚本

1. 干净目录 → `stemscribe probe` 报出后端与显存
2. `stemscribe runtime install` / `model install` → 各自下载、校验、落盘（`.part` → rename 可观察）
3. `stemscribe run --input <30s 真实语音视频>` → 输出里出现**音频中真实说过的词**（不是"合成语音片段"）
4. `transcript.json` 段数 > 0、`start_ms` 严格递增、时间码在视频时长范围内
5. 把 Vulkan 运行时改名 → 再跑 → 自动落 CPU 且日志写明原因
6. `Ctrl+C` 中途取消 → 无残留 `whisper-cli.exe` 进程
7. 断网 → 模型已装则可继续；未装则给出"需要联网"的明确错误

## 7. 边界

| 不做 | 理由 |
|---|---|
| 自动升级运行时 | 版本切换会改变转写结果，必须由用户显式触发 |
| 从第三方网盘下模型 | 校验源必须可追溯到官方上游 |
| 运行用户自带的任意 exe | 破坏 provenance（`meta.json` 要能证明用的是哪个二进制） |
