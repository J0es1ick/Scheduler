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
    <section className="public-hero" id="about">
      <div className="public-container public-hero-grid">
        <div className="public-hero-copy">
          <p className="public-kicker">Расписание вузов в Telegram</p>
          <h1>
            Знайте, какие
            <br />
            пары <span>сегодня.</span>
          </h1>
          <p className="public-lead">
            Для студентов — расписание группы. Для преподавателей — свои
            занятия. С изменениями, напоминаниями и утренним сообщением в
            удобное время.
          </p>
          <div className="public-actions">
            <a
              className="public-primary-button"
              href={botURL}
              target="_blank"
              rel="noreferrer"
            >
              Открыть Scheduler <ArrowUpRight size={19} />
            </a>
            <a
              className="public-secondary-button"
              href={projectURL}
              target="_blank"
              rel="noreferrer"
            >
              <Github size={18} /> Код проекта
            </a>
          </div>
          <p className="public-hero-note">
            <Check size={15} /> Вы выбираете, какие сообщения получать
          </p>
        </div>
        <div className="public-hero-product" aria-label="Пример расписания">
          <div className="public-product-heading">
            <span>Внутри бота</span>
            <small>Пример</small>
          </div>
          <div
            className="public-example-tabs"
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
          <div className="public-product-date">
            <strong>Понедельник</strong>
            <span>
              {role === "student" ? "Группа ИВТ-21" : "Павлова Елена Андреевна"}
            </span>
          </div>
          <div className="public-example-lessons" aria-live="polite">
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
              <article className="public-lesson" key={lesson.time}>
                <div className="public-lesson-time">
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
          <div className="public-product-footer">
            <Bell size={16} />
            <span>Каждый день в 07:00 — если захотите</span>
          </div>
        </div>
      </div>
    </section>
  );
}
