# LLM Provider 层

> 对应 F-06 / F-07，决策见 ADR-005。Schema：`docs/schema/provider.schema.json`。
> 核心原则：**用户必须能看见并修改模型的每一项能力**，而不是被埋在代码里猜。

## 1. 协议与适配器

| 适配器 | 覆盖 | 端点 |
|---|---|---|
| `openai_compat`（默认） | OpenAI、Azure OpenAI(兼容模式)、DeepSeek、Kimi/Moonshot、Qwen(DashScope 兼容)、智谱、OpenRouter、one-api/new-api、vLLM、SGLang、Ollama、LM Studio、llama.cpp server | `GET {base}/models`、`POST {base}/chat/completions` |
| `anthropic` | Claude 原生 | `POST {base}/v1/messages`（`x-api-key` + `anthropic-version`） |

**为什么默认 OpenAI-compatible**：一套实现覆盖线上十余家 + 全部本地推理框架；这是唯一能同时满足「自定义本地 URL」与「线上 provider」的抽象（ADR-005）。

## 2. Provider 配置模型

```jsonc
{
  "schema": 1,
  "providers": [
    {
      "id": "local-ollama",
      "label": "本机 Ollama",
      "kind": "openai_compat",
      "base_url": "http://127.0.0.1:11434/v1",
      "has_secret": false,
      "extra_headers": {},
      "timeout_s": 300,
      "max_retries": 2,
      "enabled": true,
      "models": [
        {
          "id": "qwen2.5:14b",
          "display": "Qwen2.5 14B",
          "caps": {
            "context_window": 131072,
            "max_output_tokens": 8192,
            "vision": false,
            "tools": true,
            "reasoning": { "supported": false, "mode": null },
            "streaming": true,
            "sampling": { "temperature": true, "top_p": true, "top_k": false,
                          "seed": true, "stop": true, "penalties": false },
            "response_format": ["text"]
          },
          "caps_source": "manual",     // registry | probe | manual
          "params": {
            "temperature": 0.3, "top_p": 0.9, "max_tokens": 6000,
            "reasoning_effort": null, "thinking_budget_tokens": null,
            "stop": [], "seed": null, "response_format": "text"
          }
        }
      ]
    }
  ]
}
```

**约定**：`has_secret` 只是布尔标记，密钥本体在 `secrets.bin`（DPAPI），**永不进入此文件、永不出现在 API 返回值中**。

## 3. 能力信息三层（这是最关键的设计）

```
① known-models.json（随 app 发版的能力库）
         ↓ 命中则预填
② GET /v1/models（仅给 id，几乎从不给能力）
         ↓ 用户点「刷新」后与已存列表合并
③ 用户手改覆盖（caps_source="manual"，优先级最高，永不被 ① ② 覆盖）
```

**合并规则**
```
caps = merge(known_models[id], probe_result[id], manual_override[id])
manual_override 永远赢；其余字段按 ① → ② 顺序填充
所有未知字段用 conservative_default：
  context_window=131072, max_output_tokens=4096,
  vision=false, tools=false, reasoning.supported=false
```

**为什么要手改**：多数端点（尤其本地 vLLM/Ollama/one-api）不返回 context window；而上下文窗口恰恰是长文润色最容易翻车的参数。**用户必须能填**，这是 PRD F-07 的硬要求。

## 4. 参数面与投影

统一参数对象按适配器投影，**不支持的字段必须置灰**（UI）+ **不发送**（代码）：

| 通用参数 | OpenAI-compat 投影 | Anthropic 投影 | 不支持时 |
|---|---|---|---|
| `temperature` | 同名 | 同名 | 置灰 |
| `top_p` | 同名 | 同名 | 置灰 |
| `top_k` | 部分实现支持 | 同名 | 置灰 |
| `max_tokens` | 同名（必填上下文充足） | 同名（必填） | — |
| `stop` | `stop`（string/array） | `stop_sequences` | 置灰 |
| `seed` | 同名 | ❌ 不支持 | 置灰 + 提示 |
| `presence/frequency_penalty` | 同名 | ❌ 不支持 | 置灰 + 提示 |
| `reasoning` | `reasoning_effort: "low\|medium\|high"`（OpenAI o 系）或 `enable_thinking`+`thinking_budget`（Qwen/GLM 系） | `thinking: {type:"enabled", budget_tokens:N}` | 置灰 |
| `vision` | `content: [{type:"image_url"}]` | `content: [{type:"image"}]` | 置灰 |
| `tools` | `tools` + `tool_choice` | `tools` + `tool_choice` | 置灰 |
| `response_format` | `{type:"json_object"}` 或 `json_schema` | 无（用提示词约束 + 后置解析） | 置灰 |

> **`reasoning` 是最容易踩坑的字段**：三家语义完全不同（OpenAI 的 effort 档位 / Anthropic 的 token 预算 / 开源模型的 `enable_thinking` 开关）。适配器内部统一成 `enum{off,low,medium,high} | budget_tokens`，投影时按 provider 能力翻译；无法翻译则置灰并在悬浮说明写「该端点不支持推理深度控制」。

## 5. 刷新模型列表（F-07 的交互契约）

```
点击 [刷新] →
  POST/GET {base_url}/models（带 Authorization）
  → 200: 解析 data[].id（兼容 {models:[]} 与裸数组）
  → 合并策略：
       已存在的 model 保留其 params 与 manual caps
       新增的 model 用 known_models 预填 caps
       消失的 model 标 stale=true（不删除，避免用户配置丢失）
  → UI：列表刷新 + 每个模型显示 caps 来源徽标（内置库/探测/手动）
  → 401/403：提示「Key 无效或无权限」，不弹崩溃
  → 超时：提示「端点无响应（300s）」，并给出排查建议（本地端点是否启动）
```

**必须支持的边界**
- 本地端点无 key（Ollama）→ `has_secret=false` 时不发 `Authorization` 头。
- 自签 HTTPS → 提供「跳过证书校验」开关（默认关，开启时显红字警告）。
- 代理 → 尊重系统代理 + 可配置 per-provider 代理。

## 6. 流式解析

```
OpenAI:     data: {"choices":[{"delta":{"content":"..."}}]}\n\n   ... data: [DONE]
Anthropic:  event: content_block_delta\ndata: {"delta":{"type":"text_delta","text":"..."}}
```

实现要求：
- 逐行扫描，遇非法行记 warn 后跳过（不中断）。
- 客户端取消：`context.WithCancel` + 关闭 body；已产出文本保留。
- 超时：连接超时 30 s、整体超时按 `timeout_s`；流式期间若 60 s 无新 token 视为卡死（可配）。
- 重试：仅对「未产出任何 token」的失败重试（已产出则续传不可靠，改为降级为整体重跑本块）。

## 7. 长文分块（`LLM_CONTEXT_EXCEEDED` 的自动处理）

润色对象是转写稿，必须保住时间戳锚点，所以分块**不能按字符切**：

```
输入: [{start,end,text}, ...]
估算 tokens ≈ 字符数 * 1.8（中文）/ * 0.35（英文），取保守值
if estimate > context_window * 0.75:      # 预留输出空间
    按「段落边界 + 累计 token ≤ 窗口*0.4」切块
    每块润色时附带：上一块最后 2 句（上下文）+ 本块锚点范围
    合并时校验：锚点集合必须等于输入锚点集合
    任一锚点丢失 → 该块降级为原文并标 degraded
```

**为什么是 0.4 而不是 0.75**：润色输出通常与输入等长或更长，输入占 40% 给输出留足空间，避免输出被截断。

## 8. 提示词管理

```
prompts/
├── clean-transcript/
│   ├── 1@sha256-abc123.md     # 历史版本（不可变）
│   └── current -> 1@sha256-abc123.md
├── outline-summary/
├── term-glossary/
└── qa-prep/
```

- 内置模板随 app 发版；用户修改产出新版本（旧版本保留，可回滚）。
- 产物 `meta.json` 记录 `prompt_id` + `prompt_hash`（这是 G-5 复现的关键）。
- 变量注入用 `{{transcript}}`、`{{title}}`、`{{duration}}`，**禁止**把转写稿拼进 system prompt（防止提示词注入污染指令）。

**内置模板的硬约束（写进模板正文）**
```
1) 只做语言层规整：补标点、分段、去口癖；禁止增删事实、禁止改写观点。
2) 每一段输出必须以 [HH:MM:SS] 开头，锚点必须来自输入，不得新造。
3) 无法确定的地方保留原文并标注 [?]，禁止猜测。
4) 输出语言与输入一致（除明确要求翻译）。
```

## 9. 错误码与降级

| 码 | 含义 | 行为 |
|---|---|---|
| `LLM_AUTH` | 401/403 | 不重试；跳过润色，保留逐字稿；提示检查 Key |
| `LLM_NOT_FOUND` | 404 模型不存在 | 不重试；提示刷新模型列表 |
| `LLM_RATE_LIMIT` | 429 | 按 `Retry-After` 等待重试（最多 2 次） |
| `LLM_TIMEOUT` | 超时 | 重试 2 次（指数退避） |
| `LLM_CONTEXT_EXCEEDED` | 上下文超限 | 自动分块（§7） |
| `LLM_BAD_FORMAT` | 返回非法 JSON/结构 | 重试 1 次（提高约束提示）；再失败则本块降级原文 |
| `LLM_REFUSED` | 模型拒答 | 不重试；标 `skipped`；逐字稿仍可用 |
| `LLM_NETWORK` | 连接失败 | 重试 2 次 |

**铁律**：润色是增值层，任何 `LLM_*` 失败都不得导致整集失败（见 `PIPELINE.md` §6）。

## 10. 安全与隐私

| 项 | 措施 |
|---|---|
| 外发内容 | 仅发送**文本**（转写稿 + 提示词）；音视频永不出本机（PRD N-05） |
| 首次外发提示 | 第一次启用润色时显式弹窗：「本步骤会把转写文本发送到 <host>」，需勾选确认 |
| 密钥 | DPAPI 加密；日志层过滤 `sk-*`/`Bearer *`；崩溃报告不含 headers |
| 自定义 header | 允许，但不允许覆盖 `Authorization`（避免混淆密钥来源） |
| 自签证书 | 默认校验证书；跳过需显式开关 + 红字警告 |
| 提示词注入 | 转写稿作为 `user` 消息的**数据块**传入，system 指令不含用户内容；输出后置校验锚点 |

## 11. 内置能力库（`known-models.json`）维护规则

- 只收录**有官方文档可查**的能力字段，每条附 `source_url` 与 `verified_at`。
- 每季度复核一次；无法复核的条目降级为「未验证」，字段清空（回退 conservative default）。
- 不收录价格（变动太快且与功能无关）。

```jsonc
{
  "gpt-4.1": { "context_window": 1047576, "max_output_tokens": 32768,
               "vision": true, "tools": true,
               "reasoning": {"supported": false},
               "source_url": "https://platform.openai.com/docs/models", "verified_at": "2026-11-01" }
}
```

> `context_window: 1047576` 就是主人说的「一般是 1M」的落地位；UI 显示为 `1M`，手改可填任意值。

## 12. 验收（AC-11 的可执行定义）

```go
// internal/provider/openai_compat_test.go
srv := httptest.NewServer(mockHandler)     // 提供 /v1/models 与 /v1/chat/completions(SSE)
p := NewProvider(Config{BaseURL: srv.URL + "/v1", Model: "mock-1",
                        Params: Params{Temperature: 0.3, MaxTokens: 4096}})

models, err := p.RefreshModels(ctx)        // 断言 len>0 且 id 匹配
require.NoError(t, err)

out, err := p.Polish(ctx, transcript, "clean-transcript")
require.NoError(t, err)
assert.Contains(t, mockLastRequest.Temperature, 0.3)      // 参数透传
assert.Equal(t, 4096, mockLastRequest.MaxTokens)
assert.Equal(t, []string{"00:00:03","00:00:07"}, anchorsOf(out))  // 锚点守恒
```
