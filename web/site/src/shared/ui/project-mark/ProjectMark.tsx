import styles from "./ProjectMark.module.css";
export function ProjectMark() {
  return (
    <a className={styles["public-brand"]} href="/" aria-label="Scheduler — главная">
      <span className={styles["public-brand-mark"]} aria-hidden="true">
        <i />
        <i />
        <b />
      </span>
      <span>
        <strong>Scheduler</strong>
        <small>расписание вузов</small>
      </span>
    </a>
  );
}
