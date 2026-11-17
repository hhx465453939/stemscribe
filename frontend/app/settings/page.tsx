"use client";

import { useState } from "react";
import {
  demoModels,
  formatContextWindow,
  type ProviderModel,
} from "@/lib/api";
import { usePrefs } from "../components/Providers";

// Unsupported capability fields are rendered greyed-out with an explicit reason
// rather than hidden (PRD F-07). The user must be able to see what the model they
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
      <td className="muted">{note}</td>
    </tr>
  );
}

export default function SettingsPage() {
  const { t } = usePrefs();
  const [models] = useState<ProviderModel[]>(demoModels);
  const [selected, setSelected] = useState(0);
  const [baseUrl, setBaseUrl] = useState("http://127.0.0.1:11434/v1");
  const [apiKey, setApiKey] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [refreshed, setRefreshed] = useState(false);

  const model = models[selected];
  const sourceLabel = model
    ? t("settings.source", {
        s:
          model.caps.caps_source === "manual"
            ? t("settings.source.manual")
            : model.caps.caps_source === "registry"
              ? t("settings.source.registry")
              : t("settings.source.probe"),
      })
    : "";

  return (
    <>
      <h1 className="page-title">{t("settings.title")}</h1>
      <p className="page-sub">{t("settings.subtitle")}</p>

      <h2 className="section-title">{t("settings.provider")}</h2>
      <div className="panel stack">
        <div className="row">
          <label style={{ width: 96 }} className="muted">
            {t("settings.baseUrl")}
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
            {t("settings.apiKey")}
          </label>
          <input
            className="input"
            type="password"
            placeholder={t("settings.apiKeyPlaceholder")}
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            spellCheck={false}
          />
        </div>

        <div className="row row--between">
          <span className="mono muted">
            {refreshed ? t("settings.refreshed", { n: models.length }) : t("settings.hint")}
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
            {refreshing ? t("settings.refreshing") : t("settings.refresh")}
          </button>
        </div>
      </div>

      <h2 className="section-title">{t("settings.models")}</h2>
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
              {sourceLabel}
            </span>
          </div>

          <table className="grid" style={{ marginTop: "var(--sp-3)" }}>
            <tbody>
              <CapRow
                label={t("caps.context")}
                value={formatContextWindow(model.caps.context_window)}
                supported
                note={t("caps.contextTokens", { n: model.caps.context_window.toLocaleString() })}
              />
              <CapRow
                label={t("caps.maxOutput")}
                value={model.caps.max_output_tokens.toLocaleString()}
                supported
                note={t("caps.supported")}
              />
              <CapRow
                label={t("caps.reasoning")}
                value={
                  model.caps.reasoning_supported
                    ? `${model.caps.reasoning_mode ?? "effort"} · ${model.params.reasoning_effort ?? "medium"}`
                    : t("caps.unsupported")
                }
                supported={model.caps.reasoning_supported}
                note={t("caps.reasoningNote")}
              />
              <CapRow
                label={t("caps.vision")}
                value={model.caps.vision ? t("caps.supported") : t("caps.unsupported")}
                supported={model.caps.vision}
                note={t("caps.visionNote")}
              />
              <CapRow
                label={t("caps.tools")}
                value={model.caps.tools ? t("caps.supported") : t("caps.unsupported")}
                supported={model.caps.tools}
                note={t("caps.toolsNote")}
              />
              <CapRow
                label={t("caps.stream")}
                value={model.caps.streaming ? t("caps.on") : t("caps.off")}
                supported={model.caps.streaming}
                note={t("caps.supported")}
              />
              <CapRow
                label={t("caps.temperature")}
                value={String(model.params.temperature)}
                supported
                note={t("caps.temperatureNote")}
              />
              <CapRow label={t("caps.topP")} value={String(model.params.top_p)} supported note={t("caps.supported")} />
            </tbody>
          </table>

          <p className="mono muted" style={{ margin: "var(--sp-3) 0 0" }}>
            {t("caps.greyNote")}
          </p>
        </div>
      ) : null}
    </>
  );
}
