import {
  BellRing,
  Braces,
  Database,
  Search,
  ShieldCheck,
  Waypoints,
  type LucideIcon,
} from "lucide-react";

export interface HomeContentGroup {
  title: string;
  icon: LucideIcon;
  items: string[];
}

export interface WorkflowStep {
  number: string;
  title: string;
  text: string;
  icon: LucideIcon;
}

export const technologyGroups: HomeContentGroup[] = [
  {
    title: "Сервис и парсеры",
    icon: Braces,
    items: ["Go", "Telegram Bot API", "pgx", "HTML-парсеры"],
  },
  {
    title: "Данные",
    icon: Database,
    items: [
      "PostgreSQL 16",
      "Миграции",
      "Очереди уведомлений",
      "Снимки расписаний",
    ],
  },
  {
    title: "Веб-приложение",
    icon: Waypoints,
    items: ["React 19", "TypeScript", "Vite", "Mini App"],
  },
  {
    title: "Эксплуатация",
    icon: ShieldCheck,
    items: ["Docker Compose", "Health checks", "Prometheus", "Аудит действий"],
  },
];

export const workflowSteps: WorkflowStep[] = [
  {
    number: "01",
    title: "Собираем",
    text: "Адаптеры регулярно читают официальные страницы расписаний подключённых учебных заведений.",
    icon: Search,
  },
  {
    number: "02",
    title: "Проверяем",
    text: "Новая версия сравнивается с текущей. Подозрительные изменения не попадают к пользователям автоматически.",
    icon: ShieldCheck,
  },
  {
    number: "03",
    title: "Доставляем",
    text: "Бот показывает актуальные пары и уведомляет подписчиков, когда расписание их группы изменилось.",
    icon: BellRing,
  },
];
