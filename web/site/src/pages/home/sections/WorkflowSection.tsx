export function WorkflowSection() {
  return (
    <section className="public-section public-workflow" id="how-it-works">
      <div className="public-container">
        <div className="public-section-heading">
          <div>
            <span className="public-kicker">Начать пользоваться</span>
            <h2>Пара минут на настройку.</h2>
          </div>
          <p>
            Дальше расписание всегда под рукой: на сегодня, неделю или выбранную
            дату.
          </p>
        </div>
        <ol className="public-workflow-grid">
          {[
            {
              title: "Выберите роль и вуз",
              text: "Студент или преподаватель — бот предложит подходящий способ найти расписание.",
            },
            {
              title: "Найдите своё расписание",
              text: "Студент выбирает учебную группу. Преподаватель находит и подтверждает своё ФИО.",
            },
            {
              title: "Настройте сообщения",
              text: "Выберите время ежедневного расписания. Напоминания, изменения и новости сервиса включаются отдельно.",
            },
          ].map((step, index) => (
            <li key={step.title}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div>
                <h3>{step.title}</h3>
                <p>{step.text}</p>
              </div>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}
