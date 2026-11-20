# CHG-0014 · CHG Log

## 变更摘要

让「中文长视频 → 可当字幕读的逐字稿」成立：换可用模型（medium）+ 界面发起运行时默认做句级切分。

## 关键决策

### ADR-031 · spec 默认与 UI 基线分开

`paramSpecs` 的默认值必须等于 `whisper-cli --help` 的实测默认（CHG-0008 AC-02），
所以 `max_len` 的 spec 默认仍是 `0`。面向字幕的基线值（`max_len=30`、`split_on_word=true`）
只在**从界面发起运行**时由前端合并，且**用户显式值优先**。

这样既保住「参数表忠实于引擎」，又让默认行为符合我们的实际用途。两层混淆会两头都错。

### ADR-032 · 基线值必须可见

前端合并基线后会把这些键一并提交，产物 `params_snapshot` 因此能看到
`max_len=30`、`split_on_word=true`——用户不会遇到「不知道它替我改了什么」。

## 影响面

- `internal/webapi/handler.go`：`ParamsFromMap` 增加 `max_len`、`split_on_word`。
- `frontend/app/components/NewTask.tsx`：基线与用户参数合并。
- `internal/registry/data/registry.json`：medium 的 sha256 实测回填。

## 验证策略

1. 同集视频跑两次（`max_len=30` vs `0`），统计行宽 → AC-01/AC-02。
2. 下载完成后算 sha256，比对体积 → AC-03。
3. `go test` + `typecheck` + `lint` + `i18n-check` → AC-04。
