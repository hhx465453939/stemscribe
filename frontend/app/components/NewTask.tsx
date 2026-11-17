"use client";

import { useState } from "react";
import { usePrefs } from "./Providers";

export function NewTask() {
  const { t } = usePrefs();
  const [value, setValue] = useState("");
  const [separate, setSeparate] = useState(true);
  const [polish, setPolish] = useState(false);

  return (
    <div className="panel stack">
      <div className="row">
        <input
          className="input"
          placeholder={t("home.placeholder")}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          aria-label={t("home.placeholder")}
        />
        <button className="btn btn--primary" type="button" disabled={value.trim() === ""}>
          {t("home.start")}
        </button>
      </div>

      {/* Defaults must let most users press Start immediately; advanced knobs stay folded. */}
      <div className="row" style={{ gap: "var(--sp-6)", flexWrap: "wrap" }}>
        <label className="row" style={{ gap: "var(--sp-2)" }}>
          <input
            type="checkbox"
            checked={separate}
            onChange={(e) => setSeparate(e.target.checked)}
          />
          <span>{t("home.separate")}</span>
        </label>

        <label className="row" style={{ gap: "var(--sp-2)" }}>
          <input type="checkbox" checked={polish} onChange={(e) => setPolish(e.target.checked)} />
          <span>{t("home.polish")}</span>
          <span className="tag">{t("home.polishTag")}</span>
        </label>

        <span className="mono muted">{t("home.meta")}</span>
      </div>

      {polish ? (
        <p className="mono muted" style={{ margin: 0 }}>
          {t("home.polishNote")}
        </p>
      ) : null}
    </div>
  );
}
