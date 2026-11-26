"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePrefs } from "../components/Providers";
import {
  getInstallStatus,
  getSettings,
  listCatalog,
  saveSettings,
  shell,
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

// Architecture names are proper nouns (MDX-Net, Demucs); only the fallback is a
// dictionary key, so a new architecture renders as itself instead of a raw key.
const archLabel: Record<string, string> = {
  mdx: "MDX-Net",
  vr: "VR",
  demucs: "Demucs",
  mdxc: "MDX23C",
  roformer: "Roformer",
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
  // The separation model the run path will actually use. Without showing it the
  // page can install Demucs and still leave the next run on Kim Vocal 2.
  const [currentSep, setCurrentSep] = useState("");
  const timer = useRef<number | null>(null);

  const reload = useCallback(async () => {
    const [next, settings] = await Promise.all([listCatalog(), getSettings()]);
    setCatalog(next);
    setCurrentSep(settings?.separator ?? "");
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

  // Persist the choice and reflect it immediately: the run path reads the setting,
  // so a failure here has to surface on the row instead of looking like success.
  const selectSeparator = useCallback(async (id: string) => {
    setError("");
    setTried(id);
    const res = await saveSettings({ separator: id });
    if (res.error) {
      setError(res.error);
      return;
    }
    setCurrentSep(id);
  }, []);

  const openDir = useCallback(async (dir: string) => {
    const bridge = shell();
    if (bridge?.OpenFolder) {
      await bridge.OpenFolder(dir);
      return;
    }
    void navigator.clipboard?.writeText(dir);
    setError("");
  }, []);

  const label = (m: CatalogItemView) => t(kindKey[m.kind] ?? "models.kind.stt");

  const rows = (items: CatalogItemView[], installed: boolean) => (
    <table className="grid">
      <thead>
        <tr>
          <th>{t("models.colName")}</th>
          <th>{t("models.colKind")}</th>
          <th>{t("models.version")}</th>
          <th className="num">{t("models.colSize")}</th>
          <th className="num">{t("models.colVram")}</th>
          <th>{t("models.colLicense")}</th>
          <th>{t("models.path")}</th>
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
              <td>
                <span className="mono">{label(m)}</span>
                {m.architecture ? (
                  <span className="mono muted"> · {archLabel[m.architecture] ?? m.architecture}</span>
                ) : null}
                {m.stems && m.stems.length > 0 ? (
                  <span className="tag" style={{ marginLeft: "var(--sp-2)" }}>
                    {t("models.stems").replace("{n}", String(m.stems.length))}
                  </span>
                ) : null}
              </td>
              <td className="mono muted">{m.version || "—"}</td>
              <td className="num">{m.size_mb > 0 ? `${m.size_mb} MB` : "—"}</td>
              <td className="num">{m.vram_mb > 0 ? `${m.vram_mb} MB` : "—"}</td>
              <td className="mono">
                {m.license || "—"}
                {m.non_commercial ? (
                  <span className="tag tag--nc" title={t("models.nonCommercialHint")} style={{ marginLeft: "var(--sp-2)" }}>
                    {t("models.nonCommercial")}
                  </span>
                ) : null}
              </td>
              <td>
                {m.path ? (
                  <span
                    className="mono muted"
                    title={m.path}
                    style={{
                      display: "inline-block",
                      maxWidth: 220,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                      verticalAlign: "bottom",
                    }}
                  >
                    {m.path}
                  </span>
                ) : (
                  <span className="muted">—</span>
                )}
              </td>
              <td>
                {installed ? (
                  <>
                    <span className="dot dot--done" aria-hidden /> {t("models.verified")}
                    {m.backend ? (
                      <span className="tag" style={{ marginLeft: "var(--sp-2)" }}>
                        {m.backend === "cuda12" ? "CUDA" : "CPU"}
                      </span>
                    ) : null}
                    {m.torch_backend ? (
                      <span className="tag" style={{ marginLeft: "var(--sp-2)" }}>
                        {m.torch_backend === "cuda"
                          ? t("models.torchCuda")
                          : m.torch_backend === "cpu"
                            ? t("models.torchCpu")
                            : t("models.torchAbsent")}
                      </span>
                    ) : null}
                    {m.kind === "separator" ? (
                      currentSep === m.id ? (
                        <span className="tag tag--manual" style={{ marginLeft: "var(--sp-2)" }}>
                          {t("models.currentSeparator")}
                        </span>
                      ) : (
                        <button
                          type="button"
                          className="btn"
                          style={{ marginLeft: "var(--sp-3)" }}
                          disabled={busy || running}
                          onClick={() => void selectSeparator(m.id)}
                        >
                          {t("models.useAsSeparator")}
                        </button>
                      )
                    ) : null}
                    {m.upgrade ? (
                      <button
                        type="button"
                        className="btn"
                        style={{ marginLeft: "var(--sp-3)" }}
                        disabled={busy || running}
                        onClick={() => void install(m.id)}
                      >
                        {busy ? `${t("models.installing")}${job && job.percent > 0 ? ` ${job.percent}%` : ""}` : t("models.upgradeCuda")}
                      </button>
                    ) : null}
                    {m.path ? (
                      <button
                        type="button"
                        className="btn"
                        style={{ marginLeft: "var(--sp-3)" }}
                        onClick={() => void openDir(m.path as string)}
                      >
                        {t("models.openDir")}
                      </button>
                    ) : null}
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
