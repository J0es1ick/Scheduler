import {
  ArrowRight,
  Bot,
  CalendarCheck2,
  Check,
  Github,
  RefreshCw,
} from "lucide-react";

interface HeroSectionProps {
  botURL: string;
  projectURL: string;
}

export function HeroSection({ botURL, projectURL }: HeroSectionProps) {
  return (
    <section className="public-hero" id="about">
      <div className="public-container public-hero-grid">
        <div className="public-hero-copy">
          <div className="public-eyebrow">
            <span />
            Открытый проект для студентов
          </div>
          <h1>
            Расписание, которое
            <em> обновляется само</em>
          </h1>
          <p className="public-lead">
            Scheduler собирает занятия с официальных сайтов вузов, проверяет
            обновления и доставляет их в Telegram — без ручного переноса пар и
            постоянной проверки страниц.
          </p>
          <div className="public-actions">
            <a
              className="public-primary-button"
              href={botURL}
              target="_blank"
              rel="noreferrer"
            >
              <Bot size={19} />
              Попробовать в Telegram
              <ArrowRight size={18} />
            </a>
            <a
              className="public-secondary-button"
              href={projectURL}
              target="_blank"
              rel="noreferrer"
            >
              <Github size={19} />
              Исходный код
            </a>
          </div>
          <div className="public-live-note">
            <span />
            Статистика загружается из работающего сервиса
          </div>
        </div>

        <div className="public-hero-product" aria-label="Пример расписания">
          <div className="public-product-top">
            <div>
              <span>Сегодня</span>
              <strong>Понедельник, 27 июля</strong>
            </div>
            <span className="public-product-status">
              <i />
              обновлено
            </span>
          </div>
          <div className="public-lesson is-current">
            <time>09:50</time>
            <div>
              <span>Лекция</span>
              <strong>Информационные технологии</strong>
              <small>Аудитория А-206 · Павлова Е.А.</small>
            </div>
          </div>
          <div className="public-lesson">
            <time>12:10</time>
            <div>
              <span>Практика</span>
              <strong>Иностранный язык</strong>
              <small>Аудитория К-401 · чётная неделя</small>
            </div>
          </div>
          <div className="public-product-footer">
            <CalendarCheck2 size={18} />
            <span>Следующее изменение бот пришлёт сам</span>
            <Check size={17} />
          </div>
          <div className="public-product-stamp" aria-hidden="true">
            <RefreshCw size={22} />
            <span>auto</span>
          </div>
        </div>
      </div>
    </section>
  );
}
