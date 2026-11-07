import { classNames } from "../../../../../../shared/classNames";
import styles from "./HeroSection.module.css";
import sectionStyles from "../../../../shared/ui/section/Section.module.css";
import linkStyles from "../../../../shared/ui/link/Link.module.css";
import { useState } from "react";
import { ArrowUpRight, Bell, Check, Github } from "lucide-react";

export function HeroSection({
  botURL,
  projectURL,
}: {
  botURL: string;
  projectURL: string;
}) {
  const [role, setRole] = useState<"student" | "teacher">("student");
  return (
    <section className={styles["public-hero"]} id="about">
      <div className={classNames(sectionStyles["public-container"], styles["public-hero-grid"])}>
        <div className={styles["public-hero-copy"]}>
          <p className={sectionStyles["public-kicker"]}>Расписание вузов в Telegram</p>
          <h1>
            Знайте, какие
            <br />
            пары <span>сегодня.</span>
          </h1>
          <p className={styles["public-lead"]}>
            Для студентов — расписание группы. Для преподавателей — свои
            занятия. С изменениями, напоминаниями и утренним сообщением в
            удобное время.
          </p>
          <div className={styles["public-actions"]}>
            <a
              className={linkStyles["public-primary-button"]}
              href={botURL}
              target="_blank"
              rel="noreferrer"
            >
              Открыть Scheduler <ArrowUpRight size={19} />
            </a>
            <a
              className={linkStyles["public-secondary-button"]}
              href={projectURL}
              target="_blank"
              rel="noreferrer"
            >
              <Github size={18} /> Код проекта
            </a>
          </div>
          <p className={styles["public-hero-note"]}>
            <Check size={15} /> Вы выбираете, какие сообщения получать
          </p>
        </div>
        <div className={styles["public-product-stage"]}>
          <div className={styles["public-stage-label"]}>
            <span>На каждый день</span>
            <span>В одном сообщении</span>
          </div>
          <div className={styles["public-hero-product"]} aria-label="Пример расписания" data-surface-light>
            <div className={styles["public-product-heading"]}>
              <span>Внутри бота</span>
              <small>Пример</small>
            </div>
            <div
              className={styles["public-example-tabs"]}
              data-role={role}
              role="group"
              aria-label="Пример для роли"
            >
              <button
                type="button"
                aria-pressed={role === "student"}
                onClick={() => setRole("student")}
              >
                Студент
              </button>
              <button
                type="button"
                aria-pressed={role === "teacher"}
                onClick={() => setRole("teacher")}
              >
                Преподаватель
              </button>
            </div>
            <div className={styles["public-example-content"]} aria-live="polite">
              <div key={role} className={styles["public-role-content"]}>
                <div className={styles["public-product-date"]}>
                  <strong>Понедельник</strong>
                  <span>
                    {role === "student" ? "Группа ИВТ-21" : "Павлова Елена Андреевна"}
                  </span>
                </div>
                <div className={styles["public-example-lessons"]}>
                  {[
                    {
                      time: "09:50",
                      end: "11:20",
                      title: "Информационные технологии",
                      type: "Лекция",
                      room: "А-206",
                      teacher: "Павлова Е.А.",
                      group: "ИВТ-21, ИВТ-22",
                    },
                    {
                      time: "12:10",
                      end: "13:40",
                      title: role === "student" ? "Иностранный язык" : "Базы данных",
                      type: "Практика",
                      room: "К-401",
                      teacher: "Смирнов А.В.",
                      group: "ИВТ-31",
                    },
                  ].map((lesson) => (
                    <article className={styles["public-lesson"]} key={lesson.time}>
                      <div className={styles["public-lesson-time"]}>
                        <time>{lesson.time}</time>
                        <small>{lesson.end}</small>
                      </div>
                      <div>
                        <span>{lesson.type}</span>
                        <h3>{lesson.title}</h3>
                        <p>Ауд. {lesson.room}</p>
                        <small>
                          {role === "student" ? lesson.teacher : lesson.group}
                        </small>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </div>
            <div className={styles["public-product-footer"]}>
              <Bell size={16} />
              <span>Каждый день в 07:00 — если захотите</span>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
