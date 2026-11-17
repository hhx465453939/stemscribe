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
  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}

function readLang(): Lang {
  return document.documentElement.lang.toLowerCase().startsWith("zh") ? "zh" : "en";
}

export function Providers({ children }: { children: React.ReactNode }) {
  // Initial state deliberately matches what the server prerendered (light/zh).
  // Anything else would make hydration text mismatch; instead we adopt the real
  // preference in an effect, so colours are already correct (boot script) and
  // only non-default locales swap their copy on the first frame.
  const [theme, setThemeState] = useState<Theme>("light");
  const [lang, setLangState] = useState<Lang>("zh");
  const [adopted, setAdopted] = useState(false);

  useEffect(() => {
    setThemeState(readTheme());
    setLangState(readLang());
    setAdopted(true);
  }, []);

  useEffect(() => {
    if (!adopted) return;
    document.documentElement.dataset.theme = theme;
  }, [theme, adopted]);

  useEffect(() => {
    if (!adopted) return;
    document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
  }, [lang, adopted]);

  // Persist inside the user action, never in an effect.
  //
  // Writing from an effect meant the value derived from `?theme=`/`?lang=` was
  // also written back, so opening a documentation deep link silently overwrote
  // the reader's saved preference. Persisting only on an explicit choice keeps
  // URL overrides ephemeral, which is what SPEC AC-08 requires.
  const setTheme = useCallback((next: Theme) => {
    setThemeState(next);
    try {
      localStorage.setItem(THEME_KEY, next);
    } catch {
      /* private mode: preference just will not persist */
    }
  }, []);

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    try {
      localStorage.setItem(LANG_KEY, next);
    } catch {
      /* ignore */
    }
  }, []);

  const t = useCallback((key: Key, vars?: Vars) => translate(lang, key, vars), [lang]);

  const value = useMemo<Prefs>(
    () => ({ theme, setTheme, lang, setLang, t }),
    [theme, setTheme, lang, setLang, t],
  );

  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}

export function usePrefs(): Prefs {
  const ctx = useContext(PrefsContext);
  if (!ctx) throw new Error("usePrefs must be used inside <Providers>");
  return ctx;
}
