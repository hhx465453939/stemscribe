# CHG-0011 · TRACEABILITY

## 1. 需求 → AC → 验证

| 需求 | 来源 | AC | 验证 | 落点 | 里程碑 |
|---|---|---|---|---|---|
| F-01 可点安装 | U-1 | AC-01 | 截图 + 手工 | `frontend/app/models/page.tsx` | M4 |
| F-02 完成判定来自磁盘 | U-3 | AC-02 | 装完刷新，`CatalogNow` 重新分类 | `internal/runctl/catalog.go`（只读） | M2 |
| F-03 校验缺失拒装 | U-4 | AC-03 | 对 `uvr-vr-hp2-vocals` 点安装 | `internal/runctl/install.go` | M2 |
| F-04 双语双主题 | — | AC-04 | `i18n-check.sh` + 截图 | `frontend/lib/i18n.ts` | M4 |
| F-05 CLI 不退化 | — | AC-05 | CLI 输出逐字比对 | `cmd/stemscribe/install.go` | M1 |
| F-06 安装真的生效 | U-5 | AC-06 | `curl /api/capabilities` 前后对比 | `internal/runctl/**`、`internal/resource`（只读） | M5 |

## 2. 里程碑状态

| 里程碑 | 状态 | 证据 |
|---|---|---|
| M1 下沉 `runctl.Install` | ⬜ | — |
| M2 `InstallService` + 状态 | ⬜ | — |
| M3 webapi 端点 | ⬜ | — |
| M4 前端安装按钮 | ⬜ | — |
| M5 真装 vulkan 并回填 | ⬜ | — |

## 3. 实测数据回填（禁止编造）

| 项 | 值 | 来源 |
|---|---|---|
| 验证目标体积 | 14,997,989 B（`whispercpp-vulkan-win-x64`） | registry `archive.size`，本会话早已实测 sha256 |
| 安装耗时 | 待测 | 本机真装一次 |
| 装后 backend | 待测（预期 `cpu` → `vulkan`） | `curl /api/capabilities` |

## 4. 缺陷记录

| # | 缺陷 | 根因 | 状态 |
|---|---|---|---|
| 1 | 模型中心「未安装」无任何动作 | 安装逻辑锁在 `cmd` 包，webapi 调不到 | 本 CHG 修 |

## 5. 未覆盖

- 卸载、并发安装、断点续传：明确不做（见 SPEC §2）。
- Demucs 四轨条目：仍因无可验证直链被 CHG-0008 挂起，与本 CHG 无关。
