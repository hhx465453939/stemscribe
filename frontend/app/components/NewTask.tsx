"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePrefs } from "./Providers";
import {
  enqueue,
  getQueue,
  getSettings,
  listCatalog,
  removeQueued,
  saveSettings,
  shell,
  type CatalogItemView,
  type QueueItemView,
} from "@/lib/api";

const PARAMS_KEY = "stemscribe.params";
const POLL_MS = 500;

// Subtitle-friendly baseline for runs started from the UI.
//
// The engine's own default is max_len=0 (no limit), which merged whole sentences into
// 15-second paragraphs that cannot be used as subtitles. The spec default stays 0
// because it must match `whisper-cli --help` (CHG-0008 AC-02); this baseline belongs
// to our usage, not to the engine. An explicit value on the parameter page wins.
const RUN_BASELINE: Record<string, string> = {
  max_len: "30",
  split_on_word: "true",
};

/** Reads the parameter page's storage and layers it over the run baseline. */
function readParams(): Record<string, string> {
  let user: Record<string, string> = {};
  if (typeof window !== "undefined") {
    try {
      const raw = localStorage.getItem(PARAMS_KEY);
      if (raw) user = JSON.parse(raw) as Record<string, string>;
    } catch {
      user = {};
    }
  }
  return { ...RUN_BASELINE, ...user };
}

export function NewTask() {
  const { t } = usePrefs();
  const [paths, setPaths] = useState("");
  const [outputDir, setOutputDir] = useState("");
  const [model, setModel] = useState("");
  const [installed, setInstalled] = useState<CatalogItemView[]>([]);
  const [items, setItems] = useState<QueueItemView[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const timer = useRef<number | null>(null);

  const active = items.some((i) => i.state === "running" || i.state === "waiting");

  const refresh = useCallback(async () => {
    const q = await getQueue();
    if (q) setItems(q.items);
  }, []);

  useEffect(() => {
    void refresh();
    void getSettings().then((s) => {
      if (s) {
        setOutputDir(s.output_dir ?? "");
        setModel(s.model ?? "");
      }
    });
    void listCatalog().then((c) => {
      // Only installed models are selectable: offering an uninstalled one would fail
      // at run time, which is the trap the audit called out.
      setInstalled(c.installed.filter((m) => m.kind === "whisper"));
    });
  }, [refresh]);

  useEffect(() => {
    if (!active) {
      if (timer.current !== null) window.clearInterval(timer.current);
      timer.current = null;
      return;
    }
    timer.current = window.setInterval(() => void refresh(), POLL_MS);
    return () => {
      if (timer.current !== null) window.clearInterval(timer.current);
      timer.current = null;
    };
  }, [active, refresh]);

  const pick = useCallback(async () => {
    const bridge = shell();
    if (!bridge?.PickFiles) {
      setNotice(t("task.pickHint"));
      return;
    }
    const files = await bridge.PickFiles();
    if (files && files.length > 0) {
      setPaths((prev) =>
        prev.trim() === "" ? files.join("\n") : `${prev.trim()}\n${files.join("\n")}`,
      );
      setNotice("");
    }
  }, [t]);

  const start = useCallback(async () => {
    const inputs = paths
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
    if (inputs.length === 0 || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      // Parameters come from the parameter page; sending {} here was why every knob
      // looked adjustable and changed nothing.
      const res = await enqueue(inputs, { outputDir, model, params: readParams() });
      if (res.error) {
        setError(res.error);
        return;
      }
      if (res.rejected && res.rejected.length > 0) {
        setNotice(`${t("task.paths")}: ${res.rejected.join(", ")}`);
      }
      setPaths("");
      await refresh();
    } finally {
      setBusy(false);
    }
  }, [paths, busy, outputDir, model, refresh, t]);

  const persist = useCallback(async () => {
    const res = await saveSettings({ output_dir: outputDir, model });
    if (res.error) setError(res.error);
    else setNotice(t("task.saved"));
  }, [outputDir, model, t]);

  const openFolder = useCallback(async (dir: string) => {
    const bridge = shell();
    if (bridge?.OpenFolder) {
      await bridge.OpenFolder(dir);
      return;
    }
    // Browser fallback: the path is still useful, just not clickable.
    void navigator.clipboard?.writeText(dir);
    setNotice(dir);
  }, []);

  return (
    <>
      <div className="panel stack">
        <label className="stack" style={{ gap: "var(--sp-2)" }}>
          <span className="mono muted">{t("task.paths")}</span>
          <textarea
            className="input mono"
            rows={3}
            value={paths}
            onChange={(e) => setPaths(e.target.value)}
            placeholder={"C:\\videos\\a.mp4\nC:\\videos\\b.mkv"}
            spellCheck={false}
          />
        </label>

        <div className="row row--between" style={{ flexWrap: "wrap" }}>
          <button type="button" className="btn" onClick={() => void pick()}>
            {t("task.pick")}
          </button>
          <button
            type="button"
            className="btn btn--primary"
            disabled={paths.trim() === "" || busy}
            onClick={() => void start()}
          >
            {t("task.queue")}
          </button>
        </div>

        <div className="row" style={{ gap: "var(--sp-6)", flexWrap: "wrap" }}>
          <label className="row" style={{ gap: "var(--sp-2)", flex: "1 1 320px", minWidth: 0 }}>
            <span className="mono muted" style={{ whiteSpace: "nowrap", flexShrink: 0 }}>
              {t("task.outputDir")}
            </span>
            <input
              className="input"
              value={outputDir}
              onChange={(e) => setOutputDir(e.target.value)}
              placeholder="D:\\transcripts"
              spellCheck={false}
            />
          </label>

          <label className="row" style={{ gap: "var(--sp-2)" }}>
            <span className="mono muted" style={{ whiteSpace: "nowrap", flexShrink: 0 }}>
              {t("task.model")}
            </span>
            <select
              className="input"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              aria-label={t("task.model")}
            >
              <option value="">—</option>
              {installed.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name || m.id}
                </option>
              ))}
              {/* The configured model may not be installed. Showing a blank control made
                  it look broken; naming it explains why the run will substitute another
                  model (the backend does exactly that and logs MODEL_SUBSTITUTED). */}
              {model !== "" && !installed.some((m) => m.id === model) ? (
                <option value={model}>{`${model} · ${t("task.modelMissing")}`}</option>
              ) : null}
            </select>
          </label>

          <button type="button" className="btn" onClick={() => void persist()}>
            {t("task.saveSettings")}
          </button>
        </div>

        <p className="mono muted" style={{ margin: 0 }}>
          {t("task.outputDirHint")}
        </p>
        <p className="mono muted" style={{ margin: 0 }}>
          {t("task.pickHint")}
        </p>

        {error !== "" ? (
          <p className="mono" style={{ margin: 0, color: "var(--text-1)" }} role="alert">
            {error}
          </p>
        ) : null}
        {notice !== "" ? (
          <p className="mono muted" style={{ margin: 0 }}>
            {notice}
          </p>
        ) : null}
      </div>

      {items.length > 0 ? (
        <section style={{ marginTop: "var(--sp-6)" }}>
          <h2 className="section-title" style={{ marginTop: 0 }}>
            {t("task.queue")} · {items.filter((i) => i.state === "done").length}/{items.length}
          </h2>
          <table className="grid">
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td style={{ width: "38%" }}>{item.input.split(/[\\/]/).pop()}</td>
                  <td className="mono muted" style={{ width: "14%" }}>
                    {item.state === "running"
                      ? `${t("run.running")} ${item.percent}%`
                      : item.state === "done"
                        ? t("run.done")
                        : item.state === "failed"
                          ? t("run.failed")
                          : t("task.queued")}
                  </td>
                  <td className="muted" style={{ width: "30%" }}>
                    {item.state === "failed"
                      ? item.message
                      : item.segments > 0
                        ? t("run.segments", { n: String(item.segments) })
                        : ""}
                  </td>
                  <td>
                    {item.state === "done" && item.artifact_dir !== "" ? (
                      <button
                        type="button"
                        className="btn"
                        onClick={() => void openFolder(item.artifact_dir)}
                      >
                        {t("task.openFolder")}
                      </button>
                    ) : item.state === "waiting" ? (
                      <button
                        type="button"
                        className="btn"
                        onClick={() => void removeQueued(item.id).then(refresh)}
                      >
                        {t("task.remove")}
                      </button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      ) : null}
    </>
  );
}
