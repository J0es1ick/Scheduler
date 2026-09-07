import { useEffect, useLayoutEffect, useState } from "react";

export type Theme = "light" | "dark";
export type ThemePreference = Theme | "system";
export interface ThemeController {
  theme: Theme;
  preference: ThemePreference;
  setPreference: (preference: ThemePreference) => void;
}

const storageKey = "scheduler.admin.theme";

function readPreference(): ThemePreference {
  try {
    const stored = window.localStorage.getItem(storageKey);
    if (stored === "light" || stored === "dark") return stored;
  } catch {
    return "system";
  }
  return "system";
}

function deviceTheme(): Theme {
  const app = window.Telegram?.WebApp;
  if (
    app?.initData &&
    (app.colorScheme === "light" || app.colorScheme === "dark")
  ) {
    return app.colorScheme;
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export function useTheme(): ThemeController {
  const [preference, setStoredPreference] =
    useState<ThemePreference>(readPreference);
  const [systemTheme, setSystemTheme] = useState<Theme>(deviceTheme);
  const theme = preference === "system" ? systemTheme : preference;

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const app = window.Telegram?.WebApp;
    const updateDevice = () => setSystemTheme(deviceTheme());
    const updateStorage = (event: StorageEvent) => {
      if (event.key === storageKey || event.key === null)
        setStoredPreference(readPreference());
    };
    media.addEventListener("change", updateDevice);
    app?.onEvent?.("themeChanged", updateDevice);
    window.addEventListener("storage", updateStorage);
    return () => {
      media.removeEventListener("change", updateDevice);
      app?.offEvent?.("themeChanged", updateDevice);
      window.removeEventListener("storage", updateStorage);
    };
  }, []);

  useLayoutEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = theme;
    const colors = getComputedStyle(root);
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute("content", colors.getPropertyValue("--paper").trim());
    const app = window.Telegram?.WebApp;
    if (app?.initData) {
      app.setHeaderColor?.(colors.getPropertyValue("--paper-deep").trim());
      app.setBackgroundColor?.(colors.getPropertyValue("--paper").trim());
    }
  }, [theme]);

  function setPreference(next: ThemePreference) {
    setStoredPreference(next);
    try {
      if (next === "system") window.localStorage.removeItem(storageKey);
      else window.localStorage.setItem(storageKey, next);
    } catch {
      return;
    }
  }

  return { theme, preference, setPreference };
}
