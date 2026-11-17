"use client";

import { NewTask } from "./components/NewTask";
import { TaskBoard } from "./components/TaskBoard";
import { usePrefs } from "./components/Providers";

export default function HomePage() {
  const { t } = usePrefs();

  return (
    <>
      <h1 className="page-title">{t("home.title")}</h1>
      <p className="page-sub">{t("home.subtitle")}</p>

      <NewTask />

      <h2 className="section-title">{t("home.tasks")}</h2>
      <TaskBoard />
    </>
  );
}
