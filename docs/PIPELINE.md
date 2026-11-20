# 流水线（Pipeline）

> 对应需求 F-01…F-14，状态机定义见 `ARCHITECTURE.md` §8。本文定义**阶段语义、DAG、幂等、事件、失败处理**。

## 1. 阶段总览

| # | 阶段 | 输入 | 输出 | 依赖 | 可跳过 | 预计耗时（30 min 音频 / 3060） |
|---|---|---|---|---|---|---|
| 1 | `resolve` | URL 或文件路径 | `SourceMeta{video_id,title,duration,parts[]}` | — | 本地文件直接填 | < 2 s |
| 2 | `download` | SourceMeta | `source/video.mp4`（或本地引用） | resolve | 本地文件 | 视网速 |
| 3 | `probe` | video | `{duration_ms, streams, sample_rate}` | download | — | < 1 s |
| 4 | `demux` | video | `audio/full.wav`(44.1k st) + `audio/16k.wav`(16k mono) | probe | — | ~20 s |
| 5 | `separate` | full.wav | `audio/vocals.wav` + `audio/instrumental.wav` | demux | ✅ 可关（直接转写 full） | ~2–4 min |
| 6 | `transcribe` | vocals.wav 或 16k.wav | `text/transcript.json` + `.srt`/`.vtt`/`.md` | separate（或 demux） | — | ~4–6 min |
| 7 | `align` | transcript + 段落边界 | 修正后的 transcript（句子合并、低置信度标记） | transcribe | ✅ 无分离时跳过交叉段修复 | < 2 s |
| 8 | `polish` | transcript.md | `text/polished.md` + `text/outline.md` | align | ✅ 默认关 | 视 provider |
| 9 | `export` | 以上全部 | 见 `MODEL-REGISTRY.md` §bundle、`meta.json` 定稿 | 全部 | — | < 1 s |

> **时间预算的理由**：RTX 3060 上 whisper.cpp `medium-q5_0` 的 RTF 约 0.15–0.25（待 CHG-0002 实测回填 `TRACEABILITY.md` §3）。PRD N-04 要求 30 min 音频 ≤6 min 转写，即 RTF ≤0.2，属可达区间。

## 2. DAG

```
resolve ──► download ──► probe ──► demux ──┬─► separate ─┐
                                            └─────────────┴─► transcribe ──► align ──► polish ──► export
```

**并行点**（v0.2）：`demux` 后可同时产出两路（分离 + 16k 直转），UI 层让用户选"质量优先"（走分离）或"速度优先"（跳过分离）。

**批量的并行度**：任务级并发由 `resource` 预算决定，默认 `max_concurrent = floor(预算 / 单任务峰值)`，且硬上限 `min(2, NumGPU)`。单 GPU 上转写与分离**不同时跑**（避免争显存），排队串行。

## 2.1 运行控制面（CHG-0010）

阶段由 `internal/runctl` 驱动，CLI 与界面**共用这一条路径**（ADR-024）：

```
runctl.Service.Start(opts) ──► goroutine ──► Run() ──► pipeline.Runner ──► engine
        │                                            │
        └── Status 快照（id/state/stage/percent）◄───┴── emit(Event)
```

**进度不是估算的**（ADR-023）：分子 = 已 `stage.end` 或已被 checkpoint 判定有效的 `stage.skip`；
分母 = 本次**启用**的阶段数（`separate` 关闭、`polish` 恒关时不计）。
因此进度条可能**回跳前先停**，但绝不会跑在工作前面。

`stage.skip` 有两种语义，必须区分：

| Note | 含义 | 计入完成数 |
|---|---|---|
| `already valid` | checkpoint 已证明产物有效，属于**已完成的活** | ✅ |
| `disabled by settings` | 本次不启用，**不在分母里** | ❌ |

> 混同这两者曾让进度报出 `9/7`。计数与分母必须用同一条规则。

事件同时写入 `<bundle>/runs/<runID>.jsonl`，因此界面、CLI 与事后审计看到的是同一份记录。

## 3. 幂等与 checkpoint

### 3.1 checkpoint 文件

`.state.json`（每阶段结束原子写，`.tmp` → rename）：

```jsonc
{
  "schema": 1,
  "video_id": "BV1xx411c7mD",
  "run_id": "01J8Z...",
  "completed": ["resolve","download","probe","demux","separate","transcribe"],
  "current": "align",
  "fingerprints": {
    "source/video.mp4":   {"size": 214748364, "mtime": 1731840000},
    "audio/full.wav":     {"size": 317520044, "mtime": 1731840120},
    "audio/vocals.wav":   {"size": 317520044, "mtime": 1731840400},
    "text/transcript.json":{"size": 88213,   "mtime": 1731840900}
  },
  "engine": {"transcribe": {"name":"whisper.cpp","version":"1.7.4","backend":"cuda12","model":"ggml-medium-q5_0.bin","model_sha12":"a1b2c3d4e5f6"}},
  "params_hash": "sha256:...",       // 参数快照的 hash，参数变了则相关阶段失效
  "updated_at": "2026-11-17T22:10:00Z"
}
```

### 3.2 续跑判定

```
for stage in DAG:
    if stage not in completed: resume_from = stage; break
    if fingerprint_changed(stage.outputs): completed 截断到此 stage 之前; resume_from = stage; break
    if params_hash_changed(stage): completed 截断; resume_from = stage; break
```

- **参数变更即失效**：用户把 `medium` 改成 `large-v3`，`params_hash` 变化 → `transcribe` 及其后阶段重跑，但 `demux`/`separate` 保留（它们的参数没变）。这靠"每阶段独立 params hash"实现，不是全局 hash。
- **fingerprint 变更即失效**：用户手动删了 `vocals.wav` → `separate` 重跑。
- 断言（AC-15 变体）：kill -9 后重启，日志中不得出现已完成阶段的重复执行记录。

## 4. 分段与对齐（质量关键）

### 4.1 分离分段

- 段长默认 15 s，重叠 2 s，边缘线性交叉淡化（crossfade）。
- 段长与显存联动：`resource` 预算不足时自动降到 8 s（并在 UI 提示 `分段 8s（显存自适应）`）。
- 输出拼接后做**响度归一化**（`-14 LUFS`，可配 `-23` 广播标准），避免不同段音量跳变。

### 4.2 转写切片

- 用 VAD（whisper.cpp 内置 `--vad`）切成语音片段，避免静音段空转（30 min 音频通常可省 20–40% 时间）。
- 切片边界对齐到 30 ms 网格，保证与 srt 时间轴一致。
- 段级结果即时落盘（`transcript.partial.jsonl`），崩溃可从最后一段续。

### 4.3 对齐修复（`align` 阶段）

三个必须处理的问题：

| 问题 | 处理 |
|---|---|
| **段边界切断句子**（跨切片半句） | 若末段不以句末标点结尾且下段长度 < 12 字 → 合并；**合并后整段不得超过 40 字**（`MaxMergedRunes`），否则另起一段；合并后重算 `start/end` |
| **幻觉重复**（whisper 在静音处重复上一句） | 连续 ≥3 段文本相似度 > 0.9 且间隔 < 1 s → 保留首段，其余标 `dropped_hallucination` |
| **低置信度** | `confidence < 0.45` → 标 `low_conf:true`（UI 黄色高亮，不删除） |

> 这三条是本地转写最常见的翻车点，不处理会让"档案"不可信。

## 5. 事件协议

写入 `runs/<run_id>.jsonl`，每行一条，**不可修改只可追加**：

```jsonc
{"ts":"2026-11-17T22:00:01.120Z","ev":"stage.start","stage":"download","params":{"url":"...","quality":"1080p"}}
{"ts":"...","ev":"stage.progress","stage":"download","pct":37,"bytes":81234567,"total":219876543,"speed_bps":4200000}
{"ts":"...","ev":"stage.end","stage":"download","ok":true,"elapsed_ms":52400,"outputs":["source/video.mp4"]}
{"ts":"...","ev":"engine.spawn","engine":"whisper.cpp","pid":12044,"backend":"cuda12","model":"ggml-medium-q5_0.bin"}
{"ts":"...","ev":"stage.warn","stage":"align","code":"HALLUCINATION_DROPPED","count":3}
{"ts":"...","ev":"stage.retry","stage":"transcribe","attempt":2,"reason":"ENGINE_OOM","action":"downgrade:small"}
{"ts":"...","ev":"task.end","ok":false,"code":"DL_403","hint":"视频可能已删除或需要登录"}
```

**用途**：① 崩溃排查 ② provenance 审计 ③ 用户可"导出运行日志"提 issue（自动脱敏）。

## 6. 失败处理矩阵

| 错误码 | 根因 | 自动动作 | 用户可见提示 |
|---|---|---|---|
| `DL_403` / `DL_404` | 视频删除 / 会员专享 / 风控 | 不重试，标失败 | 「该视频无法下载（可能已删除或需登录）」 |
| `DL_NET` | 网络中断 | 退避重试 3 次 | 「网络不稳定，已重试 3 次」 |
| `FS_DISK_FULL` | 磁盘满 | 不重试，暂停队列 | 「磁盘剩余 1.2 GB，预计需要 3.4 GB」 |
| `ENGINE_OOM` | 显存不足 | **降档一次**再重试（large→medium→small→cpu） | 「显存不足，已自动降档到 small」 |
| `ENGINE_CRASH` | 引擎崩溃 | 重启一次；再崩则失败 | 「转写引擎异常退出，已保存进度」 |
| `ENGINE_BAD_OUTPUT` | 协议行非法 | 跳过该行继续；>10 行则失败 | 「引擎输出异常」 |
| `LLM_AUTH` | key 无效 | 不重试，跳过润色继续导出 | 「润色未完成（API Key 无效），逐字稿已保存」 |
| `LLM_TIMEOUT` | 超时 | 退避重试 2 次（流式可续） | 「润色超时，已保留未润色稿」 |
| `LLM_CONTEXT_EXCEEDED` | 超上下文 | **自动分块**（按锚点均分，逐块润色后合并） | 「文本超出上下文，已分 3 块处理」 |
| `CFG_INVALID` | 配置非法 | 不执行 | 「工作目录不可写」 |

**铁律**：润色失败**不得**导致整集失败——逐字稿必须先落盘，润色是增值层。

## 7. 批量队列语义

- 队列持久化在 SQLite（`tasks` 表），重启恢复。
- 排序：`priority DESC, created_at ASC`；用户可拖动改序。
- 单条失败不阻塞后续（`continue_on_error=true` 默认）。
- 批量导入：粘贴 UP 主空间 → 展示选集列表（标题/时长/是否已存在）→ 全选/区间选。
- 已存在的 `video_id` 默认**跳过**（幂等），可选"强制重跑"。

## 8. 端到端验收（AC-15 的可执行定义）

```powershell
# 合成 30s 测试音频（含语音与静音交替），不依赖网络
ffmpeg -f lavfi -i "sine=frequency=440:duration=2" -f lavfi -i "sine=frequency=880:duration=2" \
       -filter_complex "[0][1]concat=n=2:v=0:a=1,apad=pad_dur=26" -ar 16000 -ac 1 tests/fixtures/tone30.wav

stemscribe-cli run --input tests/fixtures/tone30.wav --skip separate --no-polish --workdir .\tmp\e2e
# 断言
#  1) tmp/e2e/libraries/<id>/meta.json 存在且 stages 全 ok
#  2) text/transcript.json 是数组、length>0、start 严格递增
#  3) text/transcript.srt 首行形如 "1" 且时间行匹配 /^\d{2}:\d{2}:\d{2},\d{3} --> /
#  4) runs/*.jsonl 含 task.end ok=true
```

> 纯音调无法验证转写文本质量（whisper 会输出空或幻觉），因此 CI 中的 AC-10 只断言**结构与单调性**，文本质量由人工在真实素材上验收。这是诚实的边界，不假装"CI 全绿=效果可用"。
