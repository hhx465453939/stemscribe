# CHG-0015 · TRACEABILITY

## 1. 需求 → AC → 验证 → 落点

| 需求 | AC | 验证 | 落点 |
|---|---|---|---|
| 先抽 vocal，再转写 | AC-01 | 阶段顺序 + 文件 mtime + meta | `internal/runctl/run.go`、`internal/pipeline/pipeline.go` |
| 分离不可静默跳过 | AC-02 | 临时 workdir 复现 + 日志断言 | `internal/runctl/run.go`、`internal/engine/separator.go` |
| 默认最大模型 | AC-03 | 干净 settings 真跑 + run log | `internal/config/config.go`、`internal/runctl/run.go` |
| 出 txt | AC-04 | 文件与段数断言 | `internal/runctl/helpers.go` |
| 拖入多文件即跑 | AC-05 | 桌面手工 E2E + 队列快照 | `cmd/stemscribe-gui/main_gui.go`、`frontend/app/components/NewTask.tsx` |
| 选项生效 | AC-06 | 两次运行对比 | 前端 + `internal/webapi/handler.go` |
| 端到端 | AC-07 | 真实视频 + Checkfix | 全仓 |

### 1.1 AC 结果（2026-11-26 实测）

| AC | 结果 | 证据 |
|---|---|---|
| AC-01 强制分离 | ✅ | 整集 run 日志 `separate` 在 `transcribe` 之前且 ok；`audio/{vocals,instrumental,vocals16k}.wav` 存在；meta `provenance.engines.separate.model_id = uvr-mdx-kim-vocal-2` |
| AC-02 不许静默跳过 | ✅ | 裸 workdir 运行退出 1，提示含 `runtime install separator-python` 与 `model install uvr-mdx-kim-vocal-2`；`SEPARATE_UNAVAILABLE` 已从代码移除（`grep` 0 命中）；单测 `TestMissingHintsNameSeparatorInstall` |
| AC-03 最大模型默认 | ✅ | `dist/workdir/config/settings.json` 默认 `whisper-large-v3`；整集 meta `engines.transcribe.model_id = whisper-large-v3`，引擎 1.9.5/vulkan；降档路径单测 + `MODEL_SUBSTITUTED` 队列行翻译 |
| AC-04 TXT | ✅ | 整集 `text/transcript.txt` 273 行 = `transcript.json` 273 段；`TestToTextOneLinePerSegment` |
| AC-05 拖入即跑 | ✅（拖入回调链路）/ ⚠️（真实鼠标拖拽未自动化） | 3 个 20 s 视频经**拖入回调所调用的同一端点** `POST /api/queue` 入队 → 3 项全部 done、串行执行、产物 3 份（`tmp/evidence/ac05-queue.json`）；窗口实机启动 `Stemscribe.exe`（title=Stemscribe）不闪退；Wails `DragAndDrop{EnableFileDrop:true}` + `runtime.OnFileDrop`（v2.17.1 runtime 源码已核对）。**未做**：OS→WebView2 的真实鼠标拖放自动化，需人工 30 s 复核 |
| AC-06 选项生效 | ✅ | 同一队列用 `output_dir=tmp/queue-out`、`input_language=zh`、`max_len=30`、`split_on_word=true`；产物落点在该目录，meta `params_snapshot` 三项齐全、`params_snapshot_hash` 随变化（fnv:052f7f91b04cfcce ≠ 整集默认的 b71f053c2c25db4e） |
| AC-07 端到端 | ✅ | 29:57 带 BGM 真实视频：vocals/instrumental/vocals16k + txt/srt/vtt/md/json；Checkfix 见 §4 |

## 2. 状态

| 项 | 状态 | 证据 |
|---|---|---|
| M1 分离运行时/模型/适配 | ✅ | 30 min 整集真实跑通：`audio/{vocals,instrumental,vocals16k}.wav` 产出，事件顺序 `separate` 先于 `transcribe`；缺依赖时诊断给出 `runtime install separator-python` / `model install uvr-mdx-kim-vocal-2`（`internal/engine/separator_test.go` `TestMissingHintsNameSeparatorInstall`） |
| M2 最大模型默认 | ✅ | 干净 workdir 首跑 `engine:` 行报 `whisper.cpp 1.9.5 (vulkan)`，meta 记录 `whisper-large-v3`；OOM 降档路径有单测（`TestNextSmallerInstalledWalksTheLadder`、`TestIsAllocationFailure`） |
| M3 TXT/拖入/选项 | ✅ | `text/transcript.txt` 行数 = 段数（`TestToTextOneLinePerSegment` + 整集产物）；Wails `DragAndDrop{EnableFileDrop:true}` + `runtime.OnFileDrop` 已接；模型页可装分离运行时（`TestCatalogOffersSeparatorRuntime`） |
| M4 E2E/提交 | ✅ | 整集 E2E 见 §1.1/§3；`go build/vet/test -race`、`pnpm lint/typecheck/build`、`verify-contract`、`i18n-check`、`contrast` 全绿；已 commit/push |

## 3. 实测数据（禁止编造，未测写 TBD）

| 项 | 值 | 来源 |
|---|---|---|
| medium 实测（已有） | 29:57 → 199.13 s（RTF 0.111）；CER 粗估 7.12% | CHG-0014 关闭证据 |
| large-v3 字节/sha256 | **3,095,033,483 B**；`64d182b440b98d5203c4f9bd541544d84c605196c4f7b845dfa11fb23594d1e2`（HF ETag 与本地一致） | 本机下载后 `sha256sum` |
| 分离模型 | `uvr-mdx-kim-vocal-2`（Kim_Vocal_2.onnx，MDX-NET）；**66,759,214 B**；`ce74ef3b6a6024ce44211a07be9cf8bc6d87728cc852a68ab34eb8e58cde9c8b` | hf-mirror 下载后实测 |
| 分离耗时（30 min 素材） | **18 min 58 s**（01:02:46→01:21:44 UTC，RTF ≈0.63）；峰值 RSS ≈5.1 GB；0 显存 | 本机 CPU onnxruntime，`runs/*.jsonl` + 进程采样 |
| 分离模型运行环境 | Python venv + `audio-separator` 0.48.0（CPU onnxruntime），`uv pip install audio-separator[cpu]`，venv 磁盘 1040 MB | `runtimes/separator/INSTALLED.json` |
| large-v3 转写耗时（29:57 整集） | **287.19 s（RTF 0.160）**，273 段 | `runs/*.jsonl`（transcribe start/end） |
| 端到端（含分离）30 min 耗时 | **1428 s ≈ 23 min 48 s**（resolve→export；分离占 79.7%） | 同一 run 日志 |
| large-v3 CER（vs 同集参考字幕） | **6.42%**（Levenshtein 真 CER；hyp 7879 字 / ref 7682 字，edit 493） | `tmp/cer.py` 对本轮 `text/transcript.txt` |
| CER 方法差异 | medium 的 7.12% 是早期「等长前缀」粗估上界，与本轮 Levenshtein 不同度量，**不可直接相减**；本轮同方法结论：6.42% 处同一量级且更低 | `docs/GAP-ANALYSIS.md` §1.1 注 |

## 4. 决策记录（详见 `CHG.md`）

- 分离走 `audio-separator`（UVR5 推理栈）外置进程，不在 Go 内做张量运算。
- 分离固定开启；缺依赖失败，不跳过。
- 默认最大模型 + 可见降档，不做隐式 CPU 切换。
- 拖入即入队，保留浏览按钮；浏览器壳只支持手填路径。

## 5. 验收证据计划

1. M1：临时 workdir 删掉分离运行时/模型，断言运行失败且错误含安装指引；恢复后跑通并列出 vocals/instrumental。
2. M2：干净 settings 跑一集，断言 meta 模型 = `whisper-large-v3`；模拟 OOM（或装不上时）断言提示可见。
3. M3：窗口拖入 3 个视频，截图队列；断言产物含 txt 且行数匹配。
4. M4：真实带 BGM 视频全流程 + 全量 Checkfix，记录耗时与 CER。

## 6. 未覆盖 / 顺延

| 项 | 归属 |
|---|---|
| VAD、逐字稿查看器、取消、卸载 | CHG-0016（未立项） |
| 分离运行时/模型的分发打包 | 后续 CHG（M1 先做本地按需安装） |
| CUDA 版 whisper 运行时 | CHG-0002 遗留 |
