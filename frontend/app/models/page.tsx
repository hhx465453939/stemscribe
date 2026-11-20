"use client";

import { useEffect, useState } from "react";
import { usePrefs } from "../components/Providers";
import { listCatalog, type CatalogItemView, type CatalogView } from "@/lib/api";
import type { Key } from "@/lib/i18n";

// Registry family -> dictionary key. Kept as a lookup rather than string
// concatenation so a new family fails loudly at the type level instead of rendering
// a raw key.
const kindKey: Record<string, Key> = {
  whisper: "models.kind.stt",
  separator: "models.kind.sep",
  runtime: "models.kind.runtime",
};

const tierKey: Record<string, Key> = {
  light: "models.tier.light",
  balanced: "models.tier.balanced",
  max: "models.tier.max",
};

export default function ModelsPage() {
  const { t } = usePrefs();
  const [catalog, setCatalog] = useState<CatalogView>({ installed: [], available: [] });
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let alive = true;
    listCatalog().then((next) => {
      if (alive) {
        setCatalog(next);
        setLoaded(true);
      }
    });
    return () => {
      alive = false;
    };
  }, []);

  const label = (m: CatalogItemView) => t(kindKey[m.kind] ?? "models.kind.stt");

  const rows = (items: CatalogItemView[], installed: boolean) => (
    <table className="grid">
      <thead>
        <tr>
          <th>{t("models.colName")}</th>
          <th>{t("models.colKind")}</th>
          <th className="num">{t("models.colSize")}</th>
          <th className="num">{t("models.colVram")}</th>
          <th>{t("models.colLicense")}</th>
          <th>{t("models.colState")}</th>
        </tr>
      </thead>
      <tbody>
        {items.map((m) => {
          const tierLabel = m.tier ? tierKey[m.tier] : undefined;
          return (
            <tr key={m.id}>
              <td>{m.name || m.id}</td>
              <td className="mono">{label(m)}</td>
              <td className="num">{m.size_mb > 0 ? `${m.size_mb} MB` : "—"}</td>
              <td className="num">{m.vram_mb > 0 ? `${m.vram_mb} MB` : "—"}</td>
              <td className="mono">{m.license || "—"}</td>
              <td>
                <span className={installed ? "dot dot--done" : "dot"} aria-hidden />{" "}
                {installed ? t("models.verified") : t("models.notInstalled")}
                {tierLabel !== undefined ? (
                  <span className="tag" style={{ marginLeft: "var(--sp-2)" }}>
                    {t(tierLabel)}
                  </span>
                ) : null}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );

  return (
    <>
      <h1 className="page-title">{t("models.title")}</h1>
      <p className="page-sub">{t("models.subtitle")}</p>

      {!loaded ? <p className="mono muted">{t("home.loading")}</p> : null}

      <h2 className="section-title">{t("models.installed")}</h2>
      {loaded && catalog.installed.length === 0 ? (
        <p className="mono muted">{t("models.empty")}</p>
      ) : null}
      {catalog.installed.length > 0 ? rows(catalog.installed, true) : null}

      <h2 className="section-title">{t("models.available")}</h2>
      {loaded && catalog.available.length === 0 ? (
        <p className="mono muted">{t("models.empty")}</p>
      ) : null}
      {catalog.available.length > 0 ? rows(catalog.available, false) : null}
    </>
  );
}
