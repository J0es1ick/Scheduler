export interface PublicInfo {
  universities: number;
  groups: number;
  lessons: number;
  users: number;
  subscriptions: number;
  university_names: string[];
  sources: PublicSourceStatus[];
  project_url: string;
  bot_url: string;
  updated_at: string;
}

export interface PublicSourceStatus {
  university_name: string;
  schedule_url: string;
  state: "current" | "stale" | "error" | "disabled";
  secure: boolean;
  last_success_at: string | null;
}
