"use client";

import { usePrefs } from "../components/Providers";

export default function LibraryPage() {
  const { t } = usePrefs();

  const items = [
    { id: "BV1xx411c7mD", titleKey: "demo.interview", segs: 240, dur: "42:10" },
    { id: "BV1yy411c7mE", titleKey: "demo.teardown", segs: 96, dur: "18:02" },
  ] as const;

  return (
    <>
      <h1 className="page-title">{t("library.title")}</h1>
      <p className="page-sub">{t("library.subtitle")}</p>

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
              <td>{t(item.titleKey)}</td>
              <td className="mono">{item.id}</td>
              <td className="num">{item.segs}</td>
              <td className="num">{item.dur}</td>
              <td>
                <span className="dot dot--done" aria-hidden /> {t("stage.done")}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}
