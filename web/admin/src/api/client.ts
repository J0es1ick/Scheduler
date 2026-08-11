import type {
  AdminIdentity,
  AuditLogView,
  Dashboard,
  EditorSchedule,
  EditorLesson,
  GroupView,
  LessonView,
  LessonMutationPayload,
  Page,
  ParserSnapshot,
  SnapshotPreview,
  SnapshotScheduleComparison,
  ParseLogView,
  SourceView,
  SupportRequestView,
  UniversityOption,
  UserView,
  ConnectorClient,
  ConnectorCredentials,
  ConnectorRun,
  ConnectorStatus,
  IntegrationMode,
  ManagedParserCatalogItem,
  SourceQualityPolicy,
} from "../types";

export class APIError extends Error {
  status: number;
  code: string;
  requestID: string;

  constructor(status: number, message: string, code = "", requestID = "") {
    super(message);
    this.status = status;
    this.code = code;
    this.requestID = requestID;
  }
}

let csrfToken = "";

export function getCSRFToken(): string {
  return csrfToken;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const method = (options.method ?? "GET").toUpperCase();
  const headers = new Headers(options.headers);
  if (options.body) headers.set("Content-Type", "application/json");
  if (!["GET", "HEAD", "OPTIONS"].includes(method) && csrfToken) {
    headers.set("X-CSRF-Token", csrfToken);
  }

  const response = await fetch(path, {
    ...options,
    headers,
    credentials: "include",
  });
  if (response.status === 401 && !path.startsWith("/api/auth/")) {
    window.dispatchEvent(new CustomEvent("scheduler:session-expired"));
  }
  if (!response.ok) {
    let message = `Ошибка ${response.status}`;
    let code = "";
    let requestID = response.headers.get("X-Request-ID") ?? "";
    try {
      const payload = (await response.json()) as {
        error?: string;
        code?: string;
        request_id?: string;
      };
      if (payload.error) message = payload.error;
      if (payload.code) code = payload.code;
      if (payload.request_id) requestID = payload.request_id;
    } catch {
      // Апи всё ещё может возвращать пустой ответ для ошибок уровня прокси.
    }
    throw new APIError(response.status, message, code, requestID);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

function rememberUser(user: AdminIdentity): AdminIdentity {
  csrfToken = user.csrf_token;
  return user;
}

export const api = {
  authConfig: () =>
    request<{ access_key_enabled: boolean }>("/api/auth/config"),
  async me() {
    const payload = await request<{ user: AdminIdentity }>("/api/auth/me");
    return rememberUser(payload.user);
  },

  async loginWithAccessKey(accessKey: string) {
    const payload = await request<{ user: AdminIdentity }>(
      "/api/auth/access-key",
      {
        method: "POST",
        body: JSON.stringify({ access_key: accessKey }),
      },
    );
    return rememberUser(payload.user);
  },

  async loginWithTelegram(initData: string) {
    const payload = await request<{ user: AdminIdentity }>(
      "/api/auth/telegram",
      {
        method: "POST",
        body: JSON.stringify({ init_data: initData }),
      },
    );
    return rememberUser(payload.user);
  },

  async logout() {
    await request<void>("/api/auth/logout", { method: "POST" });
    csrfToken = "";
  },

  dashboard: () => request<Dashboard>("/api/dashboard"),
  sources: async () =>
    (await request<{ items: SourceView[] }>("/api/sources")).items,
  updateSource: (
    id: string,
    settings: { update_interval?: number; is_enabled?: boolean },
  ) =>
    request(`/api/sources/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(settings),
    }),
  deleteSource: (id: string) =>
    request(`/api/sources/${encodeURIComponent(id)}`, { method: "DELETE" }),
  restoreSource: (id: string) =>
    request(`/api/sources/${encodeURIComponent(id)}/restore`, {
      method: "POST",
    }),
  syncSource: (id: string) =>
    request(`/api/sources/${encodeURIComponent(id)}/sync`, { method: "POST" }),
  rollbackSource: (id: string) =>
    request<ParserSnapshot>(`/api/sources/${encodeURIComponent(id)}/rollback`, {
      method: "POST",
    }),
  parserSnapshots: async (source = "", status = "") => {
    const query = new URLSearchParams({ limit: "100" });
    if (source) query.set("source", source);
    if (status) query.set("status", status);
    return (
      await request<{ items: ParserSnapshot[] }>(
        `/api/parser-snapshots?${query}`,
      )
    ).items;
  },
  parserSnapshotPreview: (id: string) =>
    request<SnapshotPreview>(
      `/api/parser-snapshots/${encodeURIComponent(id)}/preview`,
    ),
  parserSnapshotSchedule: (id: string, groupID: string) => {
    const query = new URLSearchParams({ group: groupID });
    return request<SnapshotScheduleComparison>(
      `/api/parser-snapshots/${encodeURIComponent(id)}/schedule?${query}`,
    );
  },
  publishParserSnapshot: (id: string, reviewNote = "") =>
    request<ParserSnapshot>(
      `/api/parser-snapshots/${encodeURIComponent(id)}/publish`,
      {
        method: "POST",
        body: JSON.stringify({ review_note: reviewNote }),
      },
    ),
  rejectParserSnapshot: (id: string, reviewNote: string) =>
    request<{ status: string }>(
      `/api/parser-snapshots/${encodeURIComponent(id)}/reject`,
      {
        method: "POST",
        body: JSON.stringify({ review_note: reviewNote }),
      },
    ),
  logs: async (source = "", status = "") => {
    const query = new URLSearchParams({ limit: "150" });
    if (source) query.set("source", source);
    if (status) query.set("status", status);
    return (await request<{ items: ParseLogView[] }>(`/api/logs?${query}`))
      .items;
  },
  universities: async () =>
    (await request<{ items: UniversityOption[] }>("/api/universities")).items,
  groups: async (params: {
    page: number;
    q?: string;
    university?: string;
    pageSize?: number;
    selector?: boolean;
  }) => {
    const query = new URLSearchParams({
      page: String(params.page),
      page_size: String(params.pageSize ?? 30),
    });
    if (params.q) query.set("q", params.q);
    if (params.university) query.set("university", params.university);
    if (params.selector) query.set("selector", "true");
    const page = await request<Page<GroupView>>(`/api/groups?${query}`);
    return { ...page, items: page.items ?? [] };
  },
  lessons: async (params: {
    page: number;
    q?: string;
    university?: string;
    group?: string;
  }) => {
    const query = new URLSearchParams({
      page: String(params.page),
      page_size: "30",
    });
    if (params.q) query.set("q", params.q);
    if (params.university) query.set("university", params.university);
    if (params.group) query.set("group", params.group);
    const page = await request<Page<LessonView>>(`/api/lessons?${query}`);
    return { ...page, items: page.items ?? [] };
  },
  editorSchedule: (groupID: string) =>
    request<EditorSchedule>(
      `/api/editor/schedule?group=${encodeURIComponent(groupID)}`,
    ),
  createEditorLesson: (lesson: LessonMutationPayload) =>
    request<{ id: string }>("/api/editor/lessons", {
      method: "POST",
      body: JSON.stringify(lesson),
    }),
  updateEditorLesson: (id: string, lesson: LessonMutationPayload) =>
    request<{ id: string }>(`/api/editor/lessons/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(lesson),
    }),
  deleteEditorLesson: (
    lesson: Pick<EditorLesson, "id" | "updated_at" | "subject" | "group_id">,
  ) =>
    request<void>(`/api/editor/lessons/${encodeURIComponent(lesson.id)}`, {
      method: "DELETE",
      body: JSON.stringify({
        expected_updated_at: lesson.updated_at,
        subject: lesson.subject,
        group_id: lesson.group_id,
      }),
    }),
  restoreEditorLesson: (id: string) =>
    request<{ status: string }>(
      `/api/editor/lessons/${encodeURIComponent(id)}/restore`,
      {
        method: "POST",
      },
    ),
  users: async (q = "") => {
    const query = new URLSearchParams({ limit: "200" });
    if (q) query.set("q", q);
    return (await request<{ items: UserView[] }>(`/api/users?${query}`)).items;
  },
  updateUser: (id: string, adminRole: UserView["admin_role"]) =>
    request(`/api/users/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ admin_role: adminRole }),
    }),
  connectors: async () =>
    (await request<{ items: ConnectorClient[] }>("/api/connectors")).items,
  connectorCatalog: async () =>
    (
      await request<{ items: ManagedParserCatalogItem[] }>(
        "/api/connectors/catalog",
      )
    ).items,
  createConnector: (payload: {
    integration_mode: IntegrationMode;
    parser_id: string;
    declarative_url: string;
    update_interval: number;
    university_id: string;
    university_name: string;
    university_full_name: string;
    schedule_url: string;
    timezone: string;
    locale: string;
    display_name: string;
    description: string;
    maintainer_name: string;
    maintainer_url: string;
  }) =>
    request<{
      connector: ConnectorClient;
      credentials?: ConnectorCredentials;
      credentials_warning: string;
    }>("/api/connectors", { method: "POST", body: JSON.stringify(payload) }),
  updateConnector: (
    id: string,
    payload: { status?: ConnectorStatus; quality_policy?: SourceQualityPolicy },
  ) =>
    request<{ connector: ConnectorClient }>(
      `/api/connectors/${encodeURIComponent(id)}`,
      { method: "PATCH", body: JSON.stringify(payload) },
    ),
  rotateConnectorKey: (id: string) =>
    request<{
      credentials: ConnectorCredentials;
      credentials_warning: string;
    }>(`/api/connectors/${encodeURIComponent(id)}/rotate-key`, {
      method: "POST",
    }),
  connectorRuns: async (id: string) =>
    (
      await request<{ items: ConnectorRun[] }>(
        `/api/connectors/${encodeURIComponent(id)}/runs`,
      )
    ).items,
  supportRequests: async (
    params: { status?: string; type?: string; q?: string } = {},
  ) => {
    const query = new URLSearchParams({ limit: "200" });
    if (params.status) query.set("status", params.status);
    if (params.type) query.set("type", params.type);
    if (params.q) query.set("q", params.q);
    return (
      await request<{ items: SupportRequestView[] }>(
        `/api/support-requests?${query}`,
      )
    ).items;
  },
  resolveSupportRequest: (
    id: string,
    status: "approved" | "rejected",
    reviewNote: string,
  ) =>
    request<{ status: string }>(
      `/api/support-requests/${encodeURIComponent(id)}`,
      {
        method: "PATCH",
        body: JSON.stringify({ status, review_note: reviewNote }),
      },
    ),
  audit: async () =>
    (await request<{ items: AuditLogView[] }>("/api/audit?limit=200")).items,
};
