import { workflowSteps } from "../content";

export function WorkflowSection() {
  return (
    <section className="public-section public-workflow" id="how-it-works">
      <div className="public-container">
        <div className="public-section-heading">
          <div>
            <span className="public-kicker">Как это устроено</span>
            <h2>От страницы вуза до сообщения в Telegram</h2>
          </div>
          <p>
            Обновление проходит контролируемый путь. Ошибка на сайте источника
            не должна превращаться в пустое расписание у всех студентов.
          </p>
        </div>
        <div className="public-workflow-grid">
          {workflowSteps.map((item) => {
            const Icon = item.icon;
            return (
              <article key={item.number} className="public-workflow-card">
                <div className="public-workflow-card-top">
                  <span>{item.number}</span>
                  <Icon size={22} />
                </div>
                <h3>{item.title}</h3>
                <p>{item.text}</p>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}
