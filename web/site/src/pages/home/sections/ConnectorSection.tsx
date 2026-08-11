import {
  ArrowUpRight,
  Braces,
  CloudUpload,
  FileJson,
  ServerCog,
  ShieldCheck,
} from "lucide-react";

interface ConnectorSectionProps {
  projectURL: string;
}

const steps = [
  {
    number: "01",
    title: "Передайте парсер",
    text: "Основной путь: реализуйте компактный контракт и отправьте код. Scheduler сам будет запускать и наблюдать его.",
    icon: ServerCog,
  },
  {
    number: "02",
    title: "Укажите JSON URL",
    text: "Если источник уже отдаёт Snapshot v1, Scheduler может безопасно забирать его по публичному HTTPS адресу.",
    icon: FileJson,
  },
  {
    number: "03",
    title: "Размещайте сами",
    text: "Для независимого размещения остаётся подписанный Connector API с Go и Python SDK.",
    icon: CloudUpload,
  },
  {
    number: "04",
    title: "Единая проверка",
    text: "Независимо от способа первый снимок проверяется человеком, а аномальные изменения уходят в карантин.",
    icon: ShieldCheck,
  },
];

export function ConnectorSection({ projectURL }: ConnectorSectionProps) {
  const repository = projectURL.replace(/\/$/, "");
  const docsURL = `${repository}/blob/master/docs/managed-parsers.md`;
  const connectorDocsURL = `${repository}/blob/master/docs/connector-api.md`;
  const schemaURL = `${repository}/blob/master/docs/schema/schedule-snapshot-v1.json`;
  const goURL = `${repository}/tree/master/connector/v1`;
  const pythonURL = `${repository}/tree/master/sdk/python`;
  const parserURL = `${repository}/tree/master/parser/v1`;
  const exampleURL = `${repository}/tree/master/integrations/ivgpu`;

  return (
    <section className="public-section public-connectors" id="connectors">
      <div className="public-container">
        <div className="public-section-heading">
          <div>
            <span className="public-kicker">Открытые интеграции</span>
            <h2>Напишите парсер — его запуск возьмёт на себя Scheduler</h2>
          </div>
          <p>
            Для основного сценария не нужны VPS, Docker или доступ к базе.
            Передайте код по открытому контракту — Scheduler обеспечит
            расписание запусков, повторы, снимки, карантин и публикацию.
          </p>
        </div>

        <div className="public-connector-steps">
          {steps.map((step) => {
            const Icon = step.icon;
            return (
              <article key={step.number}>
                <header>
                  <span>{step.number}</span>
                  <Icon size={20} />
                </header>
                <h3>{step.title}</h3>
                <p>{step.text}</p>
              </article>
            );
          })}
        </div>

        <div className="public-connector-kit">
          <div>
            <span className="public-kicker">Быстрый старт</span>
            <h3>Начните с управляемого парсера</h3>
            <p>
              Парсер отвечает только за особенности конкретного сайта. Если
              передать код нельзя, остаются JSON pull и внешний подписанный
              Connector API.
            </p>
            <div className="public-connector-links">
              <a href={docsURL} target="_blank" rel="noreferrer">
                Как написать парсер <ArrowUpRight size={15} />
              </a>
              <a href={connectorDocsURL} target="_blank" rel="noreferrer">
                Внешний Connector API <ArrowUpRight size={15} />
              </a>
              <a href={schemaURL} target="_blank" rel="noreferrer">
                JSON Schema <ArrowUpRight size={15} />
              </a>
            </div>
          </div>
          <div className="public-sdk-options">
            <a href={parserURL} target="_blank" rel="noreferrer">
              <Braces size={18} />
              <span>
                <strong>Parser SDK v1</strong>
                <small>Три метода для управляемого парсера</small>
              </span>
              <ArrowUpRight size={15} />
            </a>
            <a href={goURL} target="_blank" rel="noreferrer">
              <Braces size={18} />
              <span>
                <strong>Go SDK</strong>
                <small>Типизированные модели и клиент</small>
              </span>
              <ArrowUpRight size={15} />
            </a>
            <a href={pythonURL} target="_blank" rel="noreferrer">
              <Braces size={18} />
              <span>
                <strong>Python SDK</strong>
                <small>Удобен для существующих парсеров</small>
              </span>
              <ArrowUpRight size={15} />
            </a>
            <a href={exampleURL} target="_blank" rel="noreferrer">
              <Braces size={18} />
              <span>
                <strong>Пример ИВГПУ</strong>
                <small>Рабочий парсер без собственного контейнера</small>
              </span>
              <ArrowUpRight size={15} />
            </a>
            <pre>
              <code>FetchGroups(ctx){"\n"}FetchSchedule(ctx, groupID)</code>
            </pre>
          </div>
        </div>
      </div>
    </section>
  );
}
