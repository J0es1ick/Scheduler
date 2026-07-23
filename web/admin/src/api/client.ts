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
  ParseLogView,
  SourceView,
  SupportRequestView,
  UniversityOption,
  UserView,
} from "../types";

export class APIError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

let csrfToken = "";

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
  if (!response.ok) {
    let message = `Ошибка ${response.status}`;
    try {
      const payload = (await response.json()) as { error?: string };
      if (payload.error) message = payload.error;
    } catch {
      // Апи всё ещё может возвращать пустой ответ для ошибок уровня прокси.
    }
    throw new APIError(response.status, message);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

function rememberUser(user: AdminIdentity): AdminIdentity {
  csrfToken = user.csrf_token;
  return user;
}

export const api = {
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
  updateSource: (id: string, updateInterval: number) =>
    request(`/api/sources/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ update_interval: updateInterval }),
    }),
  syncSource: (id: string) =>
    request(`/api/sources/${encodeURIComponent(id)}/sync`, { method: "POST" }),
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
  updateUser: (id: string, isAdmin: boolean) =>
    request(`/api/users/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify({ is_admin: isAdmin }),
    }),
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
