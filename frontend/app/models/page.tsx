"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePrefs } from "../components/Providers";
import {
  getInstallStatus,
  listCatalog,
  startInstall,
  type CatalogItemView,
  type CatalogView,
  type InstallJobView,
} from "@/lib/api";
import type { Key } from "@/lib/i18n";

// Registry family -> dictionary key. A lookup rather than string concatenation so a
// new family fails at the type level instead of rendering a raw key.
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
  const [job, setJob] = useState<InstallJobView | null>(null);
  const [error, setError] = useState("");
  // Which row the user just clicked. The install job is global and single-flight, so a
  // failure has to be attributed to a row explicitly; otherwise a refusal lands in a
  // banner above the tables and the user sees "nothing happened".
  const [tried, setTried] = useState("");
  const timer = useRef<number | null>(null);

  const reload = useCallback(async () => {
    const next = await listCatalog();
    setCatalog(next);
    setLoaded(true);
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const running = job?.state === "running";

  // Poll only while an install is in flight. "Installed" is decided by the backend
  // re-reading the disk, so the board cannot drift from reality.
  useEffect(() => {
    if (!running) {
      if (timer.current !== null) window.clearInterval(timer.current);
      timer.current = null;
      return;
    }
    timer.current = window.setInterval(() => {
      void getInstallStatus().then((next) => {
        if (!next) return;
        setJob(next);
        if (next.state === "done") void reload();
        if (next.state === "failed") setError(next.message);
      });
    }, 500);
    return () => {
      if (timer.current !== null) window.clearInterval(timer.current);
      timer.current = null;
    };
  }, [running, reload]);

  const install = useCallback(async (id: string) => {
    setError("");
    setTried(id);
    const res = await startInstall(id);
    if (res.error) {
      setError(res.error);
      return;
    }
    const st = await getInstallStatus();
    if (st) setJob(st);
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
          const busy = running && job?.target === m.id;
          // Attribute a refusal to this row: async failure via job.target, immediate
          // failure (bad id, busy) via the row we just clicked.
          const rowMsg =
            job?.state === "failed" && job.target === m.id
              ? job.message
              : tried === m.id
                ? error
                : "";
          return (
            <tr key={m.id}>
              <td>{m.name || m.id}</td>
              <td className="mono">{label(m)}</td>
              <td className="num">{m.size_mb > 0 ? `${m.size_mb} MB` : "—"}</td>
              <td className="num">{m.vram_mb > 0 ? `${m.vram_mb} MB` : "—"}</td>
              <td className="mono">{m.license || "—"}</td>
              <td>
                {installed ? (
                  <>
                    <span className="dot dot--done" aria-hidden /> {t("models.verified")}
                  </>
                ) : (
                  <>
                    <span className="dot" aria-hidden /> {t("models.notInstalled")}
                    <button
                      type="button"
                      className="btn"
                      style={{ marginLeft: "var(--sp-3)" }}
                      disabled={busy || running}
                      onClick={() => void install(m.id)}
                    >
                      {busy
                        ? `${t("models.installing")}${job && job.percent > 0 ? ` ${job.percent}%` : ""}`
                        : t("models.install")}
                    </button>
                  </>
                )}
                {tierLabel !== undefined ? (
                  <span className="tag" style={{ marginLeft: "var(--sp-2)" }}>
                    {t(tierLabel)}
                  </span>
                ) : null}
                {rowMsg !== "" ? (
                  <span
                    className="mono"
                    role="alert"
                    style={{ display: "block", marginTop: "var(--sp-2)", color: "var(--text-1)" }}
                  >
                    {rowMsg}
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

      {/* Refusals are shown verbatim: "sha256 not measured" is an answer, and hiding it
          behind a generic failure would send the user hunting for a network problem. */}
      {error !== "" ? (
        <p className="mono" style={{ margin: 0, color: "var(--text-1)" }} role="alert">
          {t("models.installFailed")}: {error}
        </p>
      ) : null}

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
