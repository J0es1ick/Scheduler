import { classNames } from "../../../../../../shared/classNames";
import styles from "./CallToActionSection.module.css";
import sectionStyles from "../../../../shared/ui/section/Section.module.css";
import linkStyles from "../../../../shared/ui/link/Link.module.css";
import { ArrowUpRight, Bot, Users } from "lucide-react";

const formatNumber = new Intl.NumberFormat("ru-RU");

interface CallToActionSectionProps {
  botURL: string;
  users?: number;
}

export function CallToActionSection({
  botURL,
  users,
}: CallToActionSectionProps) {
  return (
    <section className={styles["public-final-cta"]}>
      <div className={classNames(sectionStyles["public-container"], styles["public-final-card"])} data-reveal>
        <div>
          <span className={sectionStyles["public-kicker"]}>Начните с вашего вуза</span>
          <h2>Пусть расписание будет под рукой.</h2>
          <p>
            Выберите роль и найдите свою группу или ФИО. Следующие пары,
            личные правки и напоминания — всё в Telegram.
          </p>
        </div>
        <div className={styles["public-final-actions"]}>
          <a
            className={linkStyles["public-primary-button"]}
            href={botURL}
            target="_blank"
            rel="noreferrer"
          >
            <Bot size={19} />
            Запустить Scheduler
            <ArrowUpRight size={18} />
          </a>
          <span>
            <Users size={17} />
            Уже используют:{" "}
            {users === undefined ? "считаем…" : formatNumber.format(users)}
          </span>
        </div>
      </div>
    </section>
  );
}
