"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePrefs } from "./Providers";
import {
  enqueue,
  getQueue,
  getSettings,
  listCatalog,
  onFilesDropped,
  removeQueued,
  saveSettings,
  shell,
  stopQueued,
  type CatalogItemView,
  type QueueItemView,
} from "@/lib/api";
import type { Key } from "@/lib/i18n";
import {
  addStaged,
  baseName,
  clearStaged,
  formatSize,
  removeStaged,
  type StagedFile,
} from "@/lib/staging";

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

/** Pipeline stage id -> dictionary key. Unknown ids fall through verbatim. */
const STAGE_KEYS: Record<string, Key> = {
  resolve: "stage.preparing",
  demux: "stage.preparing",
  separate: "stage.separating",
  transcribe: "stage.transcribing",
  align: "stage.aligning",
  polish: "stage.polishing",
  export: "stage.exporting",
};

/** Stem name -> dictionary key. Unknown names render as themselves. */
const STEM_KEYS: Record<string, Key> = {
  vocals: "task.stem.vocals",
  instrumental: "task.stem.instrumental",
  drums: "task.stem.drums",
  bass: "task.stem.bass",
  other: "task.stem.other",
  guitar: "task.stem.guitar",
  piano: "task.stem.piano",
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

function elapsed(ms: number | undefined): string {
  if (!ms || ms <= 0) return "";
  const total = Math.floor(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function joinWorkdirLibraries(workdir: string): string {
  if (workdir === "") return "";
  const sep = workdir.includes("\\") ? "\\" : "/";
  return workdir.endsWith(sep) ? `${workdir}libraries` : `${workdir}${sep}libraries`;
}

export function NewTask() {
  const { t } = usePrefs();
  // Staged entries are paths (plus size for display). No file is copied, moved or
  // read until the user presses Start (ADR-042).
  const [staged, setStaged] = useState<StagedFile[]>([]);
  const [manual, setManual] = useState("");
  const [outputDir, setOutputDir] = useState("");
  const [model, setModel] = useState("");
  // Stage 1 is separation: which separator model runs, and which of the stems it
  // produces are kept. Both are per-run choices, sent with the queue request, so a
  // one-off Demucs run does not need the Models page first.
  const [separator, setSeparator] = useState("");
  const [sepModels, setSepModels] = useState<CatalogItemView[]>([]);
  const [stems, setStems] = useState<string[]>([]);
  const [have, setHave] = useState<string[]>([]);
  const [language, setLanguage] = useState("zh");
  const [workdir, setWorkdir] = useState("");
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
    void Promise.all([getSettings(), listCatalog()]).then(([s, c]) => {
      if (s) {
        setOutputDir(s.output_dir ?? "");
        setModel(s.model ?? "");
        setWorkdir(s.workdir ?? "");
      }
      // Only installed models are selectable: offering an uninstalled one would fail
      // at run time, which is the trap the audit called out.
      setInstalled(c.installed.filter((m) => m.kind === "whisper"));
      // Stage-1 selectors come from the same catalog, so a model installed on the
      // Models page shows up here without a reload of anything else.
      const seps = c.installed.filter((m) => m.kind === "separator");
      setSepModels(seps);
      setHave(c.installed.map((m) => m.id));
      const configured = s?.separator ?? "";
      setSeparator(configured);
      // The configured model's declared stems are the default retention set; the
      // user can uncheck the ones that cost disk without ever wanting them.
      const chosen = seps.find((m) => m.id === configured);
      if (chosen?.stems && chosen.stems.length > 0) setStems(chosen.stems);
    });
    // Language lives with the other engine parameters, so an explicit choice on the
    // parameter page survives a reload too.
    try {
      const raw = localStorage.getItem(PARAMS_KEY);
      const saved = raw ? (JSON.parse(raw) as Record<string, string>) : {};
      if (saved.input_language) setLanguage(saved.input_language);
    } catch {
      /* corrupt storage: keep the default */
    }
  }, [refresh]);

  // Prerequisites the chosen run needs but the disk does not have. Computed from
  // the live selectors, not captured once at load: switching to another installed
  // separator must clear the gap instead of leaving a stale banner.
  const missing = useMemo(() => {
    const present = new Set(have);
    const gaps: string[] = [];
    // Separation is mandatory (ADR-037): the Python runtime and the chosen weights
    // are hard prerequisites, not conveniences.
    if (!present.has("separator-python")) gaps.push("separator-python");
    if (separator !== "" && !present.has(separator)) gaps.push(separator);
    if (model !== "" && !present.has(model)) gaps.push(model);
    return gaps;
  }, [have, separator, model]);

  const selectedSep = useMemo(
    () => sepModels.find((m) => m.id === separator),
    [sepModels, separator],
  );
  // The stems this model declares. The list is the model's property (ADR-019), so
  // the checkboxes cannot offer a track the engine would never write. Stable in a
  // memo so the run callback does not see a new array on every render.
  const stemChoices = useMemo(() => selectedSep?.stems ?? [], [selectedSep]);

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

  /** Paths -> {path,size}. Only stats the file; never copies or opens media. */
  const statPaths = useCallback(async (paths: string[]): Promise<StagedFile[]> => {
    const bridge = shell();
    if (bridge?.StatFiles) {
      const metas = await bridge.StatFiles(paths);
      return metas.filter((m) => !m.error).map((m) => ({ path: m.path, size: m.size }));
    }
    return paths.map((p) => ({ path: p, size: -1 }));
  }, []);

  const addPaths = useCallback(
    async (paths: string[]) => {
      if (paths.length === 0) return;
      const files = await statPaths(paths);
      setStaged((prev) => addStaged(prev, files));
      setError("");
      setNotice("");
    },
    [statPaths],
  );

  const pick = useCallback(async () => {
    const bridge = shell();
    if (!bridge?.PickFiles) {
      setNotice(t("task.pickHint"));
      return;
    }
    const files = await bridge.PickFiles();
    if (files && files.length > 0) await addPaths(files);
  }, [addPaths, t]);

  // Dropped files are staged, not started (ADR-042): the old behaviour queued them
  // immediately, which left no moment to check files, model or output folder.
  useEffect(() => {
    return onFilesDropped((dropped) => {
      void addPaths(dropped);
    });
  }, [addPaths]);

  const chooseOutput = useCallback(async () => {
    const bridge = shell();
    if (!bridge?.PickFolder) {
      setNotice(t("task.pickHint"));
      return;
    }
    const dir = await bridge.PickFolder(outputDir);
    if (!dir) return;
    // Persist immediately so the choice survives a reload; an unwritable folder is
    // refused here, not after separation.
    const res = await saveSettings({ output_dir: dir });
    if (res.error) {
      setError(res.error);
      return;
    }
    setOutputDir(dir);
    setError("");
    setNotice(t("task.saved"));
  }, [outputDir, t]);

  const openFolder = useCallback(async (dir: string) => {
    if (dir === "") {
      setNotice(t("task.outputDirHint"));
      return;
    }
    const bridge = shell();
    if (bridge?.OpenFolder) {
      await bridge.OpenFolder(dir);
      return;
    }
    // Browser fallback: the path is still useful, just not clickable.
    void navigator.clipboard?.writeText(dir);
    setNotice(dir);
  }, [t]);

  const openOutput = useCallback(async () => {
    const target = outputDir.trim() === "" ? joinWorkdirLibraries(workdir) : outputDir;
    await openFolder(target);
  }, [outputDir, workdir, openFolder]);

  const start = useCallback(async () => {
    if (busy) return;
    const manualPaths = manual
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
    const seen = new Set<string>();
    const inputs: string[] = [];
    for (const p of [...staged.map((f) => f.path), ...manualPaths]) {
      if (seen.has(p.toLowerCase())) continue;
      seen.add(p.toLowerCase());
      inputs.push(p);
    }
    if (inputs.length === 0) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      // An empty field means "put bundles under <workdir>/libraries" (the hint says so).
      // Sending "" with the enqueue is not enough: the run falls back to the *saved*
      // output_dir, so a previously persisted folder would silently win over the
      // field the user just cleared. Clear the setting first, then enqueue.
      if (outputDir.trim() === "") {
        const cleared = await saveSettings({ output_dir: "" });
        if (cleared.error) {
          setError(cleared.error);
          return;
        }
      }
      // Parameters come from the parameter page; sending {} here was why every knob
      // looked adjustable and changed nothing.
      const res = await enqueue(inputs, {
        outputDir,
        model,
        separator,
        // An empty list means "keep everything"; that is the backend's documented
        // default and avoids sending a stale set when the model is unknown here.
        stems: stemChoices.length > 0 ? stems : [],
        params: { ...readParams(), input_language: language },
      });
      if (res.error) {
        setError(res.error);
        return;
      }
      const rejected = new Set((res.rejected ?? []).map((p) => p.toLowerCase()));
      const accepted = new Set(inputs.filter((p) => !rejected.has(p.toLowerCase())));
      // Only accepted files leave the staging list; a rejected one stays visible with
      // the reason in the banner instead of silently disappearing.
      setStaged((prev) => prev.filter((f) => !accepted.has(f.path.toLowerCase())));
      setManual((prev) =>
        prev
          .split("\n")
          .filter((line) => {
            const p = line.trim();
            return p !== "" && !accepted.has(p.toLowerCase());
          })
          .join("\n"),
      );
      if (rejected.size > 0) setNotice(`${t("task.paths")}: ${[...rejected].join(", ")}`);
      await refresh();
    } finally {
      setBusy(false);
    }
  }, [busy, manual, staged, outputDir, model, separator, stems, stemChoices, language, refresh, t]);

  const stop = useCallback(
    async (id: string) => {
      const res = await stopQueued(id);
      if (res.error) setError(res.error);
      await refresh();
    },
    [refresh],
  );

  const changeLanguage = useCallback((value: string) => {
    setLanguage(value);
    try {
      const raw = localStorage.getItem(PARAMS_KEY);
      const saved = raw ? (JSON.parse(raw) as Record<string, string>) : {};
      localStorage.setItem(PARAMS_KEY, JSON.stringify({ ...saved, input_language: value }));
    } catch {
      /* storage unavailable: the value still applies to this session */
    }
  }, []);

  const persist = useCallback(async () => {
    // Saving here also makes the stage-1 choice the default for future runs; the
    // Models page shows the same value, so the two cannot disagree.
    const res = await saveSettings({ output_dir: outputDir, model, separator });
    if (res.error) setError(res.error);
    else setNotice(t("task.saved"));
  }, [outputDir, model, separator, t]);

  const totalStaged = staged.length + manual.split("\n").filter((s) => s.trim() !== "").length;
  const ready = totalStaged > 0 && missing.length === 0 && !busy;

  const changeSeparator = useCallback(
    (id: string) => {
      setSeparator(id);
      // Switching models resets retention to "everything this model produces";
      // keeping a previous selection could silently drop the new model's tracks.
      const chosen = sepModels.find((m) => m.id === id);
      setStems(chosen?.stems ?? []);
    },
    [sepModels],
  );

  const toggleStem = useCallback((stem: string) => {
    // vocals is what the transcript is made from; refusing the uncheck is kinder
    // than letting the run fail after separation.
    if (stem === "vocals") return;
    setStems((prev) =>
      prev.includes(stem) ? prev.filter((s) => s !== stem) : [...prev, stem],
    );
  }, []);

  const stateLabel = (item: QueueItemView): string => {
    switch (item.state) {
      case "running":
        return t("run.running");
      case "done":
        return t("run.done");
      case "failed":
        return t("run.failed");
      case "canceled":
        return t("task.canceled");
      default:
        return t("task.queued");
    }
  };

  const stageLabel = (item: QueueItemView): string => {
    if (item.state !== "running" && item.state !== "waiting") return "";
    const key = item.stage ? STAGE_KEYS[item.stage] : undefined;
    const name = key ? t(key) : item.stage ?? "";
    const time = elapsed(item.elapsed_ms);
    return [name, item.state === "running" ? `${item.percent}%` : "", time]
      .filter(Boolean)
      .join(" · ");
  };

  return (
    <>
      <div className="panel stack">
        <section className="dropzone" aria-label={t("task.dropTitle")}>
          <p className="dropzone__title">{t("task.dropTitle")}</p>
          <p className="mono muted dropzone__hint">{t("task.dropHint")}</p>
          <div className="row" style={{ gap: "var(--sp-3)", flexWrap: "wrap" }}>
            <button type="button" className="btn" onClick={() => void pick()}>
              {t("task.browse")}
            </button>
            <span className="mono muted">{t("task.pickHint")}</span>
          </div>
        </section>

        <div className="row row--between">
          <h2 className="section-title" style={{ margin: 0 }}>
            {t("task.staged")}
            {staged.length > 0 ? ` · ${staged.length}` : ""}
          </h2>
          {staged.length > 0 ? (
            <button type="button" className="btn" onClick={() => setStaged(clearStaged())}>
              {t("task.clear")}
            </button>
          ) : null}
        </div>

        {staged.length > 0 ? (
          <table className="grid">
            <tbody>
              {staged.map((f) => (
                <tr key={f.path}>
                  <td style={{ width: "62%" }} title={f.path}>
                    {baseName(f.path)}
                  </td>
                  <td className="num mono muted" style={{ width: "22%" }}>
                    {formatSize(f.size)}
                  </td>
                  <td style={{ width: "16%" }}>
                    <button
                      type="button"
                      className="btn"
                      onClick={() => setStaged((prev) => removeStaged(prev, f.path))}
                    >
                      {t("task.remove")}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p className="mono muted" style={{ margin: 0 }}>
            {t("task.empty")}
          </p>
        )}

        <div className="row" style={{ gap: "var(--sp-6)", flexWrap: "wrap" }}>
          <label className="row" style={{ gap: "var(--sp-2)", flex: "1 1 420px", minWidth: 0 }}>
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
            <button type="button" className="btn" onClick={() => void chooseOutput()}>
              {t("task.outputPick")}
            </button>
            <button type="button" className="btn" onClick={() => void openOutput()}>
              {t("task.outputOpen")}
            </button>
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

          <label className="row" style={{ gap: "var(--sp-2)" }}>
            <span className="mono muted" style={{ whiteSpace: "nowrap", flexShrink: 0 }}>
              {t("task.language")}
            </span>
            <select
              className="input"
              value={language}
              onChange={(e) => changeLanguage(e.target.value)}
              aria-label={t("task.language")}
            >
              <option value="zh">{t("task.langZh")}</option>
              <option value="en">{t("task.langEn")}</option>
              <option value="auto">{t("task.langAuto")}</option>
            </select>
          </label>

          <button type="button" className="btn" onClick={() => void persist()}>
            {t("task.saveSettings")}
          </button>
        </div>

        <div
          className="row"
          style={{ gap: "var(--sp-6)", flexWrap: "wrap", alignItems: "center" }}
        >
          <label className="row" style={{ gap: "var(--sp-2)" }}>
            <span className="mono muted" style={{ whiteSpace: "nowrap", flexShrink: 0 }}>
              {t("task.separator")}
            </span>
            <select
              className="input"
              value={separator}
              onChange={(e) => changeSeparator(e.target.value)}
              aria-label={t("task.separator")}
            >
              <option value="">—</option>
              {sepModels.map((m) => (
                <option key={m.id} value={m.id}>
                  {[
                    m.name || m.id,
                    m.architecture,
                    m.stems && m.stems.length > 0
                      ? t("models.stems").replace("{n}", String(m.stems.length))
                      : "",
                    m.non_commercial ? t("models.nonCommercial") : "",
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </option>
              ))}
              {separator !== "" && !sepModels.some((m) => m.id === separator) ? (
                <option value={separator}>{`${separator} · ${t("task.modelMissing")}`}</option>
              ) : null}
            </select>
          </label>

          {stemChoices.length > 0 ? (
            <div className="row" style={{ gap: "var(--sp-3)", flexWrap: "wrap" }}>
              <span className="mono muted" style={{ whiteSpace: "nowrap" }}>
                {t("task.stems")}
              </span>
              {stemChoices.map((stem) => {
                const locked = stem === "vocals";
                return (
                  <label
                    key={stem}
                    className="row"
                    style={{ gap: "var(--sp-1)", cursor: locked ? "default" : "pointer" }}
                    title={locked ? t("task.stem.locked") : undefined}
                  >
                    {locked ? (
                      // A disabled native checkbox is drawn grey, which reads as
                      // "not kept" in the dark theme — the opposite of what
                      // locking vocals means. A filled marker states the truth
                      // in both themes and cannot be clicked.
                      <span
                        className="stem-lock"
                        role="img"
                        aria-label={t("task.stem.locked")}
                      />
                    ) : (
                      <input
                        type="checkbox"
                        checked={stems.includes(stem)}
                        onChange={() => toggleStem(stem)}
                      />
                    )}
                    <span className="mono">{STEM_KEYS[stem] ? t(STEM_KEYS[stem]) : stem}</span>
                  </label>
                );
              })}
            </div>
          ) : null}
        </div>

        <p className="mono muted" style={{ margin: 0 }}>
          {t("task.stemsHint")}
        </p>

        <p className="mono muted" style={{ margin: 0 }}>
          {t("task.outputDirHint")}
        </p>

        <details className="manual-paths">
          <summary className="mono muted">{t("task.manualPaths")}</summary>
          <textarea
            className="input mono"
            rows={3}
            value={manual}
            onChange={(e) => setManual(e.target.value)}
            placeholder={"C:\\videos\\a.mp4\nC:\\videos\\b.mkv"}
            spellCheck={false}
            style={{ marginTop: "var(--sp-2)" }}
          />
        </details>

        {missing.length > 0 ? (
          <p className="mono" style={{ margin: 0, color: "var(--text-1)" }} role="alert">
            {t("task.notReady")}: {missing.join(", ")} ·{" "}
            <Link href="/models">{t("task.goModels")}</Link>
          </p>
        ) : null}

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

        <div className="row" style={{ gap: "var(--sp-3)" }}>
          <button
            type="button"
            className="btn btn--primary"
            disabled={!ready}
            onClick={() => void start()}
          >
            {totalStaged > 0 ? t("task.startCount", { n: String(totalStaged) }) : t("task.start")}
          </button>
        </div>
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
                  <td style={{ width: "34%" }} title={item.input}>
                    {baseName(item.input)}
                  </td>
                  <td className="mono muted" style={{ width: "16%" }}>
                    {stateLabel(item)}
                  </td>
                  <td className="muted" style={{ width: "28%" }}>
                    {stageLabel(item)}
                    {item.state === "failed" || item.state === "canceled"
                      ? item.message || elapsed(item.elapsed_ms)
                      : item.segments > 0
                        ? t("run.segments", { n: String(item.segments) })
                        : ""}
                    {item.warnings && item.warnings.length > 0 ? (
                      <div className="mono" style={{ color: "var(--text-1)" }}>
                        {item.warnings
                          .map((code) =>
                            code === "MODEL_SUBSTITUTED" ? t("warn.modelSubstituted") : code,
                          )
                          .join(" · ")}
                      </div>
                    ) : null}
                  </td>
                  <td style={{ width: "22%" }}>
                    {item.state === "running" ? (
                      <button
                        type="button"
                        className="btn"
                        disabled={item.stopping === true}
                        onClick={() => void stop(item.id)}
                      >
                        {item.stopping === true ? t("task.stopping") : t("task.stop")}
                      </button>
                    ) : item.state === "waiting" ? (
                      <button
                        type="button"
                        className="btn"
                        onClick={() => void removeQueued(item.id).then(refresh)}
                      >
                        {t("task.remove")}
                      </button>
                    ) : item.artifact_dir !== "" ? (
                      <button
                        type="button"
                        className="btn"
                        onClick={() => void openFolder(item.artifact_dir)}
                      >
                        {t("task.openFolder")}
                      </button>
                    ) : item.state === "canceled" ? (
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
