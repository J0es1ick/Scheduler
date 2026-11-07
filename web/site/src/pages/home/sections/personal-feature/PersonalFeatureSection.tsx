import { classNames } from "../../../../../../shared/classNames";
import sectionStyles from "../../../../shared/ui/section/Section.module.css";
import styles from "./PersonalFeatureSection.module.css";
import linkStyles from "../../../../shared/ui/link/Link.module.css";
import { ArrowUpRight, Check, MapPin, PencilLine } from "lucide-react";

export function PersonalFeatureSection({ botURL }: { botURL: string }) {
  return (
    <section className={classNames(sectionStyles["public-section"], styles["public-personal-feature"])}>
      <div className={classNames(sectionStyles["public-container"], styles["public-personal-grid"])}>
        <div
          className={styles["public-personal-preview"]}
          data-reveal
          data-surface-light
          aria-label="Пример личной правки"
        >
          <div className={styles["public-personal-caption"]}>
            <PencilLine size={18} />
            <span>Ваши правки</span>
            <small>Пример</small>
          </div>
          <div className={styles["public-personal-lesson"]}>
            <span>09:50 — 11:20 · Лекция</span>
            <strong>Информационные технологии</strong>
            <p>Павлова Е.А.</p>
          </div>
          <div className={styles["public-room-change"]}>
            <MapPin size={18} />
            <span>Аудитория</span>
            <s>А-206</s>
            <strong>А-101</strong>
          </div>
          <div className={styles["public-change-result"]}>
            <Check size={16} /> Изменено только для вас
          </div>
        </div>
        <div className={styles["public-personal-copy"]} data-reveal>
          <span className={sectionStyles["public-kicker"]}>Личное пространство</span>
          <h2>Общее расписание.<br /><span>Ваши детали.</span></h2>
          <p>
            Другая аудитория, замена преподавателя или отмена пары — настройте
            расписание под себя в Mini App.
          </p>
          <ul>
            <li><Check size={16} /> На одну дату или подтверждённые повторы</li>
            <li><Check size={16} /> Те же правки в боте и напоминаниях</li>
            <li><Check size={16} /> Расписание остальных остаётся прежним</li>
          </ul>
          <a className={linkStyles["public-text-link"]} href={botURL} target="_blank" rel="noreferrer">
            Открыть через бота <ArrowUpRight size={18} />
          </a>
        </div>
      </div>
    </section>
  );
}
