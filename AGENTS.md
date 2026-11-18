# AGENTS.md - Stemscribe

Scope: this directory and all subdirectories.
本文件与全局 `~/.agents/AGENTS.md`（Windows: `%USERPROFILE%\AGENTS.md`）**叠加生效**；冲突时以本文件的项目特化条款为准。

## Mission

把「视频 → 音频 → 文字 → 润色 → 归档」的本地链路做成一个可复现、可批量、可开源的 Windows 客户端。
不追求功能多，追求**每一步都可解释、可审计、可重跑**。

## 项目重心（判断优先级时的排序）

1. **正确性**：产物必须可信（时间戳守恒、provenance 完整）。宁可慢，不可错。
2. **默认可用**：用户第一次点「开始」就该成功，不能要求他先懂显存。
3. **可复现**：同一输入 + 同一参数 + 同一模型 → 同一产物结构。
4. **体积与分发**：安装包 ≤40 MB，模型按需下载，不随包。
5. 美观：见 `docs/UI-DESIGN.md`，是硬约束不是加分项。

## 铁律（项目级）

1. **不许把推理代码写进 Go**。Go 只编排；张量运算一律走 `internal/engine` 的外置进程（ADR-002）。
2. **不许编造校验值**。registry 的 `sha256` 未实测就写 `null` + `tbd:true`，禁止填假值。
3. **不许静默降级**。任何后端/模型/精度降级必须写进事件日志并在 UI 显性提示（ADR-004）。
4. **不许让润色失败拖垮整集**。逐字稿必须先落盘，润色是增值层（`docs/PIPELINE.md` §6）。
5. **不许把密钥写进任何文件或日志**。DPAPI 加密 + 日志层脱敏（ADR-008）。
6. **不许把模型权重提交进仓库**。`.gitignore` 已覆盖 `models/`、`runtimes/`、`libraries/`。
7. **不许在 CI 里跳过测试换绿**。测试失败就修，不加 `|| true`。
8. **不许用 WMI 查显存**。`AdapterRAM` 是 32 位字段，12 GB 卡会报 4 GB（本机已实测踩坑）。

## 依赖源与目录（硬约束）

### 模型下载源

- **模型权重只从 `https://hf-mirror.com` 取**。国内直连 `huggingface.co` 基本不可达（本会话多次实测为 `000`），
  而 `hf-mirror.com` 可用（302 → `cas-bridge.xethub.hf.co` → 200，`Content-Length` 与实测一致）。
- registry 里模型 `files[].url` **必须是绝对 URL**。写相对路径会被 mirror base 拼成
  `github.com/.../releases/download/...`，必然 404——这个坑已踩过。
- 镜像只换「下载源」，**不换校验值**。sha256 仍以官方文件的实测值为准；镜像下的字节与官方一致
  （`ggml-large-v3.bin` 与 WhisperDesktop 自带副本逐字节相同，可作交叉验证）。
- 换镜像时，先 `HEAD` 核对 `Content-Length` 与 registry 的 `size` 一致，再 `GET` 算 sha256 回填。
- **禁止编造 sha256**：未实测就写 `null` + `tbd:true`，UI 与 `install` 都必须拒绝安装。

### 引擎运行时

由本仓库 CI 自建（`.github/workflows/runtime.yml`）并作为 release 资产发布，附 `SHA256SUMS.txt`。
不使用第三方预编译包（来源不可审计）。

### 工作目录（不许污染系统路径）

- 默认工作目录 = **可执行文件同级的 `workdir/`**。模型、运行时、产物全部落在里面，整个文件夹可搬走。
- `go run` 时二进制在系统临时目录，此时回退到**当前目录的 `workdir/`**（开发时即仓库内）。
- **禁止**把 `%USERPROFILE%`、`%TEMP%`、`%APPDATA%` 当作默认落盘位置：模型单个 0.5–3 GB，
  散落到用户目录会让安装不可搬、不可备份，卸载后还会留下几个 GB。
- 测试与一次性验证可以用临时目录，但**必须显式传 `--workdir`**，不能靠默认值。
- `workdir/` 已在 `.gitignore` 中，绝不入库。

## 变更契约（CHG）流程

**任何代码改动前**必须：

1. 读 `tasks/CHG-<id>/SPEC.md` + `PRD.md` + `TRACEABILITY.md`（三件齐全、status ≠ draft）。
2. 确认目标文件在该 CHG 的独占清单内（见 `SPEC.md` §7）。
3. 能一句话回答：**改完哪条 AC 变绿？**
4. 无对应 CHG 就不动手，先立项。

**新增 CHG**：复制 `tasks/CHG-0001/` 为模板，编号递增（`CHG-0002` 起），并在 `tasks/README.md` 登记。

**关闭 CHG**：其 `SPEC.md` §6 的全部 AC 必须可验证通过，且 `CHG.md` 记录关闭时间与验证证据（命令输出/CI 链接）。

## 开发命令

```powershell
.\scripts\dev.ps1              # 开发（wails dev + next dev）
.\scripts\build.ps1            # 构建可执行
.\scripts\smoke-test.ps1       # 端到端冒烟
.\scripts\verify-contract.sh   # 校验文档/契约完整性
.\scripts\secrets-scan.ps1     # 扫描明文密钥
```

**收尾必须全绿**（Checkfix）：

```powershell
go build ./...
go vet ./...
golangci-lint run ./...
go test ./... -race
pnpm --dir frontend lint
pnpm --dir frontend build          # 必须产出 frontend/out/
```

## 代码风格

| 语言 | 规则 |
|---|---|
| Go | `gofumpt` + `golangci-lint`（启用 `errcheck`/`gosec`/`revive`/`bodyclose`）；错误必须 wrap（`fmt.Errorf("ctx: %w", err)`）；不用 `panic` 处理业务错误 |
| TS/TSX | ESLint（`next/core-web-vitals` + `@typescript-eslint`）+ Prettier；组件文件 PascalCase，hook 用 `use*` |
| CSS | 只用 `docs/UI-DESIGN.md` 的 token；禁写裸色值与裸圆角 |
| 提交 | Conventional Commits：`feat|fix|docs|refactor|test|chore|build|ci(scope): subject` |
| 注释 | 解释「为什么」，不解释「是什么」；中文注释允许，但导出符号的 godoc 用英文 |

## 测试卫生

- **T0 测试前**：检查 `tmp/`、`frontend/out/`、`__pycache__`、`testdata/*.wav` 是否有上轮残留；报告后再跑。
- **T1 测试后**：清理本轮生成的临时产物；`libraries/` 下的真实数据**绝不删**。
- **T2 三类区分**：测试产物（可删）· 运行数据（绝不删）· 模糊项（问主人）。
- **T3 代码层清扫**：`fmt.Println`/`console.log`/`debugger`/注释掉的旧实现/硬编码测试数据，随测试闭环清掉。

## 多 agent 并行

- 动手前先看 `git status -sb` + `git log --oneline -6`，识别他人在途改动。
- **绝不** `git add .` / `git add -A`；显式列文件。
- 不动他人未提交改动；不叠加内容到他人正在改的文件。
- 一个逻辑单元一个 commit；自己范围改动尽早 push。

## 开源脱敏（commit/push 前必过）

禁止进入仓库：真实公网/内网 IP、域名、端口、`C:\Users\<user>` 绝对路径、API Key/Token/私钥、云资源名与桶名、隧道服务名。

```powershell
.\scripts\redact-scan.sh      # 0 命中才能 push
git log --all -S'<敏感串>' --oneline
```

一旦历史泄漏过，必须 `git-filter-repo` 全史清除后强推——只改最新 commit 不够。

## 文档同步

| 改动类型 | 必须同步 |
|---|---|
| 架构/进程/IPC | `docs/ARCHITECTURE.md` + 对应 ADR |
| 阶段/流程 | `docs/PIPELINE.md` |
| registry 字段 | `docs/MODEL-REGISTRY.md` + `docs/schema/model-registry.schema.json` |
| provider 参数 | `docs/LLM-PROVIDERS.md` + `docs/schema/provider.schema.json` |
| 显存/内存策略 | `docs/RESOURCE-SCHEDULER.md` |
| 视觉/组件 | `docs/UI-DESIGN.md` |
| AC 状态 | `tasks/CHG-<id>/TRACEABILITY.md` |

**实测数据**必须回填 `TRACEABILITY.md` §3（GPU/速度/显存/体积），禁止编造。

## 安全与合规

- 音视频字节不出本机；仅文本可外发（需用户显式开启）。
- 不内置 API Key、不分发第三方模型权重。
- 下载物强校验 sha256；release 附 `SHA256SUMS.txt`。
- 提醒用户遵守平台条款与版权法规；不实现绕过付费/风控的功能。
