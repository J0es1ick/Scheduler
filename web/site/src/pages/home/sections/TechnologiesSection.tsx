import { technologyGroups } from "../content";

export function TechnologiesSection() {
  return (
    <section className="public-section public-technologies" id="technologies">
      <div className="public-container">
        <div className="public-section-heading">
          <div>
            <span className="public-kicker">Технологии</span>
            <h2>Не демонстрация интерфейса, а работающий сервис</h2>
          </div>
          <p>
            Проект включает бот, парсеры, базу, фоновые очереди, наблюдаемость и
            полноценную административную панель.
          </p>
        </div>
        <div className="public-tech-grid">
          {technologyGroups.map((group) => {
            const Icon = group.icon;
            return (
              <article key={group.title} className="public-tech-card">
                <div>
                  <Icon size={21} />
                  <h3>{group.title}</h3>
                </div>
                <ul>
                  {group.items.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}
