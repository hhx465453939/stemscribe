# CHG-0011 · CHG Log

## 变更摘要

把「模型中心能看不能装」变成能装。安装逻辑从 `cmd/stemscribe` 下沉到 `internal/runctl`，
由 webapi 暴露给两个外壳（浏览器与原生窗口共用同一 handler）。

## 关键决策

### ADR-025 · 校验值缺失即拒装

registry 里 `sha256: null` + `tbd: true` 的条目一律拒绝安装，并在界面写出原因。
理由：下载一个无法校验的 3 GB 权重，比告诉用户「暂不可安装」坏得多——
前者会在几周后以「模型加载失败」的形式重现，而没人能回溯到今天的决定。

这条与项目铁律「不许编造校验值」互补：铁律禁止**填**假值，这里禁止**绕过**缺失值。

### ADR-026 · 安装状态单一入口

沿用 ADR-022：`InstallService` 属于 `internal/runctl`，CLI 与 webapi 共用。
不新增第二套进度机制；进度源是 `downloader` 的真实回调，无回调时不显示百分比。

## 影响面

- 新增 `internal/runctl/install.go`。
- `cmd/stemscribe/install.go` 改为调用 `runctl`（输出逐字不变）。
- `internal/webapi/handler.go` 增加两条 install 相关路由。
- `frontend/app/models/page.tsx` 增加安装按钮与状态；`frontend/lib/{api,i18n}.ts` 增类型与文案。

## 验证策略

1. 单测/编译：`go build/vet/test`。
2. 拒装路径：对 `uvr-vr-hp2-vocals` 点安装，必须得到明确拒绝。
3. 真装路径：装 `whispercpp-vulkan-win-x64`（15 MB，sha256 已实测），
   确认 `/api/capabilities` 的 `backend` 由 `cpu` 变 `vulkan`。
4. 前端：`typecheck`/`lint`/`i18n-check` + 安装中与安装后两张截图。
