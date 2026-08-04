import type { PublicInfo } from "../../../features/public-info/model";
import { Metric } from "../../../shared/ui/Metric";

interface StatisticsSectionProps {
  info: PublicInfo | null;
  loading: boolean;
}

export function StatisticsSection({ info, loading }: StatisticsSectionProps) {
  return (
    <section className="public-stats" aria-label="Статистика проекта">
      <div className="public-container public-stats-inner">
        <div className="public-stats-title">
          <span>Сервис уже работает</span>
          <p>Показатели обновляются вместе с данными проекта.</p>
        </div>
        <Metric
          value={info?.universities}
          label="подключённых вуза"
          loading={loading}
        />
        <Metric value={info?.users} label="пользователей" loading={loading} />
        <Metric value={info?.groups} label="учебных групп" loading={loading} />
        <Metric
          value={info?.lessons}
          label="занятий в базе"
          loading={loading}
        />
      </div>
    </section>
  );
}
