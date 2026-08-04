import { ArrowUpRight, GraduationCap, Waypoints } from "lucide-react";

interface UniversitiesSectionProps {
  botURL: string;
  universities: string[];
}

const fallbackUniversities = ["ИГХТУ", "ИГЭУ"];

export function UniversitiesSection({
  botURL,
  universities,
}: UniversitiesSectionProps) {
  const names = universities.length ? universities : fallbackUniversities;

  return (
    <section className="public-section public-universities">
      <div className="public-container public-universities-grid">
        <div className="public-university-copy">
          <span className="public-kicker">Подключённые источники</span>
          <h2>Один бот для расписаний разных вузов</h2>
          <p>
            Новый вуз подключается как отдельный адаптер, а пользователю не
            нужно менять привычный способ работы с ботом.
          </p>
          <a href={botURL} target="_blank" rel="noreferrer">
            Проверить доступные группы
            <ArrowUpRight size={17} />
          </a>
        </div>
        <div className="public-university-list">
          {names.map((name, index) => (
            <div key={name}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <GraduationCap size={21} />
              <strong>{name}</strong>
              <small>официальный источник</small>
            </div>
          ))}
          <div className="is-next">
            <span>+</span>
            <Waypoints size={21} />
            <strong>Следующий вуз</strong>
            <small>предложить через горячую линию</small>
          </div>
        </div>
      </div>
    </section>
  );
}
