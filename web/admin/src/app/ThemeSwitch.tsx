import { Monitor, Moon, Sun } from "lucide-react";
import type { ThemeController } from "./useTheme";

export function ThemeSwitch({
  theme,
  preference,
  setPreference,
}: ThemeController) {
  return (
    <div className="theme-control" role="group" aria-label="Оформление">
      <div className="theme-control-caption">
        <strong>{theme === "dark" ? "Тёмная тема" : "Светлая тема"}</strong>
        <span>
          {preference === "system" ? "Как на устройстве" : "Выбрана вручную"}
        </span>
      </div>
      <button
        type="button"
        className="theme-switch"
        role="switch"
        aria-label="Тёмная тема"
        aria-checked={theme === "dark"}
        title={
          theme === "dark" ? "Включить светлую тему" : "Включить тёмную тему"
        }
        onClick={() => setPreference(theme === "dark" ? "light" : "dark")}
      >
        <span className="theme-switch-track" aria-hidden="true">
          <Sun size={14} />
          <Moon size={14} />
          <span className="theme-switch-thumb" />
        </span>
      </button>
      <button
        type="button"
        className="theme-device-button"
        aria-label="Использовать тему устройства"
        aria-pressed={preference === "system"}
        title="Использовать тему устройства"
        onClick={() => setPreference("system")}
      >
        <Monitor size={17} aria-hidden="true" />
      </button>
    </div>
  );
}
