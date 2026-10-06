import { ArrowUpRight } from "lucide-react";
export function UniversitiesSection({
  botURL,
  universities,
}: {
  botURL: string;
  universities: string[];
}) {
  return (
    <section className="public-section public-universities" id="universities">
      <div className="public-container public-universities-grid">
        <div>
          <span className="public-kicker">Доступные вузы</span>
          <h2>
            Ваш вуз.
            <br />
            Ваше расписание.
          </h2>
          <p>
            Расписание загружается из официальных источников. Доступные группы и
            преподавателей можно найти в боте.
          </p>
          <a
            className="public-text-link"
            href={botURL}
            target="_blank"
            rel="noreferrer"
          >
            Найти своё расписание <ArrowUpRight size={17} />
          </a>
        </div>
        <div className="public-university-list">
          {universities.length ? (
            universities.map((name, index) => (
              <div key={name}>
                <span>{String(index + 1).padStart(2, "0")}</span>
                <strong>{name}</strong>
              </div>
            ))
          ) : (
            <p className="public-empty-state">
              Список вузов пока недоступен. Его можно проверить в боте.
            </p>
          )}
          <p>
            Нет вашего вуза? Предложите его через раздел «Горячая линия» в боте.
          </p>
        </div>
      </div>
    </section>
  );
}
