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
    <section className="public-final-cta">
      <div className="public-container public-final-card" data-reveal>
        <div>
          <span className="public-kicker">Начните с вашего вуза</span>
          <h2>Пусть расписание будет под рукой.</h2>
          <p>
            Выберите роль и найдите свою группу или ФИО. Следующие пары,
            личные правки и напоминания — всё в Telegram.
          </p>
        </div>
        <div className="public-final-actions">
          <a
            className="public-primary-button"
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
