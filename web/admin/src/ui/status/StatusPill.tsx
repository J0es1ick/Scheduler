import { classNames } from "../../../../shared/classNames";
import styles from "./StatusPill.module.css";
import progressStyles from "../progress/Progress.module.css";

import { AlertCircle, Check, LoaderCircle, PauseCircle, X } from "lucide-react";
import type { SourceHealth } from "../../shared/model";

export function StatusPill({
  health,
  label,
}: {
  health: SourceHealth | "success" | "failed";
  label?: string;
}) {
  const text =
    label ??
    {
      healthy: "В норме",
      running: "Обновляется",
      error: "Ошибка",
      stale: "Данные устарели",
      quarantined: "Карантин",
      empty: "Нет занятий",
      disabled: "Отключён",
      success: "Успешно",
      failed: "Ошибка",
    }[health];
  return (
    <span className={classNames(styles["status-pill"], styles[`status-${health}`])}>
      {health === "running" ? (
        <LoaderCircle size={13} className={progressStyles["spin"]} />
      ) : health === "disabled" ? (
        <PauseCircle size={13} />
      ) : health === "healthy" || health === "success" ? (
        <Check size={13} />
      ) : health === "stale" ||
        health === "quarantined" ||
        health === "empty" ? (
        <AlertCircle size={13} />
      ) : (
        <X size={13} />
      )}
      {text}
    </span>
  );
}
