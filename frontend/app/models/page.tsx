"use client";

import { usePrefs } from "../components/Providers";

export default function ModelsPage() {
  const { t } = usePrefs();

  const installed = [
    { name: "Whisper Medium (q5_0)", kind: "models.kind.stt", size: "540 MB", vram: "2.6 GB", license: "MIT" },
    { name: "MDX-Net · Kim Vocal 2", kind: "models.kind.sep", size: "63 MB", vram: "1.5 GB", license: "MIT" },
  ] as const;

  const available = [
    { name: "Whisper Small (q5_0)", kind: "models.kind.stt", size: "190 MB", vram: "1.2 GB", license: "MIT", tier: "models.tier.light" },
    { name: "Whisper Large v3 (q5_0)", kind: "models.kind.stt", size: "1.08 GB", vram: "6.2 GB", license: "MIT", tier: "models.tier.max" },
    { name: "whisper.cpp CUDA 12", kind: "models.kind.runtime", size: "48 MB", vram: "0.3 GB", license: "MIT", tier: null },
    { name: "FFmpeg 7.1 (LGPL)", kind: "models.kind.runtime", size: "68 MB", vram: "—", license: "LGPL", tier: null },
  ] as const;

  return (
    <>
      <h1 className="page-title">{t("models.title")}</h1>
      <p className="page-sub">{t("models.subtitle")}</p>

      <h2 className="section-title">{t("models.installed")}</h2>
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
          {installed.map((m) => (
            <tr key={m.name}>
              <td>{m.name}</td>
              <td className="mono">{t(m.kind)}</td>
              <td className="num">{m.size}</td>
              <td className="num">{m.vram}</td>
              <td className="mono">{m.license}</td>
              <td>
                <span className="dot dot--done" aria-hidden /> {t("models.verified")}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2 className="section-title">{t("models.available")}</h2>
      <table className="grid">
        <thead>
          <tr>
            <th>{t("models.colName")}</th>
            <th>{t("models.colKind")}</th>
            <th className="num">{t("models.colSize")}</th>
            <th className="num">{t("models.colVram")}</th>
            <th>{t("models.colLicense")}</th>
            <th>{t("models.colTier")}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {available.map((m) => (
            <tr key={m.name}>
              <td>{m.name}</td>
              <td className="mono">{t(m.kind)}</td>
              <td className="num">{m.size}</td>
              <td className="num">{m.vram}</td>
              <td className="mono">{m.license}</td>
              <td>
                {m.tier ? <span className="tag">{t(m.tier)}</span> : <span className="muted">—</span>}
              </td>
              <td style={{ textAlign: "right" }}>
                <button className="btn" type="button">
                  {t("models.install")}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <p className="mono muted" style={{ marginTop: "var(--sp-6)" }}>
        {t("models.footnote")}
      </p>
    </>
  );
}
