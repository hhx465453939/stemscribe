"use client";

import { useEffect, useMemo, useState } from "react";
import { usePrefs } from "../components/Providers";
import generated from "@/lib/params.generated.json";
import { groupLabels, paramHelps, paramLabels, pick } from "@/lib/params.i18n";

interface Spec {
  key: string;
  flag: string;
  group: string;
  kind: "int" | "float" | "bool" | "enum" | "string";
  def: unknown;
  min: number;
  max: number;
  enum?: string[];
  unit?: string;
  help: string;
}

const data = generated as { groups: string[]; specs: Spec[] };
const STORAGE_KEY = "stemscribe.params";

type Overrides = Record<string, string>;

export default function ParamsPage() {
  const { t, lang } = usePrefs();
  const [overrides, setOverrides] = useState<Overrides>({});
  const [loaded, setLoaded] = useState(false);
  const [unavailable, setUnavailable] = useState<Record<string, string>>({});

  useEffect(() => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw) setOverrides(JSON.parse(raw) as Overrides);
    } catch {
      /* private mode or corrupt value: start from defaults */
    }
    setLoaded(true);
  }, []);

  // AC-09: disable what this machine cannot actually do, and say why. The list is
  // computed by the backend from real state (installed runtimes and models); guessing
  // it here would produce a control that looks usable and fails at run time.
  useEffect(() => {
    let alive = true;
    fetch("/api/capabilities", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((d: { unavailable_params?: Record<string, string> } | null) => {
        if (alive && d?.unavailable_params) setUnavailable(d.unavailable_params);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  const set = (key: string, value: string) => {
    setOverrides((prev) => {
      const next = { ...prev };
      if (value === "") delete next[key];
      else next[key] = value;
      try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      } catch {
        /* ignore */
      }
      return next;
    });
  };

  const effective = (s: Spec): string => overrides[s.key] ?? String(s.def);

  // What a run would actually be told. Only deviations are shown, matching the
  // engine adapter, so this preview never drifts from what happens at run time.
  const preview = useMemo(() => {
    const parts: string[] = [];
    for (const s of data.specs) {
      const raw = overrides[s.key];
      if (raw === undefined || raw === String(s.def)) continue;
      if (s.kind === "bool") {
        if (raw === "true") parts.push(s.flag);
        continue;
      }
      parts.push(`${s.flag} ${raw}`);
    }
    return parts.join(" ");
  }, [overrides]);

  const changed = Object.keys(overrides).length;

  return (
    <>
      <h1 className="page-title">{t("params.title")}</h1>
      <p className="page-sub">{t("params.subtitle")}</p>

      <div className="panel stack" style={{ marginBottom: "var(--sp-6)" }}>
        <div className="row row--between">
          <span className="mono muted">
            {changed > 0 ? t("params.changed", { n: changed }) : t("params.allDefault")}
          </span>
          <button
            type="button"
            className="btn"
            disabled={changed === 0}
            onClick={() => {
              setOverrides({});
              try {
                localStorage.removeItem(STORAGE_KEY);
              } catch {
                /* ignore */
              }
            }}
          >
            {t("params.reset")}
          </button>
        </div>
        <div className="mono" style={{ color: "var(--text-1)", wordBreak: "break-all" }}>
          {preview === "" ? t("params.nothingChanged") : preview}
        </div>
        <p className="mono muted" style={{ margin: 0 }}>
          {t("params.previewNote")}
        </p>
      </div>

      {loaded
        ? data.groups.map((group) => {
            const specs = data.specs.filter((s) => s.group === group);
            if (specs.length === 0) return null;
            return (
              <section key={group} style={{ marginBottom: "var(--sp-8)" }}>
                <h2 className="section-title" style={{ marginTop: 0 }}>
                  {pick(groupLabels[group], lang, group)}
                </h2>
                <table className="grid">
                  <tbody>
                    {specs.map((s) => (
                      <tr key={s.key}>
                        <td style={{ width: "30%" }}>
                          {pick(paramLabels[s.key], lang, s.key)}
                        </td>
                        <td style={{ width: "26%" }}>
                          {control(s, effective(s), set, unavailable[s.key] !== undefined)}
                        </td>
                        <td className="mono muted">
                          {s.flag} · {t("params.default")} {String(s.def)}
                          {s.unit ? ` ${s.unit}` : ""}
                          {unavailable[s.key] !== undefined ? (
                            <span className="tag" style={{ marginLeft: "var(--sp-2)" }}>
                              {t("params.unavailable")}
                            </span>
                          ) : overrides[s.key] !== undefined ? (
                            <span className="tag tag--manual" style={{ marginLeft: "var(--sp-2)" }}>
                              {t("params.modified")}
                            </span>
                          ) : null}
                        </td>
                        <td className="muted" style={{ width: "34%" }}>
                          {unavailable[s.key] !== undefined
                            ? unavailable[s.key]
                            : pick(paramHelps[s.key], lang, s.help)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>
            );
          })
        : null}
    </>
  );
}

function control(
  s: Spec,
  value: string,
  set: (key: string, v: string) => void,
  disabled: boolean,
): React.ReactNode {
  if (s.kind === "bool") {
    return (
      <label className="row" style={{ gap: "var(--sp-2)" }}>
        <input
          type="checkbox"
          checked={value === "true"}
          disabled={disabled}
          onChange={(e) => set(s.key, e.target.checked ? "true" : "")}
        />
        <span className="mono muted">{value === "true" ? "on" : "off"}</span>
      </label>
    );
  }
  if (s.kind === "enum" && s.enum) {
    return (
      <select
        className="input"
        value={value}
        disabled={disabled}
        onChange={(e) => set(s.key, e.target.value)}
        aria-label={s.key}
      >
        {s.enum.map((opt) => (
          <option key={opt} value={opt}>
            {opt}
          </option>
        ))}
      </select>
    );
  }
  if (s.kind === "int" || s.kind === "float") {
    return (
      <input
        className="input"
        type="number"
        value={value}
        min={s.min}
        max={s.max}
        step={s.kind === "float" ? 0.05 : 1}
        disabled={disabled}
        onChange={(e) => set(s.key, e.target.value)}
        aria-label={s.key}
      />
    );
  }
  return (
    <input
      className="input"
      type="text"
      value={value}
      disabled={disabled}
      onChange={(e) => set(s.key, e.target.value)}
      aria-label={s.key}
    />
  );
}
