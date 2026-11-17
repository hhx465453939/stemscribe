"use client";

import { useEffect, useState } from "react";
import { listTasks, stageKey, type TaskView } from "@/lib/api";
import { usePrefs } from "./Providers";

const dotClass: Record<string, string> = {
  queued: "dot dot--queued",
  done: "dot dot--done",
  failed: "dot dot--fail",
};

export function TaskBoard() {
  const { t } = usePrefs();
  const [tasks, setTasks] = useState<TaskView[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let alive = true;
    listTasks().then((next) => {
      if (alive) {
        setTasks(next);
        setLoaded(true);
      }
    });
    return () => {
      alive = false;
    };
  }, []);

  if (!loaded) {
    return <p className="mono muted">{t("home.loading")}</p>;
  }
  if (tasks.length === 0) {
    return (
      <div className="empty">
        <pre className="empty__glyph">{`▁▂▃▅▇▅▃▂▁▂▃▅▇`}</pre>
        {t("home.empty")}
      </div>
    );
  }

  return (
    <div className="stack">
      {tasks.map((task) => {
        const running = !["done", "failed", "queued"].includes(task.stage);
        const pct = task.total > 0 ? Math.round((task.current / task.total) * 100) : 0;
        const title = task.titleKey ? t(task.titleKey) : (task.title ?? task.id);
        const hint = task.errorHintKey ? t(task.errorHintKey) : (task.errorHint ?? task.detail);

        return (
          <div key={task.id} className={running ? "panel panel--lit" : "panel"}>
            <div className="row row--between">
              <div className="row">
                <span className={dotClass[task.stage] ?? "dot dot--run"} aria-hidden />
                <strong>{title}</strong>
              </div>
              <span className="mono">{t(stageKey(task.stage))}</span>
            </div>

            {task.total > 0 ? (
              <div className="stack" style={{ marginTop: "var(--sp-3)" }}>
                <div className={task.stage === "done" ? "bar bar--done" : "bar"}>
                  <span style={{ width: `${pct}%` }} />
                </div>
                <div className="row row--between">
                  <span className="mono">{task.detail}</span>
                  <span className="mono">
                    {task.current}/{task.total} {t("home.segments")}
                  </span>
                </div>
              </div>
            ) : (
              <p className="mono muted" style={{ margin: "var(--sp-3) 0 0" }}>
                {hint}
              </p>
            )}
          </div>
        );
      })}
    </div>
  );
}
