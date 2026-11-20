"use client";

import { NewTask } from "./components/NewTask";
import { usePrefs } from "./components/Providers";

export default function HomePage() {
  const { t } = usePrefs();

  return (
    <>
      <h1 className="page-title">{t("home.title")}</h1>
      <p className="page-sub">{t("home.subtitle")}</p>

      <NewTask />
      {/* The task board used to sit here too, listing the same finished bundles the
          library page already shows -- two places to look at one fact. The queue above
          covers the current batch; the library covers history. */}
    </>
  );
}
