# CHG-0011 · Provisioning（从界面安装模型与运行时）

| 字段 | 值 |
|---|---|
| status | **approved** |
| owner | 本会话（owner 已授权全自动开发/测试/提交/推送） |
| 依赖 | CHG-0002（引擎与运行时，已闭环）、CHG-0010（webapi/runctl 骨架） |
| 关联 ADR | ADR-004（不许静默降级）、ADR-013（运行时由本仓库 CI 自建）、ADR-022（单一 handler）、ADR-025（校验值缺失即拒装） |

## 1. 背景与问题

模型中心已经能**列出**该装什么（`GET /api/models`，数据来自 registry + 磁盘实存），
但**没有任何安装动作**。用户看到「Whisper Large v3 · 未安装」却点不了，等于一条死路。

安装能力本身是完好的——`stemscribe model install` / `runtime install` 已在 CLI 可用，
`internal/downloader` 已实现分片下载、sha256 校验、原子改名。缺的只是：
**这段逻辑锁在 `cmd/stemscribe` 包内，webapi 调用不到**（与 CHG-0010 抽 `runctl` 完全同因）。

## 2. 范围与不做

**做**

- 把安装逻辑下沉到 `internal/runctl`（`Install(workdir, id)`），CLI 改为调用它（行为不变）。
- `POST /api/models/{id}/install` 启动安装，立即返回；`GET /api/models/install` 返回当前安装状态。
- 模型中心页：未安装项出现「安装」按钮，点击后显示进度，完成后自动刷新目录。
- **校验值缺失必须拒装**（registry `sha256:null` + `tbd:true`），并把原因显示在界面。

**不做**

- 不做暂停/续传（downloader 已有分片，够用）。
- 不做卸载（另一个动作，另立）。
- 不做并发安装（一次一个，状态机简单可审计）。

## 3. 设计

沿用 CHG-0010 的形状，避免第二套机制：

```
POST /api/models/{id}/install ──► InstallService.Start ──► goroutine
                                        │                      │
GET /api/models/install ◄──────────────┴── Job{state,percent,message}
```

- `Job.state` ∈ `idle` | `running` | `done` | `failed`。
- 进度由 `downloader` 的进度回调驱动；无回调时只显示「已开始 / 已完成」，**不伪造百分比**。
- 安装成功后 `CatalogNow` 自然把该项从 `available` 移到 `installed`（磁盘实存判定，无需额外记账）。

## 4. 契约

```json
POST /api/models/{id}/install   → 202 {"id":"i-1"}
GET  /api/models/install        → 200 {"id":"i-1","target":"whispercpp-vulkan-win-x64","state":"running","percent":42,"message":""}
                            或 200 {"state":"idle"}
```

失败时 `message` 必须是人能行动的一句话，例如
`refusing to install uvr-vr-hp2-vocals: sha256 is not measured yet (tbd)`。

## 5. 里程碑

| 里程碑 | 内容 | 文件 | 变绿 AC |
|---|---|---|---|
| M1 | 安装逻辑下沉 `runctl.Install`，CLI 改调用 | `internal/runctl/install.go`、`cmd/stemscribe/install.go` | AC-05 |
| M2 | `InstallService` + 状态 | `internal/runctl/install.go` | AC-02/AC-03 |
| M3 | webapi 端点 | `internal/webapi/handler.go` | AC-01/AC-03 |
| M4 | 模型中心安装按钮 + 进度 | `frontend/app/models/page.tsx`、`frontend/lib/{api,i18n}.ts` | AC-01/AC-04 |
| M5 | 真装一个运行时并回填证据 | `docs/images/app-models-install.png` | AC-06 |

**DoD**：AC-01…AC-06 全绿；`go build/vet/test` + `typecheck`/`lint`/`i18n-check` 全绿；
真装 `whispercpp-vulkan-win-x64`（14,997,989 B，sha256 已实测）成功，且 `/api/capabilities`
的 `backend` 从 `cpu` 变为 `vulkan`——这是「安装真的生效」的硬证据。

## 6. 验收标准

| AC | 判据 | 验证方式 |
|---|---|---|
| AC-01 | 未安装项有「安装」按钮；点击后 ≤500ms 出现安装中状态 | 截图 + 手工 |
| AC-02 | 安装成功后该项出现在「已安装」区（来源为磁盘实存，非本地记账） | 装完刷新页面 |
| AC-03 | `sha256` 为 `null`+`tbd` 的条目**拒装**并给出原因 | 对 `uvr-vr-hp2-vocals` 点安装 |
| AC-04 | 双语、双主题、无硬编码 CJK | `i18n-check.sh` + 截图 |
| AC-05 | CLI `model install` / `runtime install` 行为不变 | 逐字比对输出 |
| AC-06 | 装完 vulkan 运行时后 `capabilities.backend` 由 cpu 变 vulkan | `curl /api/capabilities` 前后对比 |

## 7. 风险与对策

| 风险 | 对策 |
|---|---|
| 抽取改变 CLI 行为 | 以现有 CLI 输出为基线比对；`go test ./...` + 真装一次验证 |
| 大模型安装阻塞 webview | 安装跑在 goroutine，HTTP 立即返回；前端轮询 |
| 界面报假进度 | 仅有回调时显示百分比，否则只显示状态词（ADR-023 同源） |
| 装到一半失败留下脏文件 | downloader 已用 `.part` + 原子 rename，失败不留半成品 |

## 8. 依赖与外部条件

- 网络：模型/运行时来自 `hf-mirror.com` 与 GitHub release（既有 registry 绝对 URL）。
- 已实测可用的验证目标：`whispercpp-vulkan-win-x64`（15 MB，sha256 已实测）。
- 无新增第三方依赖。

## 9. 独占文件清单

| 范围 | 归属 |
|---|---|
| `internal/runctl/install.go`（新建） | CHG-0011 |
| `cmd/stemscribe/install.go`（改为调用 runctl） | CHG-0011 |
| `internal/webapi/handler.go`（仅新增 install 路由） | CHG-0011（与 CHG-0010 交叠，登记） |
| `frontend/app/models/page.tsx`、`frontend/lib/{api,i18n}.ts` | CHG-0011（与 CHG-0007/0010 交叠，登记） |
| `docs/{MODEL-REGISTRY,UI-DESIGN}.md`、`tasks/README.md` | CHG-0011 |
| `internal/downloader/**`、`internal/registry/**` | **只读引用，不改**（如需改动另立子项） |
