# TRACEABILITY · CHG-0001

> 目的：让任何一个 agent 或人类，能在 30 秒内回答三件事——
> **这条需求由哪个 AC 兜住？这个 AC 怎么验？这个文件归谁？**
> 无对应关系的条目必须标 `TBD`，禁止留空冒充已完成。

---

## 1. 需求 → 判据 → 验证 → 归属

| 需求（PRD） | SPEC 目标 | 验收判据 | 验证方式（可执行） | 交付文件 / 模块 | CHG |
|---|---|---|---|---|---|
| F-01 视频获取 | G-1 | AC-15 | `scripts/smoke-test.ps1 -Full` | `internal/pipeline/source/` | CHG-0003 |
| F-02 音轨提取 | G-1 | AC-15 | 同上（断言 full.wav / 16k.wav 存在且 ffprobe 时长一致 ±100ms） | `internal/pipeline/audio/` | CHG-0003 |
| F-03 人声分离 | G-3 | AC-10、AC-15 | `go test ./internal/pipeline -run E2E` | `internal/engine/separator/` | CHG-0002/0003 |
| F-04 语音转写 | G-1、G-3 | AC-10 | 同上（断言段数 > 0、start 严格递增） | `internal/engine/transcriber/` | CHG-0002/0003 |
| F-05 字幕导出 | G-1 | AC-15 | 断言 `.srt`/`.vtt`/`.md` 三件存在且时间格式 `HH:MM:SS,mmm` | `internal/store/export/` | CHG-0003 |
| F-06 LLM 润色 | G-4 | AC-11、AC-15 | `go test ./internal/provider` + e2e | `internal/provider/`、`prompts/` | CHG-0004 |
| F-07 Provider 配置 | G-4 | AC-11 | mock provider：刷新列表 + 全参数透传断言 | `internal/provider/`、`frontend/app/settings/` | CHG-0004/0005 |
| F-08 模型与运行时管理 | G-2 | AC-04、AC-09 | schema 校验 + release 附件检查 | `internal/registry/`、`internal/downloader/` | CHG-0002 |
| F-09 工作目录 | G-2 | AC-15 | e2e 断言 workdir 自包含（拷贝到临时目录后仍可解析 registry） | `internal/config/` | CHG-0002 |
| F-10 任务看板 | G-1 | AC-07、AC-15 | 冒烟启动 + e2e 产出 | `frontend/app/` | CHG-0005 |
| F-11 产物浏览 | G-5 | AC-15 | e2e 打开详情页断言无异常（组件级） | `frontend/app/library/` | CHG-0005 |
| F-12 Provenance | G-5 | AC-15 | 断言 `meta.json` 含引擎版本/模型 sha/参数快照/提示词 hash | `internal/store/` | CHG-0003 |
| F-13 离线可用 | G-2 | AC-15 | e2e 在 `--offline` 模式下跑（禁网 stub） | `internal/pipeline/` | CHG-0003 |
| F-14 崩溃可恢复 | G-1 | AC-13（间接）+ 手工验证 | 强制 kill 后重启续跑，断言已完成阶段不重跑 | `internal/store/checkpoint/` | CHG-0003 |
| F-20/F-21/F-22 MCP 与检索 | — | **TBD（CHG-0006）** | 未立项 | `internal/mcpserver/` | CHG-0006 |
| N-02 安装包 ≤40 MB | G-6 | AC-09 | release 产物 `ls -lh` 断言 | `.github/workflows/release.yml` | CHG-0001 B |
| N-05 隐私 | G-6 | AC-14 | 脱敏扫描 + 网络 egress 白名单审计 | 全仓库 | CHG-0001 B |
| — 架构契约 | G-6 | AC-03 | grep 8 个 ADR 四段式 | `docs/DECISIONS.md` | CHG-0001 A |
| — 可构建骨架 | G-6 | AC-05/06/07 | CI job `go` / `web` / `windows` | `cmd/`、`frontend/`、`build/` | CHG-0001 B |
| — CI/CD | G-6 | AC-08、AC-09 | 故意注入错误验证变红；tag 验证产物 | `.github/workflows/` | CHG-0001 B |
| — 密钥安全 | G-6 | AC-13 | DPAPI 单测 + 落盘扫描 | `internal/vault/` | CHG-0004 |

## 2. 交付物 → 判据 → 状态

| # | 交付物 | 判据 | 状态 |
|---|---|---|---|
| 1 | `tasks/CHG-0001/SPEC.md` | AC-02 | ✅ 已交付 |
| 2 | `tasks/CHG-0001/PRD.md` | AC-02 | ✅ 已交付 |
| 3 | `tasks/CHG-0001/TRACEABILITY.md`（本文件） | AC-02 | ✅ 已交付 |
| 4 | `tasks/CHG-0001/CHG.md` | AC-02 | ✅ 已交付 |
| 5 | `docs/DECISIONS.md`（8 ADR） | AC-03 | ⏳ 待交付（本轮同批） |
| 6 | `docs/ARCHITECTURE.md` | AC-01 | ⏳ 待交付（本轮同批） |
| 7 | `docs/PIPELINE.md` | AC-01 | ⏳ 待交付（本轮同批） |
| 8 | `docs/MODEL-REGISTRY.md` + schema | AC-04 | ⏳ 待交付（本轮同批） |
| 9 | `docs/RESOURCE-SCHEDULER.md` | AC-01 | ⏳ 待交付（本轮同批） |
| 10 | `docs/LLM-PROVIDERS.md` + schema | AC-01/AC-04 | ⏳ 待交付（本轮同批） |
| 11 | `docs/UI-DESIGN.md` | AC-01 | ⏳ 待交付（本轮同批） |
| 12 | `docs/UPSTREAM-RESEARCH.md` | AC-01 | ⏳ 待交付（本轮同批） |
| 13 | `docs/ROADMAP.md` + `tasks/README.md` | AC-01 | ⏳ 待交付（本轮同批） |
| 14 | `README*.md`、`AGENTS.md`、`LICENSE`、`.gitignore`、`CONTRIBUTING.md`、`SECURITY.md`、`CHANGELOG.md` | AC-01 | ⏳ 待交付（本轮同批） |
| 15 | Go/Wails/Next 骨架 | AC-05/06/07 | ⬜ CHG-0001 B 阶段 |
| 16 | `.github/workflows/{ci,release}.yml` | AC-08/09 | ⬜ CHG-0001 B 阶段 |
| 17 | `scripts/{dev,smoke-test,verify-contract,secrets-scan,redact-scan}` | AC-01/08/13/14 | ⬜ CHG-0001 B 阶段 |

## 3. 实测数据登记（禁止编造，只登记真跑出来的）

| 项 | 值 | 来源 | 日期 |
|---|---|---|---|
| GPU | NVIDIA GeForce RTX 3060, 12288 MiB | `nvidia-smi` | 2026-11-17 |
| 驱动 / CUDA | 566.36 / 12.7 | `nvidia-smi` | 2026-11-17 |
| 参考工具：Buzz 引擎栈 | `faster_whisper-1.0.3` + `ctranslate2` + `torch-2.2.1+cu121` + `onnxruntime-1.17.1` + `whisper.dll` | 目录枚举 `E:\software\Buzz\_internal` | 2026-11-17 |
| 参考工具：UVR 引擎栈 | `torch` + `onnxruntime` + `models/{MDX_Net_Models,VR_Models,Demucs_Models}` + `lib_v5` | 目录枚举 `F:\software\Ultimate Vocal Remover` | 2026-11-17 |
| 参考工具：WhisperDesktop 权重 | `ggml-large-v3.bin` = 3,095,033,483 B；`lz4.txt` 存在（压缩权重缓存） | `ls -la` | 2026-11-17 |
| 参考工具：下载器 | `bilibili-video-downloader.exe` + 自带 `ffmpeg.exe`（Tauri 生态，`com.lanyeeee.*` bundle id） | 快捷方式解析 + 目录枚举 | 2026-11-17 |
| `large-v3` 显存占用（3060 12G，`-ngl 99`） | **TBD — CHG-0002 实测填入** | — | — |
| `medium q5_0` 转写速度（RTF） | **TBD — CHG-0002 实测填入** | — | — |
| MDX-Net ONNX 单段（15 s）耗时 | **TBD — CHG-0002 实测填入** | — | — |
| 安装包实测体积 | **TBD — CHG-0001 B 实测填入** | — | — |

## 4. CHG 拆分与依赖

```
CHG-0001 (本) 骨架 + 契约
   ├── CHG-0002 引擎与运行时（whisper.cpp 选型基准、audio-separator、registry+downloader）
   │      └── CHG-0003 流水线（DAG、checkpoint、store、导出）
   │             └── CHG-0005 界面（依赖 0003 的 IPC 契约）
   ├── CHG-0004 LLM 层（provider + vault + prompts）— 可与 0002/0003 并行
   └── CHG-0006 调度强化 + MCP server（依赖 0003）
```

**并行规则**：0002 与 0004 可完全并行（文件不相交）；0005 必须等 0003 的 IPC 契约冻结（`docs/ARCHITECTURE.md` §IPC 为准）。

## 5. 已识别但未立项的缺口（诚实登记）

| 缺口 | 影响 | 计划 |
|---|---|---|
| B 站下载接口的合规与稳定性 | 环节 1 可能随时断 | CHG-0003 内做双引擎（内置解析 + yt-dlp 兼容层） |
| UVR 模型权重许可 | 不能镜像分发 | registry 只存官方直链 + 许可字段；UI 展示许可 |
| `large-v3` 在 12 GB 上是否可全量卸载到显存 | 影响默认档位 | CHG-0002 实测后回填第 3 节 |
| Windows 代码签名证书 | SmartScreen 警告 | 开源版明示「未签名」，提供 `SHA256SUMS` 自校验 |
| 前端 static export 与 Wails 资源路径 | 可能导致白屏 | CHG-0001 B 首日验证，失败则降级 Vite |
