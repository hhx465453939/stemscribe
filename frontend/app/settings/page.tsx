"use client";

import { useState } from "react";
import { demoModels, formatContextWindow, type ProviderModel } from "@/lib/api";

// Unsupported capability fields are rendered greyed-out with an explicit reason
// rather than hidden (PRD §5). The user must be able to see what the model they
// paid for actually supports.
function CapRow({
  label,
  value,
  supported,
  note,
}: {
  label: string;
  value: string;
  supported: boolean;
  note?: string;
}) {
  return (
    <tr className={supported ? undefined : "disabled"}>
      <td style={{ width: "34%" }}>{label}</td>
      <td className="mono" style={{ width: "28%" }}>
        {value}
      </td>
      <td className="muted">{supported ? note ?? "支持" : note ?? "该端点不支持"}</td>
    </tr>
  );
}

export default function SettingsPage() {
  const [models] = useState<ProviderModel[]>(demoModels);
  const [selected, setSelected] = useState(0);
  const [baseUrl, setBaseUrl] = useState("http://127.0.0.1:11434/v1");
  const [apiKey, setApiKey] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [refreshed, setRefreshed] = useState(false);

  const model = models[selected];

  return (
    <>
      <h1 className="page-title">设置</h1>
      <p className="page-sub">端点与模型能力 · 密钥经 DPAPI 加密，不落明文</p>

      <h2 className="section-title">Provider</h2>
      <div className="panel stack">
        <div className="row">
          <label style={{ width: 96 }} className="muted">
            Base URL
          </label>
          <input
            className="input"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
            spellCheck={false}
          />
        </div>

        <div className="row">
          <label style={{ width: 96 }} className="muted">
            API Key
          </label>
          <input
            className="input"
            type="password"
            placeholder="本地端点可留空"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            spellCheck={false}
          />
        </div>

        <div className="row row--between">
          <span className="mono muted">
            {refreshed ? `已刷新 ${models.length} 个模型` : "填写后可刷新模型列表"}
          </span>
          <button
            className="btn"
            type="button"
            onClick={() => {
              setRefreshing(true);
              // Real implementation calls App.RefreshModels(providerID).
              window.setTimeout(() => {
                setRefreshing(false);
                setRefreshed(true);
              }, 400);
            }}
            disabled={refreshing}
          >
            {refreshing ? "刷新中…" : "刷新模型"}
          </button>
        </div>
      </div>

      <h2 className="section-title">模型</h2>
      <div className="row" style={{ flexWrap: "wrap", gap: "var(--sp-2)" }}>
        {models.map((m, i) => (
          <button
            key={m.id}
            type="button"
            className={i === selected ? "btn btn--primary" : "btn"}
            onClick={() => setSelected(i)}
          >
            {m.id}
          </button>
        ))}
      </div>

      {model ? (
        <div className="panel" style={{ marginTop: "var(--sp-3)" }}>
          <div className="row row--between">
            <strong className="mono">{model.id}</strong>
            <span className={model.caps.caps_source === "manual" ? "tag tag--manual" : "tag"}>
              能力来源：{model.caps.caps_source === "manual" ? "手动覆盖" : model.caps.caps_source === "registry" ? "内置库" : "探测"}
            </span>
          </div>

          <table className="grid" style={{ marginTop: "var(--sp-3)" }}>
            <tbody>
              <CapRow
                label="上下文窗口"
                value={formatContextWindow(model.caps.context_window)}
                supported
                note={`${model.caps.context_window.toLocaleString()} tokens`}
              />
              <CapRow
                label="最大输出"
                value={model.caps.max_output_tokens.toLocaleString()}
                supported
              />
              <CapRow
                label="推理深度"
                value={
                  model.caps.reasoning_supported
                    ? `${model.caps.reasoning_mode ?? "effort"} · ${model.params.reasoning_effort ?? "medium"}`
                    : "不支持"
                }
                supported={model.caps.reasoning_supported}
                note="OpenAI 为 effort 档位，Anthropic 为 token 预算"
              />
              <CapRow
                label="图像输入"
                value={model.caps.vision ? "支持" : "不支持"}
                supported={model.caps.vision}
                note="v0.1 未使用，但需可见以免误判"
              />
              <CapRow
                label="Function Call"
                value={model.caps.tools ? "支持" : "不支持"}
                supported={model.caps.tools}
                note="为后续 MCP 阶段预留"
              />
              <CapRow label="流式" value={model.caps.streaming ? "开启" : "关闭"} supported />
              <CapRow label="温度" value={String(model.params.temperature)} supported note="润色默认 0.3，避免编造" />
              <CapRow label="Top P" value={String(model.params.top_p)} supported />
            </tbody>
          </table>

          <p className="mono muted" style={{ margin: "var(--sp-3) 0 0" }}>
            灰色字段表示该端点不具备此能力，不会隐藏，也不会静默忽略。
          </p>
        </div>
      ) : null}
    </>
  );
}
