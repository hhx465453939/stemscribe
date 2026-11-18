# CLAUDE.md - Stemscribe

本文件是 `AGENTS.md` 的**镜像入口**，供只读取 `CLAUDE.md` 的 agent 使用。
**唯一真相源是 `AGENTS.md`**；两者冲突时以 `AGENTS.md` 为准，并请顺手修好本文件。

Scope: this directory and all subdirectories.

## Mission

把「视频 → 音频 → 文字 → 润色 → 归档」的本地链路做成一个可复现、可批量、可开源的 Windows 客户端。
不追求功能多，追求**每一步都可解释、可审计、可重跑**。

## 铁律（摘要，完整版见 AGENTS.md）

1. **不许把推理代码写进 Go**。Go 只编排；张量运算走 `internal/engine` 的外置进程（ADR-002）。
2. **不许编造校验值**。registry 的 `sha256` 未实测就写 `null` + `tbd:true`，`install` 必须拒绝安装。
3. **不许静默降级**。任何后端/模型/精度降级必须写进事件日志并在 UI 显性提示（ADR-004）。
4. **不许让润色失败拖垮整集**。逐字稿先落盘，润色是增值层。
5. **不许把密钥写进任何文件或日志**。DPAPI 加密 + 日志层脱敏（ADR-008）。
6. **不许把模型权重或 workdir 提交进仓库**。
7. **不许在 CI 里跳过测试换绿**。
8. **不许用 WMI 查显存**（`AdapterRAM` 是 32 位字段，12 GB 卡会报 4 GB）。

## 依赖源与目录（硬约束）

### 模型下载源：**只用 `https://hf-mirror.com`**

- 国内直连 `huggingface.co` 基本不可达（实测 `000`）；`hf-mirror.com` 可用
  （302 → `cas-bridge.xethub.hf.co` → 200，`Content-Length` 与实测字节数一致）。
- registry 的模型 `files[].url` **必须是绝对 URL**。相对路径会被拼到 GitHub release 下，
  必然 404——这个坑踩过。
- 镜像只换源、**不换校验值**：sha256 以官方文件实测为准。换源前先 `HEAD` 核 `Content-Length`。
- 下载慢（约 170 KB/s）属正常，**必须支持断点续传**，不要用短超时重试。

### 工作目录：**不许污染系统路径**

- 默认工作目录 = **可执行文件同级的 `workdir/`**；`go run` 时回退到当前目录的 `workdir/`。
- **禁止**把 `%USERPROFILE%` / `%TEMP%` / `%APPDATA%` 当默认落盘位置：
  模型单个 0.5–3 GB，散落到用户目录会让安装不可搬、不可备份，卸载后留几个 GB 垃圾。
- 测试可以用临时目录，但必须**显式** `--workdir`，不能依赖默认值。
- `workdir/` 已在 `.gitignore` 内。

## 结构速览

| 路径 | 职责 |
|---|---|
| `cmd/stemscribe/` | CLI：`run` / `probe` / `runtime install` / `model install` |
| `cmd/stemscribe-gui/` | 桌面客户端（Wails v2 + WebView2 原生窗口，`-tags gui`） |
| `internal/{config,registry,downloader}` | 工作目录、catalog、下载校验 |
| `internal/engine/` | 外置引擎适配（whisper.cpp、ffmpeg），JSONL/文件协议 |
| `internal/pipeline/`、`internal/store/` | 阶段 DAG、checkpoint、bundle |
| `internal/provider/`、`internal/vault/` | LLM 适配、DPAPI 密钥 |
| `frontend/` | Next.js 15 静态导出（双主题 + 中英双语） |
| `tasks/CHG-*/` | 变更契约（SPEC / PRD / TRACEABILITY / CHG），改代码前必读 |

## Checkfix（收尾必须全绿）

```powershell
go build ./... ; go vet ./... ; go test ./... -race
pnpm --dir frontend lint ; pnpm --dir frontend run typecheck ; pnpm --dir frontend build
bash scripts/verify-contract.sh ; bash scripts/i18n-check.sh ; python scripts/contrast.py
```

## 提交与脱敏

- Conventional Commits；**绝不** `git add .`，显式列文件。
- commit/push 前跑 `scripts/redact-scan.sh`（0 命中）与 `scripts/secrets-scan.ps1`。
- 禁止入库：真实 IP/域名、`C:\Users\<user>` 路径、密钥、模型权重、`workdir/`。
