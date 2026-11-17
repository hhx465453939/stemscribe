# 贡献指南

感谢愿意参与。这个项目对"能跑"的标准比较严，请先读这一页再动手。

## 先读什么（按顺序）

1. [`README.md`](README.md) — 项目在解决什么问题
2. [`AGENTS.md`](AGENTS.md) — 项目铁律与 Checkfix 命令（人类同样适用）
3. [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — 分层与边界
4. [`tasks/CHG-0001/SPEC.md`](tasks/CHG-0001/SPEC.md) — 立项契约与验收判据
5. [`docs/ROADMAP.md`](docs/ROADMAP.md) — 当前在哪个 CHG

## 开发环境

| 依赖 | 版本 | 说明 |
|---|---|---|
| Go | ≥ 1.23 | 后端与桌面壳逻辑 |
| Node | ≥ 20 | 前端构建 |
| pnpm | ≥ 9 | 包管理（不要用 npm/yarn 提交 lockfile） |
| MSYS2 + GCC | 最新 | **Wails v2 在 Windows 上必需**（cgo 编译 WebView2 loader） |
| WebView2 Runtime | 系统自带或[下载](https://developer.microsoft.com/microsoft-edge/webview2/) | 运行必需 |

```powershell
# 一次性准备
winget install GoLang.Go OpenJS.NodeJS msys2.msys2
# MSYS2 内安装：pacman -S mingw-w64-x86_64-gcc

pnpm install
pnpm --dir frontend install
.\scripts\dev.ps1
```

## 提交流程

1. **先看有没有对应 CHG**。没有就先开一个（复制 `tasks/CHG-0001/` 结构），在 `tasks/README.md` 登记编号。
2. 只改该 CHG 独占清单里的文件；跨界的在 `CHG.md` 写清理由。
3. 提交粒度：一个逻辑单元一个 commit，遵循 Conventional Commits：
   ```
   feat(engine): add whisper.cpp supervisor with job-object binding
   fix(registry): reject dest paths containing ..
   docs(pipeline): clarify hallucination-drop threshold
   ```
4. PR 必须包含：
   - 关联的 CHG 编号
   - **改完哪条 AC 变绿**（贴命令或 CI 链接）
   - 实测数据（如涉及性能/显存，回填 `TRACEABILITY.md` §3）
   - 截图（如涉及界面，按 `docs/UI-DESIGN.md` 反模式清单自查）

## Checkfix（PR 会被拒的情况）

```powershell
go build ./... && go vet ./... && golangci-lint run ./...
go test ./... -race
pnpm --dir frontend lint && pnpm --dir frontend build
```

- 任一失败 → 拒
- 新增 `|| true` / skip 测试 / 注释掉断言 → 拒
- 硬编码裸色值或裸圆角（违反设计 token）→ 拒
- 提交模型权重、密钥、真实 IP/域名/本机路径 → 拒（见下）

## 脱敏（提交前必过）

```bash
./scripts/redact-scan.sh      # 必须 0 命中
```

禁止出现：公网/内网 IP、域名、端口、`C:\Users\<name>` 路径、API Key、Token、私钥、桶名、隧道服务名。
如历史已泄漏，需 `git-filter-repo` 全史清理后强推——只改最新 commit 不够。

## 不接受的功能请求

| 请求 | 原因 |
|---|---|
| 云端转写 / 上传媒体 | 违背本地优先（PRD N-05） |
| 内置 API Key / 代付额度 | 成本与合规不可控 |
| 绕过平台付费或风控 | 法律风险 |
| 视频剪辑能力 | 不是编辑器 |
| 重新分发第三方模型权重 | 许可风险（`docs/MODEL-REGISTRY.md` §6） |

## 报告问题

请附：
- 版本号（设置页可复制）
- `logs/` 下对应任务的 `runs/*.jsonl`（已自动脱敏）
- 硬件：GPU 型号 + 显存 + 驱动版本（设置页「复制诊断信息」）
- 复现步骤：输入类型（链接/本地）、是否开启分离/润色、所选模型

## 许可

贡献即表示同意以 [MIT](LICENSE) 许可发布。
