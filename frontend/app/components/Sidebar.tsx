"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { usePrefs } from "./Providers";

const items = [
  { href: "/", key: "nav.newTask" },
  { href: "/library", key: "nav.library" },
  { href: "/models", key: "nav.models" },
  { href: "/settings", key: "nav.settings" },
] as const;

export function Sidebar() {
  const pathname = usePathname();
  const { t, theme, setTheme, lang, setLang } = usePrefs();

  return (
    <aside className="sidebar">
      <div className="brand">STEMSCRIBE</div>

      <nav className="nav">
        {items.map((it) => (
          <Link key={it.href} href={it.href} data-active={pathname === it.href}>
            {t(it.key)}
          </Link>
        ))}
      </nav>

      <div className="sidebar__foot">
        <div className="row row--between">
          <span className="mono muted">{t("prefs.theme")}</span>
          <div className="seg" role="group" aria-label={t("prefs.theme")}>
            <button
              type="button"
              data-on={theme === "light"}
              aria-pressed={theme === "light"}
              onClick={() => setTheme("light")}
            >
              {t("prefs.theme.light")}
            </button>
            <button
              type="button"
              data-on={theme === "dark"}
              aria-pressed={theme === "dark"}
              onClick={() => setTheme("dark")}
            >
              {t("prefs.theme.dark")}
            </button>
          </div>
        </div>

        <div className="row row--between">
          <span className="mono muted">{t("prefs.lang")}</span>
          <div className="seg" role="group" aria-label={t("prefs.lang")}>
            {/* Language names stay in their own language; translating them is a
                known usability trap. */}
            {/* i18n-allow: language names stay in their own language */}
            <button
              type="button"
              data-on={lang === "zh"}
              aria-pressed={lang === "zh"}
              onClick={() => setLang("zh")}
            >
              中文{/* i18n-allow: language name */}
            </button>
            <button
              type="button"
              data-on={lang === "en"}
              aria-pressed={lang === "en"}
              onClick={() => setLang("en")}
            >
              EN
            </button>
          </div>
        </div>

        {/* Always visible: knowing what the machine is doing is the core promise. */}
        <div className="resource-bar">
          <span>{t("resource.gpu")}</span>
          <span>cuda12 · {t("resource.ready")}</span>
        </div>
      </div>
    </aside>
  );
}
