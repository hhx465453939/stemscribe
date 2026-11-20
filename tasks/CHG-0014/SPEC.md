# CHG-0014 · Transcript Quality Baseline（阶段 A1+A2）

| 字段 | 值 |
|---|---|
| status | **approved** |
| 依据 | `docs/GAP-ANALYSIS.md` §1（实测基线）、§3（四个真卡点）、§4 阶段 A |
| 依赖 | CHG-0002（运行时）、CHG-0010（运行控制）、CHG-0012（队列）、CHG-0013（输出目录/参数生效） |
| 关联 ADR | ADR-023（进度不得伪造）、ADR-030（参数唯一来源） |

## 1. 背景（实测，不是推测）

用 `E:\VideoDownload\战锤40k 世界观 众生录\【战锤40K世界观01】….mp4`（29:57，中文）实跑：

- 引擎耗时 **104.8 s**（RTF 0.058，vulkan，whisper-tiny），163 段；
- 与 B 站 `.ai-zh.srt` 对照，字词错误严重：`统治着→通知`、`颗→科`、`飞船→非传`、`星系→新性`、`精锐→惊瑞`；
- 分段过粗：参考字幕 3 秒一行，我们 15 秒一段（三句并一）。

两个独立根因：**模型容量**（tiny 是下限）+ **`max_len` 默认 0（不限长）**。

## 2. 范围

| 步 | 内容 |
|---|---|
| A1 | 实测 `ggml-medium.bin` 的 sha256 并回填 registry；体积须与 `size` 一致，否则不写 |
| A2 | UI/队列发起运行时，默认带句级切分（`max_len≈30` 中文字 + `-sow` 按词切）；**参数页的显式值优先** |

**不做**（另立 CHG-0015）：VAD 模型接入、逐字稿查看器。

## 3. 设计

### 3.1 为什么不改 spec 默认值

CHG-0008 的 AC-02 要求 `paramSpecs` 的默认值等于 `whisper-cli --help` 的实测默认（`-ml 0`）。
所以 **spec 默认保持 0**，只在「从界面发起运行」时由前端加上面向字幕的基线值。
两者是不同层次：spec 描述引擎，UI 描述「我们的用法」。

### 3.2 显式值必须优先

前端把基线值 `{max_len:"30", split_on_word:"true"}` 与参数页的 `localStorage` 合并，
**参数页写的值覆盖基线**。否则用户在参数页改了 `max_len` 却被 UI 默认值悄悄盖掉，
就又回到「界面在说谎」。

### 3.3 模型校验

`ggml-medium.bin` 期望 `1,533,763,059` 字节（registry 已记）。
下载完成后计算 sha256；**只有字节数吻合且 sha256 算出**才写入 registry，
否则保持 `tbd` 并在 TRACEABILITY 记录实测值差异。

## 4. 契约

- `ParamsFromMap` 增加两个键：`max_len`（int）、`split_on_word`（bool）。
  未知键仍报错（不静默丢弃）。
- 前端基线：`DEFAULT_RUN_PARAMS = { max_len: "30", split_on_word: "true" }`，与用户参数合并后发送。

## 5. 验收标准

| AC | 判据 | 验证 |
|---|---|---|
| AC-01 | 用同一视频（01）重跑，输出行宽中位数 ≤ 40 个汉字（此前 15 秒/段、整段上百字） | 读 `transcript.md` 统计 |
| AC-02 | 参数页把 `max_len` 改成 0 后，跑出来回到长段（证明用户值优先、未被基线覆盖） | 两次运行对比 |
| AC-03 | `whisper-medium` 在 registry 中不再 `tbd`，`sha256` 为实测值，体积 = `1533763059` | 读 registry + 安装成功 |
| AC-04 | 双语无硬编码、`i18n-check` 通过 | `scripts/i18n-check.sh` |

## 6. 独占文件清单

| 范围 | 归属 |
|---|---|
| `internal/registry/data/registry.json`（medium 的 sha256） | CHG-0014（与 CHG-0008 交叠，登记） |
| `internal/webapi/handler.go`（ParamsFromMap 增键） | CHG-0014（与 CHG-0010/0012/0013 交叠，登记） |
| `frontend/app/components/NewTask.tsx`（基线参数合并） | CHG-0014 |
| `frontend/lib/i18n.ts`、`frontend/lib/api.ts` | CHG-0014 |
| `scripts/verify-lines.py`（AC-01/AC-02 的验证工具） | CHG-0014 |
| `docs/{GAP-ANALYSIS,PIPELINE}.md`、`tasks/README.md` | CHG-0014 |
