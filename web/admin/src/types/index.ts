export interface AdminIdentity {
  id: string;
  name: string;
  auth_method: string;
  csrf_token: string;
}

export interface DashboardStats {
  universities: number;
  groups: number;
  lessons: number;
  users: number;
  subscriptions: number;
  success_rate: number;
}

export type SourceHealth = "healthy" | "running" | "error";

export interface SourceView {
  id: string;
  university_id: string;
  university_name: string;
  university_full_name: string;
  schedule_url: string;
  adapter_type: string;
  update_interval: number;
  last_run_at: string | null;
  next_run_at: string | null;
  last_error: string;
  latest_status: string;
  latest_started_at: string | null;
  latest_finished_at: string | null;
  latest_records: number;
  group_count: number;
  lesson_count: number;
  running: boolean;
  health: SourceHealth;
}

export interface ParseLogView {
  id: string;
  data_source_id: string;
  university_name: string;
  started_at: string;
  finished_at: string | null;
  status: "running" | "success" | "failed";
  records_fetched: number;
  error_message: string;
  duration_ms: number;
}

export interface TrendPoint {
  date: string;
  records: number;
  success: number;
  failed: number;
}

export interface UniversityBreakdown {
  id: string;
  name: string;
  groups: number;
  lessons: number;
}

export interface Dashboard {
  stats: DashboardStats;
  sources: SourceView[];
  recent_logs: ParseLogView[];
  trend: TrendPoint[];
  universities: UniversityBreakdown[];
}

export interface UniversityOption {
  id: string;
  name: string;
  full_name: string;
  schedule_url: string;
  is_active: boolean;
}

export interface GroupView {
  id: string;
  name: string;
  university_id: string;
  university_name: string;
  is_active: boolean;
  lesson_count: number;
  updated_at: string;
}

export interface LessonView {
  id: string;
  university_name: string;
  group_id: string;
  group_name: string;
  subject: string;
  type: string;
  teacher: string;
  room: string;
  day_of_week: number;
  special_date: string | null;
  time_start: string;
  time_end: string;
  week_type: string;
  subgroup: number;
  valid_from: string | null;
  valid_to: string | null;
}

export interface EditorGroup {
  id: string;
  name: string;
  university_id: string;
  university_name: string;
  updated_at: string;
}

export interface SemesterOption {
  id: string;
  name: string;
  start_date: string;
  end_date: string;
}

export interface EditorLesson {
  id: string;
  university_id: string;
  semester_id: string;
  day_of_week: number;
  special_date: string | null;
  time_start: string;
  time_end: string;
  week_type: "every" | "odd" | "even" | "date";
  subject: string;
  type: string;
  teacher: string;
  room: string;
  group_id: string;
  subgroup: number;
  valid_from: string | null;
  valid_to: string | null;
  updated_at: string;
  origin: "parsed" | "manual";
  base_lesson_id: string | null;
  version: number;
  deleted: boolean;
}

export interface EditorSchedule {
  group: EditorGroup;
  semesters: SemesterOption[];
  lessons: EditorLesson[];
  deleted_lessons: EditorLesson[];
}

export interface LessonMutationPayload {
  group_id: string;
  semester_id: string;
  day_of_week: number;
  special_date: string;
  time_start: string;
  time_end: string;
  week_type: EditorLesson["week_type"];
  subject: string;
  type: string;
  teacher: string;
  room: string;
  subgroup: number;
  valid_from: string;
  valid_to: string;
  expected_updated_at?: string;
}

export interface UserView {
  id: string;
  username: string;
  is_admin: boolean;
  subscriptions: number;
  default_group_id: string;
  default_group_name: string;
  notifications_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SupportRequestView {
  id: string;
  user_id: string;
  username: string;
  request_type: "update_existing" | "new_institution";
  details: string;
  status: "pending" | "approved" | "rejected";
  review_note: string;
  reviewed_by: string;
  reviewed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface AuditLogView {
  id: string;
  actor_id: string;
  actor_name: string;
  action: string;
  object_type: string;
  object_id: string;
  details: Record<string, unknown>;
  ip_address: string;
  created_at: string;
}

export interface Pagination {
  page: number;
  page_size: number;
  total: number;
}

export interface Page<T> {
  items: T[];
  pagination: Pagination;
}

declare global {
  interface Window {
    Telegram?: {
      WebApp?: {
        initData: string;
        ready(): void;
        expand(): void;
        colorScheme?: "light" | "dark";
        setHeaderColor?(color: string): void;
        setBackgroundColor?(color: string): void;
      };
    };
  }
}
