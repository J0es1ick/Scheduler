import { technologyGroups } from "../content";

export function TechnologiesSection() {
  return (
    <section className="public-section public-technologies" id="technologies">
      <div className="public-container">
        <details>
          <summary>На чём работает Scheduler</summary>
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
        </details>
      </div>
    </section>
  );
}
