"use client";

import { useEffect, useState } from "react";
import { usePrefs } from "../components/Providers";
import { formatDuration, listLibrary, type LibraryItemView } from "@/lib/api";

export default function LibraryPage() {
  const { t } = usePrefs();
  const [items, setItems] = useState<LibraryItemView[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let alive = true;
    listLibrary().then((next) => {
      if (alive) {
        setItems(next);
        setLoaded(true);
      }
    });
    return () => {
      alive = false;
    };
  }, []);

  return (
    <>
      <h1 className="page-title">{t("library.title")}</h1>
      <p className="page-sub">{t("library.subtitle")}</p>

      {!loaded ? <p className="mono muted">{t("home.loading")}</p> : null}

      {loaded && items.length === 0 ? (
        <div className="empty">
          <pre className="empty__glyph">{`─────────────────────────`}</pre>
          {t("library.empty")}
        </div>
      ) : null}

      {items.length > 0 ? (
        <table className="grid">
          <thead>
            <tr>
              <th>{t("library.colTitle")}</th>
              <th>{t("library.colId")}</th>
              <th className="num">{t("library.colSegments")}</th>
              <th className="num">{t("library.colDuration")}</th>
              <th>{t("library.colStatus")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>{item.title}</td>
                <td className="mono">{item.id}</td>
                <td className="num">{item.segments}</td>
                <td className="num">{formatDuration(item.duration_ms)}</td>
                <td>
                  <span
                    className={item.status === "done" ? "dot dot--done" : "dot"}
                    aria-hidden
                  />{" "}
                  {item.status === "done" ? t("stage.done") : t("stage.failed")}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </>
  );
}
