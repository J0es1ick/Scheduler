import { CheckCircle2, Clock3, ExternalLink } from "lucide-react";
import type { PublicSourceStatus } from "../../../features/public-info/model";

const stateText: Record<PublicSourceStatus["state"], string> = {
  current: "данные актуальны",
  stale: "обновление задерживается",
  error: "источник временно недоступен",
  disabled: "обновление приостановлено",
};

export function StatusSection({ sources }: { sources: PublicSourceStatus[] }) {
  if (!sources.length) return null;
  return (
    <section className="public-section public-status" id="status">
      <div className="public-container">
        <div className="public-section-heading">
          <div>
            <span className="public-kicker">Состояние данных</span>
            <h2>Понятно, откуда взялось расписание</h2>
          </div>
          <p>
            Для каждого подключённого источника показаны время последнего
            успешного обновления и текущее состояние.
          </p>
        </div>
        <div className="public-status-list">
          {sources.map((source) => (
            <article
              key={source.university_name}
              className={`is-${source.state}`}
            >
              <span className="public-status-icon">
                {source.state === "current" ? (
                  <CheckCircle2 size={20} />
                ) : (
                  <Clock3 size={20} />
                )}
              </span>
              <div>
                <h3>{source.university_name}</h3>
                <p>{stateText[source.state]}</p>
              </div>
              <time>
                {source.last_success_at
                  ? new Intl.DateTimeFormat("ru-RU", {
                      dateStyle: "medium",
                      timeStyle: "short",
                    }).format(new Date(source.last_success_at))
                  : "успешных обновлений ещё не было"}
              </time>
              {source.schedule_url && (
                <a
                  href={source.schedule_url}
                  target="_blank"
                  rel="noreferrer"
                  aria-label={`Открыть источник ${source.university_name}`}
                >
                  <ExternalLink size={17} />
                </a>
              )}
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}
