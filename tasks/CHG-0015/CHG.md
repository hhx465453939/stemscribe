# CHG-0015 · CHG Log

## 变更摘要

按 owner 2026-11-26 指令重写本 CHG：把“拖视频 → 抽 vocal → 转写 → txt”做成唯一主链路，
分离必选、默认最大模型、拖入即跑。

> v1 草案（首跑诚实 + VAD + 查看器）**从未批准**，已被本节 v2 完全替换。
> v1 中的 VAD/查看器等条目移出本 CHG，登记为 CHG-0016（未立项）。

## 状态流

| 时间 | 状态 | 动作 | 执行者 |
|---|---|---|---|
| 2026-11-26 | draft | v1 草案（未批准） | agent |
| 2026-11-26 | **approved** | owner 指令：按 v2 计划执行 | owner |
| 2026-11-26 | in_progress | M1 开工 | agent |
| 2026-11-26 | in_progress | M1/M2/M3 完成（整集真实跑通：分离→large-v3→txt，CER 6.42%） | agent |
| 2026-11-26 | closed | AC-01…AC-07 有证据（AC-05 真实鼠标拖拽留人工复核），Checkfix 全绿，commit/push | agent |

## 关键决策

### ADR-037 · 分离必须是链路的前置阶段，不是开关

现状是“设置里开着、实现是空的、运行时告警跳过”。这比不做更坏：用户以为抽了人声，
实际拿原声转写。v2 去掉双重开关与跳过分支，缺依赖就失败并给出安装指引。

### ADR-038 · 分离推理用 UVR5 推理栈的外置进程

Go 不碰张量（ADR-002）。用 `audio-separator`（UVR5 的 Python 推理栈）作为外置引擎，
模型仍走 registry + hf-mirror + 实测 sha256。运行时按需安装，不塞进 40 MB 安装包。

### ADR-039 · 默认最大模型 + 可见降档

默认 `whisper-large-v3`。显存不足时降一档并写 `MODEL_SUBSTITUTED`，界面可见；
不做“按机器猜模型”的隐式矩阵——规则越简单越可审计。

### ADR-040 · 拖入即入队

拖入不是“填空”，是命令。`OnFileDrop` 收到路径后直接 `POST /api/queue`，
用户不需要再点开始。队列沿用后端串行 worker。

## 影响面

- `internal/engine`：新增 `separator.go` + 内嵌 Python helper；`locate` 增加分离运行时/模型解析。
- `internal/runctl`：强制分离；large-v3 默认；可见降档；txt 导出。
- `internal/registry`：默认分离模型 + large-v3 实测条目。
- `cmd/stemscribe-gui`：`OnFileDrop` + 事件。
- `frontend`：拖入监听、自动入队、选项、队列状态。
- `docs`：PIPELINE / MODEL-REGISTRY / RESOURCE-SCHEDULER 同步。

## 验证策略

1. M1：缺依赖失败（AC-02）→ 装好后 vocals/instrumental 生成（AC-01）。
2. M2：干净 settings 默认 large-v3（AC-03）；降档事件可见。
3. M3：拖入 3 个文件截图；txt 行数断言（AC-04/05/06）。
4. M4：真实带 BGM 视频 E2E + 全量 Checkfix（AC-07）。
5. 全部通过后按 owner 指令 commit/push（显式列文件、跑脱敏与密钥扫描）。

## 执行顺序（严格按此，不插入计划外工作）

1. M1：分离运行时 + 模型 + 适配 + 强制分离
2. M2：large-v3 实测 + 默认 + 降档
3. M3：txt + 拖入 + 选项
4. M4：E2E + Checkfix + commit/push
