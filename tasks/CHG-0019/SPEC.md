# CHG-0019 · SPEC — 阶段 1 的模型/产物选择与保留集裁剪

| 项 | 值 |
|---|---|
| status | **approved**（owner 2026-11-27 明确要求；实现已随本 CHG 提交） |
| 影响面 | 前端新建任务页、`/api/queue`、runctl 选项、pipeline 分离阶段、CLI 旗标 |
| 关联 ADR | ADR-019（registry 是能力唯一真相源）、ADR-004（不许静默降级） |

## 1. 数据流

```
前端 NewTask
  ├─ 处理方式下拉 ← 已安装 separator 的 architecture 去重（demucs → mdx → vr → …）
  ├─ 分离模型下拉 ← catalog.installed.filter(kind == "separator" 且 architecture 匹配)
  ├─ 音轨多选     ← 所选模型 registry.stems；vocals 锁定；全选/仅人声快捷
  └─ POST /api/queue { separator, stems, model, ... }
        │
        ▼
webapi.Handler → runctl.Options{SeparatorID, ExportStems}
        │  normalizeStems：去重排序；必须含 vocals；未知 stem 拒绝并列可选值
        ▼
pipeline.Runner（separate 阶段）
  Engine.Separate() → 全部音轨
  requireStems()    → registry 声明的轨必须齐
  pruneStems()      → 非保留轨 os.Remove，emit stems{kept,dropped}
        │
        ▼
runctl：dropStaleAudio() → 清上一条配置遗留的 *.wav（保护派生文件）
        ▼
meta.Provenance.Engines["separate"].stems ← os.Stat 过滤后的真实保留集
```

## 2. 关键设计

### 2.1 默认值语义

`ExportStems` 为空 = 全部保留（模型声明集合）。非空 = 交集裁剪，
且必须包含 `vocals`（转写读 `vocals16k.wav`）。空集合与「不裁剪」不能混为一谈，
由 `normalizeStems` 在入队前区分。

### 2.2 resume 哈希

`separationHash(modelID, tuning, keep)` 的 `keep` 参与哈希（空记为 `all`，非空排序后拼接）。
因此改勾选、换模型、改 shifts/overlap/batch/fp16 都会让分离重跑，
不会出现「界面显示 A、磁盘还是 B」的假一致。

### 2.3 旧模型残留清理（2026-11-27 端到端发现）

真实运行暴露：bundle 复用目录时，上一次 UVR5 写的 `instrumental.wav`
不在本次 Demucs 运行的 `sepStems` 里，`pruneStems` 覆盖不到，文件留在
`audio/` 与 UI 承诺「未勾选音轨删除」矛盾。

修复：分离阶段成功后调用 `dropStaleAudio`，规则如下——

- 只在分离启用时执行（`separate == true`）。分离被 resume 判为「already valid」
  也会执行：此时 live 集是确定的，多出来的 wav 按定义不属于当前配置。
- live 集 = 保留集；保留集为空时 = 模型声明集合。
- live 集无法确定（未知模型且保留集为空）→ **不删任何文件**，避免「保底全删」。
- 只扫 `audio/` 下的一层 `*.wav`；`full.wav` / `16k.wav` / `vocals16k.wav` 属派生文件，永不删。
- 删除项通过 `stems` 事件记录 `removed stale=`，可审计。

### 2.4 前端可读性

`vocals` 是「恒保留」的事实，不是控件。用 `span.stem-lock`（琥珀实心 + 勾）
而不是 `input[disabled][checked]`——后者在暗色主题下被浏览器画成灰空框，
读起来正好与语义相反（见 `M1-STAGE1-UI.md`）。

## 3. 验收判据（能红）

| # | 判据 | 失败条件 |
|---|---|---|
| AC-01 | 新建任务页有「处理方式」下拉（按 architecture 去重，Demucs 显式在列）与族内「分离模型」下拉，选项来自 registry 已安装 separator | 页面上没有处理方式控件，或 Demucs 不在处理方式里，或选项写死只有 UVR5 |
| AC-02 | 选项文本含架构 / 轨数 / 非商用标记 | 用户无法区分 MDX 与 Demucs |
| AC-03 | 「输出音轨」多选随所选模型变化 | 切换模型后仍显示上一个模型的轨 |
| AC-04 | vocals 恒保留且视觉为已勾选、不可取消 | 可取消 vocals，或暗色下显示为未勾选 |
| AC-05 | 未勾选轨在分离后被删除，事件记录 `kept=/dropped=` | 文件仍在，或日志无记录 |
| AC-06 | 改保留集/换模型后 resume 重跑分离 | 日志显示 separate skipped 且旧产物被复用 |
| AC-07 | meta 只记录真实存在且被保留的轨 | meta 声称的 stem 在磁盘不存在 |
| AC-08 | 换模型后旧模型遗留 stem 被清理；派生文件不受影响 | 残留 `instrumental.wav` 等；或误删 `full/16k/vocals16k` |
| AC-09 | 未知 stem 入队前被拒绝并列出可选值 | 运行到一半才失败 |
| AC-10 | 阶段 2 标签与阶段 1 区分（转写模型 / 分离模型） | 两个下拉都叫「模型」 |
| AC-11 | zh/en 文案齐平，类型检查强制 | 缺键仍能构建 |
| AC-12 | 真实运行 + 双主题截图作为证据 | 只有单测，没有端到端证据 |

## 4. 已知边界

- 已存在的旧 bundle 只有在**下一次以该输入路径运行且分离启用**时才会被清理；
  本 CHG 不做启动期全库扫描（扫描用户输出目录需单独的授权与取舍）。
- `dropStaleAudio` 只删 wav；其他扩展名的残留不在本 CHG 范围。

## 5. 修订记录

| 日期 | 修订 | 原因 |
|---|---|---|
| 2026-11-27 | §1 数据流与 AC-01 由「单一分离模型下拉」改为「处理方式 → 分离模型」两段（外加音轨多选），对齐 UVR5 的 PROCESS METHOD → MODEL → STEM(S) 结构 | owner 验收 dist 时指出看不到 Demucs：旧 exe 未重建是直接原因，扁平下拉把模型族差异藏住是设计原因；同时补「全选 / 仅人声」快捷与双主题截图证据 |
