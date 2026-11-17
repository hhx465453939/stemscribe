"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { type Key, type Lang, type Vars, translate } from "@/lib/i18n";

export type Theme = "light" | "dark";

interface Prefs {
  theme: Theme;
  setTheme: (t: Theme) => void;
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (key: Key, vars?: Vars) => string;
}

const PrefsContext = createContext<Prefs | null>(null);

const THEME_KEY = "stemscribe.theme";
const LANG_KEY = "stemscribe.lang";

// The boot script in layout.tsx has already resolved URL override -> storage ->
// system preference and written the result onto <html>. Reading it back here
// keeps a single source of truth and makes `?theme=dark&lang=en` screenshots
// deterministic without any scripted clicking.
function readTheme(): Theme {
  const v = document.documentElement.dataset.theme;
  return v === "dark" ? "dark" : "light";
}

function readLang(): Lang {
  return document.documentElement.lang.toLowerCase().startsWith("zh") ? "zh" : "en";
}

export function Providers({ children }: { children: React.ReactNode }) {
  // Initial state deliberately matches what the server prerendered (light/zh).
  // Anything else would make hydration text mismatch; instead we adopt the real
  // preference in an effect, so colours are already correct (boot script) and
  // only non-default locales swap their copy on the first frame.
  const [theme, setTheme] = useState<Theme>("light");
  const [lang, setLang] = useState<Lang>("zh");
  const [adopted, setAdopted] = useState(false);

  useEffect(() => {
    setTheme(readTheme());
    setLang(readLang());
    setAdopted(true);
  }, []);

  useEffect(() => {
    if (!adopted) return;
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch {
      /* private mode: preference just will not persist */
    }
  }, [theme, adopted]);

  useEffect(() => {
    if (!adopted) return;
    document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
    try {
      localStorage.setItem(LANG_KEY, lang);
    } catch {
      /* ignore */
    }
  }, [lang, adopted]);

  const t = useCallback(
    (key: Key, vars?: Vars) => translate(lang, key, vars),
    [lang],
  );

  const value = useMemo<Prefs>(
    () => ({ theme, setTheme, lang, setLang, t }),
    [theme, lang, t],
  );

  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}

export function usePrefs(): Prefs {
  const ctx = useContext(PrefsContext);
  if (!ctx) throw new Error("usePrefs must be used inside <Providers>");
  return ctx;
}
