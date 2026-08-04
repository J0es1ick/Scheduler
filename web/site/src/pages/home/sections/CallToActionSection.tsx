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
      <div className="public-container public-final-card">
        <div>
          <span className="public-kicker">Готово к проверке</span>
          <h2>Откройте бота и выберите свою группу</h2>
          <p>
            Расписание, подписки и уведомления доступны прямо в Telegram. Если
            вашего учебного заведения пока нет, его можно предложить через
            горячую линию.
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
            Уже используют: {users === undefined ? "считаем…" : formatNumber.format(users)}
          </span>
        </div>
      </div>
    </section>
  );
}
