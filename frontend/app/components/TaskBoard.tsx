"use client";

import { useEffect, useState } from "react";
import { listTasks, stageLabel, type TaskView } from "@/lib/api";

const dotClass: Record<string, string> = {
  queued: "dot dot--queued",
  done: "dot dot--done",
  failed: "dot dot--fail",
};

export function TaskBoard() {
  const [tasks, setTasks] = useState<TaskView[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let alive = true;
    listTasks().then((t) => {
      if (alive) {
        setTasks(t);
        setLoaded(true);
      }
    });
    return () => {
      alive = false;
    };
  }, []);

  if (!loaded) {
    return <p className="mono muted">载入中…</p>;
  }
  if (tasks.length === 0) {
    return (
      <div className="empty">
        <pre className="empty__glyph">{`▁▂▃▅▇▅▃▂▁▂▃▅▇`}</pre>
        还没有任务。粘贴一条链接，或拖入一个本地文件。
      </div>
    );
  }

  return (
    <div className="stack">
      {tasks.map((t) => {
        const running = !["done", "failed", "queued"].includes(t.stage);
        const pct = t.total > 0 ? Math.round((t.current / t.total) * 100) : 0;

        return (
          <div key={t.id} className={running ? "panel panel--lit" : "panel"}>
            <div className="row row--between">
              <div className="row">
                <span className={dotClass[t.stage] ?? "dot dot--run"} aria-hidden />
                <strong>{t.title}</strong>
              </div>
              <span className="mono">{stageLabel(t.stage)}</span>
            </div>

            {t.total > 0 ? (
              <div className="stack" style={{ marginTop: "var(--sp-3)" }}>
                <div className="bar">
                  <span style={{ width: `${pct}%` }} />
                </div>
                <div className="row row--between">
                  <span className="mono">{t.detail}</span>
                  <span className="mono">
                    {t.current}/{t.total} 段
                  </span>
                </div>
              </div>
            ) : (
              <p className="mono muted" style={{ margin: "var(--sp-3) 0 0" }}>
                {t.errorHint ?? t.detail}
              </p>
            )}
          </div>
        );
      })}
    </div>
  );
}
