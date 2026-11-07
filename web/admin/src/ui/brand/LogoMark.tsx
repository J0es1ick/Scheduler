import { classNames } from "../../../../shared/classNames";
import styles from "./LogoMark.module.css";
import stateStyles from "../state/State.module.css";

export function LogoMark({ compact = false }: { compact?: boolean }) {
  return (
    <div
      className={classNames(styles["logo-lockup"], compact ? stateStyles["is-compact"] : undefined)}
      aria-label="Scheduler Admin"
    >
      <div className={styles["logo-mark"]} aria-hidden="true">
        <span className={classNames(styles["logo-mark-line"], styles["line-a"])} />
        <span className={classNames(styles["logo-mark-line"], styles["line-b"])} />
        <span className={styles["logo-mark-dot"]} />
      </div>
      {!compact && (
        <div>
          <strong>Scheduler</strong>
          <span>админ-панель</span>
        </div>
      )}
    </div>
  );
}
