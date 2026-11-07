import { classNames } from "../../../../../../shared/classNames";
import sectionStyles from "../../../../shared/ui/section/Section.module.css";
import styles from "./WorkflowSection.module.css";
export function WorkflowSection() {
  return (
    <section className={classNames(sectionStyles["public-section"], styles["public-workflow"])} id="how-it-works">
      <div className={sectionStyles["public-container"]}>
        <div className={sectionStyles["public-section-heading"]} data-reveal>
          <div>
            <span className={sectionStyles["public-kicker"]}>Начать пользоваться</span>
            <h2>Пара минут на настройку.</h2>
          </div>
          <p>
            Дальше расписание всегда под рукой: на сегодня, неделю или выбранную
            дату.
          </p>
        </div>
        <ol className={styles["public-workflow-grid"]}>
          {[
            {
              title: "Выберите роль и вуз",
              text: "Студент или преподаватель — бот предложит подходящий способ найти расписание.",
            },
            {
              title: "Найдите своё расписание",
              text: "Студент выбирает учебную группу. Преподаватель находит и подтверждает своё ФИО.",
            },
            {
              title: "Настройте сообщения",
              text: "Выберите время ежедневного расписания. Напоминания, изменения и новости сервиса включаются отдельно.",
            },
          ].map((step, index) => (
            <li key={step.title} data-surface-light data-reveal data-reveal-delay={index * 60}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div>
                <h3>{step.title}</h3>
                <p>{step.text}</p>
              </div>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}
