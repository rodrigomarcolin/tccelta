"use client";

import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import { THEME_COOKIE, type Theme } from "@/config";
import { IconMoon, IconSun } from "@/components/icons/Icons";

const ThemeContext = createContext<Theme>("dark");

export function useTheme(): Theme {
  return useContext(ThemeContext);
}

export function saveTheme(theme: Theme, doc: Document = document): void {
  doc.documentElement.dataset.theme = theme;
  doc.cookie = `${THEME_COOKIE}=${theme}; path=/; max-age=31536000; samesite=lax`;
}

/** The initial theme comes from the cookie read on the server, to avoid flashing. */
export function ThemeProvider({ initial, children }: { initial: Theme; children: ReactNode }) {
  const [value, setTheme] = useState<Theme>(initial);
  const toggle = useCallback(
    (next: Theme) => {
      saveTheme(next);
      setTheme(next);
    },
    [setTheme],
  );
  return (
    <ThemeContext.Provider value={value}>
      <ToggleContext.Provider value={toggle}>{children}</ToggleContext.Provider>
    </ThemeContext.Provider>
  );
}

const ToggleContext = createContext<(t: Theme) => void>(() => undefined);

export function ThemeToggle() {
  const theme = useTheme();
  const toggle = useContext(ToggleContext);
  const isLight = theme === "light";
  return (
    <button
      type="button"
      className="button button-secondary button-icon"
      aria-pressed={isLight}
      aria-label="Tema claro"
      title={isLight ? "Voltar ao tema escuro" : "Usar tema claro"}
      onClick={() => toggle(isLight ? "dark" : "light")}
    >
      {isLight ? <IconSun /> : <IconMoon />}
      <span className="theme-button-text">Tema claro</span>
    </button>
  );
}
