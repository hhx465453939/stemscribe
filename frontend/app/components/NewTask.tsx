"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePrefs } from "./Providers";

type RunState = "queued" | "running" | "done" | "failed";

interface RunStatus {
  id: string;
  state: RunState;
  input: string;
  model: string;
  stage: string;
  stage_index: number;
  stage_total: number;
  percent: number;
  segments: number;
  artifact_dir: string;
  message: string;
  started_at: string;
  ended_at: string;
}

const POLL_MS = 500;

// Fetches the current run. Used on mount so reopening the window or reloading the
// page reconnects instead of showing a blank panel over a run that is still going.
async function fetchLatest(): Promise<RunStatus | null> {
  try {
    const res = await fetch("/api/runs/latest", { cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as RunStatus;
  } catch {
    return null;
  }
}

export function NewTask() {
  const { t } = usePrefs();
  const [value, setValue] = useState("");
  const [separate, setSeparate] = useState(false);
  const [polish, setPolish] = useState(false);
  const [run, setRun] = useState<RunStatus | null>(null);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const timer = useRef<number | null>(null);

  const active = run?.state === "queued" || run?.state === "running";
  const busy = active || starting;

  useEffect(() => {
    let alive = true;
    fetchLatest().then((s) => {
      if (alive && s) setRun(s);
    });
    return () => {
      alive = false;
    };
  }, []);

  // Poll only while a run is live. Progress comes from real stage events on the
  // server; nothing here invents movement (ADR-023).
  useEffect(() => {
    if (!active) {
      if (timer.current !== null) window.clearInterval(timer.current);
      timer.current = null;
      return;
    }
    timer.current = window.setInterval(() => {
      setNow(Date.now());
      fetchLatest().then((s) => {
        if (s) setRun(s);
      });
    }, POLL_MS);
    return () => {
      if (timer.current !== null) window.clearInterval(timer.current);
      timer.current = null;
    };
  }, [active]);

  const start = useCallback(async () => {
    const input = value.trim();
    if (input === "" || busy) return;
    setStarting(true);
    setError("");
    setCopied(false);
    try {
      const res = await fetch("/api/runs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ input, separate, params: {} }),
      });
      const body = (await res.json().catch(() => ({}))) as { error?: string };
      if (!res.ok) {
        setError(body.error || `HTTP ${res.status}`);
        return;
      }
      const st = await fetchLatest();
      if (st) setRun(st);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setStarting(false);
    }
  }, [value, separate, busy]);

  const elapsed = (() => {
    if (!run) return 0;
    const from = Date.parse(run.started_at);
    if (Number.isNaN(from)) return 0;
    const to = run.ended_at ? Date.parse(run.ended_at) : now;
    return Math.max(0, Math.round((to - from) / 1000));
  })();

  const stateLabel = run
    ? run.state === "queued"
      ? t("run.starting")
      : run.state === "running"
        ? t("run.running")
        : run.state === "done"
          ? t("run.done")
          : t("run.failed")
    : "";

  return (
    <>
      <div className="panel stack">
        <div className="row">
          <input
            className="input"
            placeholder={t("home.placeholder")}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void start();
            }}
            aria-label={t("home.placeholder")}
          />
          <button
            className="btn btn--primary"
            type="button"
            disabled={value.trim() === "" || busy}
            onClick={() => void start()}
          >
            {busy ? `··· ${t("run.running")}` : t("home.start")}
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

        {error !== "" ? (
          <p className="mono" style={{ margin: 0, color: "var(--text-1)" }} role="alert">
            {error}
          </p>
        ) : null}
      </div>

      {run ? (
        <section
          className="panel stack"
          style={{
            marginTop: "var(--sp-4)",
            borderLeft: "2px solid var(--lamp)",
          }}
          aria-live="polite"
        >
          <div className="row row--between">
            <span className="mono">
              {stateLabel} · {run.input.split(/[\\/]/).pop()}
            </span>
            <span className="mono muted">
              {t("run.elapsed", { s: String(elapsed) })}
            </span>
          </div>

          {/* A 2px rule, not a gradient: the single light source is the only accent. */}
          <div
            style={{
              height: 2,
              background: "var(--line)",
              position: "relative",
              overflow: "hidden",
            }}
            role="progressbar"
            aria-valuenow={run.percent}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div
              style={{
                position: "absolute",
                inset: 0,
                width: `${run.percent}%`,
                background: run.state === "failed" ? "var(--text-2)" : "var(--lamp)",
                transition: "width 240ms linear",
              }}
            />
          </div>

          <div className="row row--between">
            <span className="mono muted">
              {run.stage && run.state === "running"
                ? `${t("run.stageNow")}: ${run.stage}`
                : run.state === "done"
                  ? t("run.done")
                  : t("run.waiting")}
            </span>
            <span className="mono muted">
              {t("run.progress", {
                done: String(run.stage_index),
                total: String(run.stage_total),
              })}
            </span>
          </div>

          {run.state === "done" ? (
            <div className="stack" style={{ gap: "var(--sp-2)" }}>
              <span className="mono">{t("run.segments", { n: String(run.segments) })}</span>
              <div className="row row--between">
                <span className="mono muted" style={{ wordBreak: "break-all" }}>
                  {t("run.artifact")}: {run.artifact_dir}
                </span>
                <button
                  type="button"
                  className="btn"
                  onClick={() => {
                    void navigator.clipboard?.writeText(run.artifact_dir);
                    setCopied(true);
                  }}
                >
                  {copied ? t("run.copied") : t("run.copy")}
                </button>
              </div>
            </div>
          ) : null}

          {run.state === "failed" ? (
            <p className="mono" style={{ margin: 0 }} role="alert">
              {run.message}
            </p>
          ) : null}
        </section>
      ) : null}
    </>
  );
}
