export interface AdminIdentity {
  id: string;
  name: string;
  auth_method: string;
  csrf_token: string;
  role: AdminRole;
}

export type AdminRole =
  | "none"
  | "read_only"
  | "support"
  | "editor"
  | "reviewer"
  | "operator"
  | "owner";

export interface DashboardStats {
  universities: number;
  groups: number;
  lessons: number;
  users: number;
  subscriptions: number;
  success_rate: number;
}

export type SourceHealth =
  | "healthy"
  | "running"
  | "error"
  | "stale"
  | "quarantined"
  | "empty"
  | "disabled";

export interface SourceView {
  id: string;
  university_id: string;
  university_name: string;
  university_full_name: string;
  schedule_url: string;
  adapter_type: string;
  lifecycle_status: string;
  archived_at: string | null;
  allow_empty: boolean;
  insecure_transport: boolean;
  is_enabled: boolean;
  update_interval: number;
  last_run_at: string | null;
  last_success_at: string | null;
  next_run_at: string | null;
  last_error: string;
  consecutive_failures: number;
  next_retry_at: string | null;
  current_snapshot_id: string;
  quarantined_count: number;
  latest_status: string;
  latest_started_at: string | null;
  latest_finished_at: string | null;
  latest_records: number;
  group_count: number;
  lesson_count: number;
  diagnostic_id: string;
  diagnostic_category: string;
  diagnostic_summary: string;
  diagnostic_group_id: string;
  diagnostic_http_status: number;
  diagnostic_content_type: string;
  diagnostic_response_size: number;
  diagnostic_response_sha256: string;
  diagnostic_response_preview: string;
  diagnostic_occurrences: number;
  diagnostic_created_at: string | null;
  running: boolean;
  health: SourceHealth;
}

export interface ParseLogView {
  id: string;
  data_source_id: string;
  university_name: string;
  started_at: string;
  finished_at: string | null;
  status: "running" | "success" | "failed" | "quarantined";
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
  operations: OperationalHealth;
}

export interface SnapshotAnomaly {
  code: string;
  message: string;
  current?: number;
  candidate?: number;
  ratio?: number;
}

export interface ParserSnapshot {
  id: string;
  data_source_id: string;
  parse_log_id: string;
  status: "staged" | "quarantined" | "approved" | "published" | "rejected";
  publishable: boolean;
  group_count: number;
  lesson_count: number;
  anomaly_reasons: SnapshotAnomaly[];
  reviewed_by: string;
  review_note: string;
  created_at: string;
  published_at: string | null;
  reviewed_at: string | null;
}

export type SnapshotGroupStatus =
  | "added"
  | "removed"
  | "changed"
  | "unchanged";

export interface SnapshotComparisonSummary {
  added_groups: number;
  removed_groups: number;
  changed_groups: number;
  unchanged_groups: number;
  added_lessons: number;
  removed_lessons: number;
}

export interface SnapshotGroupDiff {
  id: string;
  current_id: string;
  candidate_id: string;
  name: string;
  status: SnapshotGroupStatus;
  current_lessons: number;
  candidate_lessons: number;
  added_lessons: number;
  removed_lessons: number;
}

export interface SnapshotPreview {
  snapshot_id: string;
  data_source_id: string;
  status: ParserSnapshot["status"];
  publishable: boolean;
  created_at: string;
  candidate_start_date: string;
  candidate_end_date: string;
  candidate_group_count: number;
  candidate_lesson_count: number;
  current_snapshot_id: string;
  current_created_at: string | null;
  current_group_count: number;
  current_lesson_count: number;
  comparison_available: boolean;
  summary: SnapshotComparisonSummary;
  groups: SnapshotGroupDiff[];
}

export interface SnapshotLesson {
  id: string;
  day_of_week: number;
  special_date: string | null;
  time_start: string;
  time_end: string;
  week_type: "every" | "odd" | "even" | "date";
  subject: string;
  type: string;
  teacher: string;
  room: string;
  subgroup: number;
  valid_from: string | null;
  valid_to: string | null;
  diff: "added" | "removed" | "unchanged";
}

export interface SnapshotScheduleComparison {
  snapshot_id: string;
  group_id: string;
  group_name: string;
  status: SnapshotGroupStatus;
  comparison_available: boolean;
  current: SnapshotLesson[];
  candidate: SnapshotLesson[];
}

export interface OperationalHealth {
  status: "healthy" | "degraded";
  database: boolean;
  sources_total: number;
  sources_healthy: number;
  sources_running: number;
  sources_stale: number;
  sources_error: number;
  sources_quarantined: number;
  sources_disabled: number;
  pending_notifications: number;
  failed_notifications: number;
  pending_outbox: number;
  failed_outbox: number;
  pending_connector_runs: number;
  failed_connector_runs: number;
  database_bytes: number;
  connector_payload_bytes: number;
  snapshot_payload_bytes: number;
  oldest_pending_seconds: number;
  last_successful_parse_at: string | null;
  checked_at: string;
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
  admin_role: AdminRole;
  subscriptions: number;
  default_group_id: string;
  default_group_name: string;
  notifications_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export type ConnectorStatus =
  | "draft"
  | "testing"
  | "pending_review"
  | "active"
  | "suspended"
  | "archived";

export type IntegrationMode =
  | "managed_parser"
  | "declarative_pull"
  | "external_push";

export interface ConnectorClient {
  id: string;
  data_source_id: string;
  university_id: string;
  university_name: string;
  display_name: string;
  integration_mode: IntegrationMode;
  parser_id: string;
  description: string;
  maintainer_name: string;
  maintainer_url: string;
  key_id: string;
  status: ConnectorStatus;
  rate_limit_per_minute: number;
  max_payload_bytes: number;
  last_seen_at: string | null;
  last_snapshot_at: string | null;
  created_by: string;
  created_at: string;
  updated_at: string;
  quality_policy: SourceQualityPolicy;
}

export interface ManagedParserManifest {
  contract_version: string;
  parser_id: string;
  version: string;
  display_name: string;
  description: string;
  institution: {
    external_id: string;
    name: string;
    full_name?: string;
    schedule_url?: string;
    timezone: string;
    locale?: string;
  };
  maintainer_name?: string;
  maintainer_url?: string;
  update_interval: number;
}

export interface ManagedParserCatalogItem {
  manifest: ManagedParserManifest;
  connected: boolean;
  connector_id?: string;
  status?: ConnectorStatus;
}

export interface ConnectorRun {
  run_id: string;
  connector_id: string;
  external_snapshot_id: string;
  schema_version: string;
  payload_sha256: string;
  status: "received" | "processing" | "staged" | "quarantined" | "published" | "rejected" | "failed";
  attempts: number;
  error?: string;
  parser_snapshot_id?: string;
  group_count: number;
  lesson_count: number;
  received_at: string;
  completed_at: string | null;
}

export interface ConnectorCredentials {
  connector_id: string;
  key_id: string;
  private_key: string;
  submit_path: string;
}

export interface SourceQualityPolicy {
  allow_empty: boolean;
  minimum_groups: number;
  minimum_lessons: number;
  maximum_group_drop_ratio: number;
  maximum_group_growth_ratio: number;
  maximum_lesson_drop_ratio: number;
  maximum_lesson_growth_ratio: number;
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
