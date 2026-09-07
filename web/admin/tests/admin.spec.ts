import { expect, test, type Page, type Route } from "@playwright/test";
import { expectReadableTheme } from "./theme-contrast";

test.afterEach(async ({ page }, info) => {
  if (info.status !== "passed") return;
  await expectReadableTheme(page);
  if (
    process.env.SCHEDULER_THEME_CAPTURE === "1" &&
    /quarantined snapshot|group search|wizard|identity conflict|backend outage/.test(
      info.title,
    )
  ) {
    await page.screenshot({
      path: info.outputPath("theme.png"),
      fullPage: false,
      animations: "disabled",
    });
  }
});

const admin = {
  id: "42",
  name: "@release_admin",
  auth_method: "telegram",
  csrf_token: "csrf-test-token",
  role: "owner",
};

const dashboard = {
  stats: {
    universities: 2,
    groups: 522,
    lessons: 1200,
    users: 10,
    subscriptions: 14,
    success_rate: 99,
  },
  sources: [],
  recent_logs: [],
  trend: [],
  universities: [],
  operations: {
    status: "healthy",
    database: true,
    sources_total: 2,
    sources_healthy: 2,
    sources_running: 0,
    sources_stale: 0,
    sources_error: 0,
    sources_quarantined: 0,
    sources_disabled: 0,
    pending_notifications: 0,
    failed_notifications: 0,
    pending_outbox: 0,
    failed_outbox: 0,
    oldest_pending_seconds: 0,
    last_successful_parse_at: "2026-08-03T12:00:00Z",
    checked_at: "2026-08-03T12:01:00Z",
  },
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function mockAuthenticated(page: Page) {
  await page.route("**/api/auth/config", (route) =>
    json(route, { access_key_enabled: true }),
  );
  await page.route("**/api/auth/me", (route) => json(route, { user: admin }));
  await page.route("**/api/client-errors", (route) =>
    route.fulfill({ status: 204 }),
  );
}

async function expectViewportDialog(page: Page) {
  const backdrop = page.locator(".dialog-backdrop").last();
  await expect(backdrop).toBeVisible();
  expect(
    await backdrop.evaluate(
      (element) =>
        element.parentElement?.parentElement === document.body &&
        !document.getElementById("root")?.contains(element),
    ),
  ).toBe(true);
  const box = await backdrop.boundingBox();
  const viewport = page.viewportSize();
  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  expect(box!.x).toBe(0);
  expect(box!.y).toBe(0);
  expect(box!.width).toBe(viewport!.width);
  expect(box!.height).toBe(viewport!.height);
}

test("general feedback can be read and filtered in support", async ({
  page,
}) => {
  await mockAuthenticated(page);
  const filters: string[] = [];
  await page.route("**/api/support-requests?*", (route) => {
    filters.push(new URL(route.request().url()).searchParams.get("type") ?? "");
    return json(route, {
      items: [
        {
          id: "feedback-1",
          user_id: "42",
          username: "synthetic",
          request_type: "feedback",
          details: "Хочу настройку размера текста в боте",
          status: "pending",
          review_note: "",
          created_at: "2026-09-07T12:00:00Z",
        },
      ],
    });
  });
  await page.goto("/#/support");
  await expect(
    page.getByText("Хочу настройку размера текста в боте"),
  ).toBeVisible();
  await page
    .getByRole("combobox", { name: "Тип обращения" })
    .selectOption("feedback");
  await expect.poll(() => filters.includes("feedback")).toBe(true);
  await expect(
    page.locator(".support-card").getByText("Пожелания и обратная связь"),
  ).toBeVisible();
});

test("access-key bootstrap login remains available only when enabled", async ({
  page,
}) => {
  await page.route("**/api/auth/config", (route) =>
    json(route, { access_key_enabled: true }),
  );
  await page.route("**/api/auth/me", (route) =>
    json(route, { error: "auth", status: 401 }, 401),
  );
  await page.route("**/api/auth/access-key", async (route) => {
    const payload = route.request().postDataJSON() as { access_key: string };
    expect(payload.access_key).toBe("release-key");
    await json(route, { user: { ...admin, auth_method: "access_key" } });
  });
  await page.route("**/api/dashboard", (route) => json(route, dashboard));

  await page.goto("/");
  await page.getByLabel("Ключ доступа").fill("release-key");
  await page.getByRole("button", { name: "Войти" }).click();

  await expect(
    page.getByRole("heading", { name: "Обзор", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("@release_admin")).toBeVisible();
});

test("integration wizard makes the managed parser the serverless default", async ({
  page,
}) => {
  await mockAuthenticated(page);
  let createdMode = "";
  let createdParser = "";
  await page.route("**/api/connectors/catalog", (route) =>
    json(route, {
      items: [
        {
          connected: false,
          manifest: {
            contract_version: "1.0",
            parser_id: "ivgpu",
            version: "1.0.0",
            display_name: "ИВГПУ · управляемый парсер",
            description: "Официальный JSON API",
            institution: {
              external_id: "ivgpu",
              name: "ИВГПУ",
              full_name:
                "Ивановский государственный политехнический университет",
              schedule_url: "https://ivgpu.ru/raspisanie",
              timezone: "Europe/Moscow",
              locale: "ru-RU",
            },
            maintainer_name: "Scheduler contributors",
            update_interval: 3600,
          },
        },
      ],
    }),
  );
  await page.route("**/api/connectors", async (route) => {
    if (route.request().method() === "POST") {
      const payload = route.request().postDataJSON() as {
        integration_mode: string;
        parser_id: string;
      };
      createdMode = payload.integration_mode;
      createdParser = payload.parser_id;
      await json(route, { connector: {}, credentials_warning: "" }, 201);
      return;
    }
    await json(route, { items: [] });
  });

  await page.goto("/#/connectors");
  await expect(
    page.getByRole("heading", { name: "Интеграции", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Подключить источник" }).click();
  await expect(
    page.getByRole("button", { name: /Управляемый парсер Рекомендуется/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /JSON по HTTPS/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /Внешний сервер/ }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: /Управляемый парсер Рекомендуется/ })
    .click();
  await page.getByRole("button", { name: /Продолжить/ }).click();
  await page
    .getByRole("button", { name: /ИВГПУ · управляемый парсер/ })
    .click();
  await page.getByRole("button", { name: /Продолжить/ }).click();
  await page.getByRole("button", { name: /Продолжить/ }).click();
  await page.getByRole("button", { name: /Создать интеграцию/ }).click();

  await expect(page.getByText("Интеграция создана в черновиках")).toBeVisible();
  expect(createdMode).toBe("managed_parser");
  expect(createdParser).toBe("ivgpu");
});

test("group search keeps the editor visible and confirms a manual change", async ({
  page,
}) => {
  await mockAuthenticated(page);
  let subject = "Технология материалов";
  let updateRequests = 0;
  await page.route("**/api/universities", (route) =>
    json(route, {
      items: [
        {
          id: "isuct",
          name: "ИГХТУ",
          full_name: "ИГХТУ",
          schedule_url: "",
          is_active: true,
        },
      ],
    }),
  );
  await page.route("**/api/groups?**", (route) =>
    json(route, {
      items: [
        {
          id: "isuct-3u-1",
          name: "3ю-1",
          university_id: "isuct",
          university_name: "ИГХТУ",
          is_active: true,
          lesson_count: 2,
          updated_at: "2026-08-03T12:00:00Z",
        },
      ],
      pagination: { page: 1, page_size: 20, total: 1 },
    }),
  );
  await page.route("**/api/editor/schedule?**", (route) =>
    json(route, {
      group: {
        id: "isuct-3u-1",
        name: "3ю-1",
        university_id: "isuct",
        university_name: "ИГХТУ",
        updated_at: "2026-08-03T12:00:00Z",
      },
      semesters: [
        {
          id: "semester",
          name: "Осень",
          start_date: "2026-09-01",
          end_date: "2026-12-31",
        },
      ],
      lessons: ["odd", "even"].map((week, index) => ({
        id: `lesson-${index}`,
        university_id: "isuct",
        semester_id: "semester",
        day_of_week: 1,
        special_date: null,
        time_start: "09:50",
        time_end: "11:25",
        week_type: week,
        subject,
        type: "lecture",
        teacher: "Иванов И.И.",
        room: "А-305",
        group_id: "isuct-3u-1",
        subgroup: 0,
        valid_from: "2026-09-01",
        valid_to: "2026-12-31",
        updated_at: "2026-08-03T12:00:00Z",
        origin: "parsed",
        base_lesson_id: null,
        version: 1,
        deleted: false,
      })),
      deleted_lessons: [],
    }),
  );
  await page.route("**/api/editor/lessons/lesson-0", async (route) => {
    updateRequests += 1;
    subject = (route.request().postDataJSON() as { subject: string }).subject;
    await json(route, { id: "lesson-0" });
  });

  await page.goto("/#/editor");
  await page.getByPlaceholder("Введите номер или часть названия").fill("3ю");
  await page.getByRole("option", { name: /3ю-1/ }).click();

  await expect(
    page.getByRole("heading", { name: "Нечётная неделя", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Чётная неделя", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Редактировать" }).first().click();
  await expectViewportDialog(page);
  await page.getByLabel("Предмет").fill("Обновлённый предмет");
  await page.getByRole("button", { name: "Проверить изменения" }).click();
  await page.getByRole("button", { name: "Подтвердить и применить" }).click();

  await expect(page.getByText("Занятие обновлено")).toBeVisible();
  expect(updateRequests).toBe(1);
});

test("an expired session immediately returns to the login screen", async ({
  page,
}) => {
  await mockAuthenticated(page);
  await page.route("**/api/universities", (route) =>
    json(
      route,
      { code: "auth.required", error: "Сессия истекла", status: 401 },
      401,
    ),
  );

  await page.goto("/#/editor");
  await expect(
    page.getByText(
      "Сессия истекла. Войдите снова через Telegram или аварийный ключ.",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Вход в админку" }),
  ).toBeVisible();
});

test("quarantined snapshot can be inspected group by group before publication", async ({
  page,
}) => {
  await mockAuthenticated(page);
  const snapshot = {
    id: "snapshot-quarantine",
    data_source_id: "isuct-main",
    parse_log_id: "parse-log",
    status: "quarantined",
    publishable: true,
    group_count: 197,
    lesson_count: 1,
    anomaly_reasons: [
      { code: "lesson_drop", message: "Количество занятий резко уменьшилось" },
    ],
    reviewed_by: "",
    review_note: "",
    created_at: "2026-08-03T14:02:00Z",
    published_at: null,
    reviewed_at: null,
  };
  await page.route("**/api/sources", (route) =>
    json(route, {
      items: [
        {
          id: "isuct-main",
          university_id: "isuct",
          university_name: "ИГХТУ",
          university_full_name:
            "Ивановский государственный химико-технологический университет",
          schedule_url: "https://example.test/schedule",
          adapter_type: "isuct",
          is_enabled: true,
          update_interval: 3600,
          last_run_at: "2026-08-03T14:02:00Z",
          last_success_at: "2026-08-03T12:00:00Z",
          next_run_at: "2026-08-03T15:02:00Z",
          last_error: "",
          consecutive_failures: 0,
          next_retry_at: null,
          current_snapshot_id: "snapshot-current",
          quarantined_count: 1,
          latest_status: "quarantined",
          latest_started_at: "2026-08-03T14:00:00Z",
          latest_finished_at: "2026-08-03T14:02:00Z",
          latest_records: 1,
          group_count: 522,
          lesson_count: 3057,
          running: false,
          health: "quarantined",
        },
      ],
    }),
  );
  await page.route("**/api/parser-snapshots?**", (route) =>
    json(route, { items: [snapshot] }),
  );
  await page.route(
    "**/api/parser-snapshots/snapshot-quarantine/preview",
    (route) =>
      json(route, {
        snapshot_id: snapshot.id,
        data_source_id: snapshot.data_source_id,
        status: snapshot.status,
        publishable: true,
        created_at: snapshot.created_at,
        candidate_start_date: "2026-09-01T00:00:00Z",
        candidate_end_date: "2026-12-31T00:00:00Z",
        candidate_group_count: 197,
        candidate_lesson_count: 1,
        current_snapshot_id: "snapshot-current",
        current_created_at: "2026-08-01T12:00:00Z",
        current_group_count: 522,
        current_lesson_count: 3057,
        comparison_available: true,
        summary: {
          added_groups: 0,
          removed_groups: 325,
          changed_groups: 1,
          unchanged_groups: 196,
          added_lessons: 1,
          removed_lessons: 3057,
        },
        groups: [
          {
            id: "isuct:new:3-147",
            current_id: "isuct:old:3-147",
            candidate_id: "isuct:new:3-147",
            name: "3/147",
            status: "changed",
            current_lessons: 1,
            candidate_lessons: 1,
            added_lessons: 1,
            removed_lessons: 1,
          },
        ],
      }),
  );
  await page.route(
    "**/api/parser-snapshots/snapshot-quarantine/schedule?**",
    (route) =>
      json(route, {
        snapshot_id: snapshot.id,
        group_id: "isuct:new:3-147",
        group_name: "3/147",
        status: "changed",
        comparison_available: true,
        current: [
          {
            id: "old-lesson",
            day_of_week: 1,
            special_date: null,
            time_start: "09:50",
            time_end: "11:25",
            week_type: "every",
            subject: "Старое занятие",
            type: "lecture",
            teacher: "Иванов И.И.",
            room: "А-101",
            subgroup: 0,
            valid_from: "2026-02-01T00:00:00Z",
            valid_to: "2026-06-30T00:00:00Z",
            diff: "removed",
          },
        ],
        candidate: [
          {
            id: "new-lesson",
            day_of_week: 1,
            special_date: null,
            time_start: "12:10",
            time_end: "13:45",
            week_type: "odd",
            subject: "Новое занятие",
            type: "practice",
            teacher: "Петров П.П.",
            room: "Б-202",
            subgroup: 0,
            valid_from: "2026-09-01T00:00:00Z",
            valid_to: "2026-12-31T00:00:00Z",
            diff: "added",
          },
        ],
      }),
  );

  await page.goto("/#/sources");
  await page.getByRole("button", { name: "Изучить данные" }).click();

  await expect(
    page.getByRole("heading", { name: "Содержимое нового снимка" }),
  ).toBeVisible();
  await expectViewportDialog(page);
  await expect(page.getByRole("button", { name: /3\/147/ })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Опубликовано сейчас" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Получено с сайта" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Нечётная неделя", exact: true }),
  ).toHaveCount(2);
  await expect(
    page.getByRole("heading", { name: "Чётная неделя", exact: true }),
  ).toHaveCount(2);
  await expect(page.getByText("Старое занятие")).toHaveCount(2);
  await expect(page.getByText("Новое занятие")).toBeVisible();
  await expect(page.getByText("удалено")).toHaveCount(2);
  await expect(page.getByText("добавлено")).toBeVisible();
});

test("source can be disabled and archiving requires confirmation", async ({
  page,
}) => {
  await mockAuthenticated(page);
  let enabled = true;
  let archived = false;
  let archiveRequests = 0;
  let restoreRequests = 0;

  await page.route("**/api/sources", (route) =>
    json(route, {
      items: [
        {
          id: "isuct-main",
          university_id: "isuct",
          university_name: "ИГХТУ",
          university_full_name:
            "Ивановский государственный химико-технологический университет",
          schedule_url: "https://example.test/schedule",
          adapter_type: "isuct",
          lifecycle_status: archived ? "archived" : "active",
          archived_at: archived ? "2026-08-03T15:00:00Z" : null,
          is_enabled: enabled,
          update_interval: 3600,
          last_run_at: "2026-08-03T14:02:00Z",
          last_success_at: "2026-08-03T14:02:00Z",
          next_run_at: enabled ? "2026-08-03T15:02:00Z" : null,
          last_error: "",
          consecutive_failures: 0,
          next_retry_at: null,
          current_snapshot_id: "snapshot-current",
          quarantined_count: 0,
          latest_status: "success",
          latest_started_at: "2026-08-03T14:00:00Z",
          latest_finished_at: "2026-08-03T14:02:00Z",
          latest_records: 3057,
          group_count: 522,
          lesson_count: 3057,
          running: false,
          health: enabled ? "healthy" : "disabled",
        },
      ],
    }),
  );
  await page.route("**/api/parser-snapshots?**", (route) =>
    json(route, { items: [] }),
  );
  await page.route("**/api/sources/isuct-main", async (route) => {
    if (route.request().method() === "PATCH") {
      const payload = route.request().postDataJSON() as {
        is_enabled?: boolean;
      };
      enabled = payload.is_enabled ?? enabled;
      await json(route, { status: "updated" });
      return;
    }
    if (route.request().method() === "DELETE") {
      archiveRequests += 1;
      archived = true;
      await json(route, { status: "archived" });
      return;
    }
    await route.fallback();
  });
  await page.route("**/api/sources/isuct-main/restore", async (route) => {
    restoreRequests += 1;
    archived = false;
    enabled = false;
    await json(route, { status: "restored", lifecycle_status: "active" });
  });

  await page.goto("/#/sources");
  await page.getByRole("button", { name: "Отключить" }).click();
  await expect(page.getByRole("button", { name: "Включить" })).toBeVisible();
  await expect(page.getByText("Отключено", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "В архив", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Архивировать источник ИГХТУ?" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Отмена" }).click();
  expect(archiveRequests).toBe(0);

  await page.getByRole("button", { name: "В архив", exact: true }).click();
  await page.getByRole("button", { name: "Перенести в архив" }).click();
  await expect.poll(() => archiveRequests).toBe(1);
  await expect(
    page.getByRole("heading", { name: "Архивировать источник ИГХТУ?" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: /Архив 1/ }).click();
  await expect(page.getByText(/Архивирован/)).toBeVisible();
  await page.getByRole("button", { name: "Восстановить" }).click();
  await expect.poll(() => restoreRequests).toBe(1);
  await expect(page.getByRole("button", { name: /Архив 0/ })).toBeVisible();
});

test("external source is managed through its connector instead of parser controls", async ({
  page,
}) => {
  await mockAuthenticated(page);
  await page.route("**/api/sources", (route) =>
    json(route, {
      items: [
        {
          id: "external-ivgpu",
          university_id: "ivgpu",
          university_name: "ИВГПУ",
          university_full_name:
            "Ивановский государственный политехнический университет",
          schedule_url: "https://ivgpu.ru/raspisanie",
          adapter_type: "external_push",
          lifecycle_status: "active",
          archived_at: null,
          insecure_transport: false,
          is_enabled: true,
          update_interval: 3600,
          last_run_at: "2026-08-10T12:26:00Z",
          last_success_at: "2026-08-10T12:26:00Z",
          next_run_at: null,
          last_error: "",
          consecutive_failures: 0,
          next_retry_at: null,
          current_snapshot_id: "snapshot-ivgpu",
          quarantined_count: 0,
          latest_status: "success",
          latest_started_at: "2026-08-10T12:26:00Z",
          latest_finished_at: "2026-08-10T12:26:01Z",
          latest_records: 1651,
          group_count: 266,
          lesson_count: 1651,
          running: false,
          health: "healthy",
        },
      ],
    }),
  );
  await page.route("**/api/parser-snapshots?**", (route) =>
    json(route, { items: [] }),
  );

  await page.goto("/#/sources");

  await expect(
    page.getByText("Внешний коннектор", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Управляется внешним коннектором", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Открыть коннектор" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Запустить" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Отключить" })).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "В архив", exact: true }),
  ).toHaveCount(0);
});

test("backend outage is shown instead of a blank screen", async ({ page }) => {
  await page.route("**/api/auth/config", (route) =>
    json(route, { error: "offline" }, 503),
  );
  await page.route("**/api/auth/me", (route) =>
    json(
      route,
      {
        code: "service.unavailable",
        error: "Сервис временно недоступен",
        status: 503,
      },
      503,
    ),
  );

  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Вход в админку" }),
  ).toBeVisible();
  await expect(page.getByRole("alert")).toContainText(
    "Сервис временно недоступен",
  );
});

test("group identity conflict is resolved explicitly before source retry", async ({
  page,
}) => {
  await mockAuthenticated(page);
  let resolution = "";
  let syncRequests = 0;
  const conflict = {
    id: "identity-conflict-1",
    data_source_id: "ispu-main",
    university_id: "ispu",
    external_group_id: "ispu:group:101016",
    existing_group_id: "ispu:group:101016",
    existing_name: "2-ЭЭ-В",
    incoming_name: "1-ЭЭ-В",
    status: "pending",
    resolution: "",
    first_seen_at: "2026-08-24T08:00:00Z",
    last_seen_at: "2026-08-24T08:10:00Z",
    occurrences: 3,
    subscription_count: 0,
    default_group_count: 0,
    chat_count: 0,
    lesson_count: 18,
  };
  await page.route(
    "**/api/sources/ispu-main/group-identity-conflicts/identity-conflict-1/resolve",
    async (route) => {
      resolution = (route.request().postDataJSON() as { resolution: string })
        .resolution;
      await json(route, { ...conflict, status: "resolved", resolution });
    },
  );
  await page.route("**/api/sources/ispu-main/sync", async (route) => {
    syncRequests += 1;
    await json(route, { status: "started", source_id: "ispu-main" }, 202);
  });
  await page.route("**/api/sources", (route) =>
    json(route, {
      items: [
        {
          id: "ispu-main",
          university_id: "ispu",
          university_name: "ИГЭУ",
          university_full_name:
            "Ивановский государственный энергетический университет",
          schedule_url: "http://schedule.ispu.ru",
          adapter_type: "ispu",
          lifecycle_status: "active",
          archived_at: null,
          allow_empty: false,
          insecure_transport: true,
          is_enabled: true,
          update_interval: 3600,
          last_run_at: "2026-08-24T08:10:00Z",
          last_success_at: "2026-08-23T08:10:00Z",
          next_run_at: "2026-08-24T08:22:00Z",
          last_error: "group identity conflict",
          consecutive_failures: 3,
          next_retry_at: "2026-08-24T08:22:00Z",
          current_snapshot_id: "snapshot-old",
          quarantined_count: 0,
          latest_status: "failed",
          latest_started_at: "2026-08-24T08:10:00Z",
          latest_finished_at: "2026-08-24T08:10:02Z",
          latest_records: 0,
          group_count: 10,
          lesson_count: 120,
          identity_conflicts: [conflict],
          running: false,
          health: "error",
        },
      ],
    }),
  );
  await page.route("**/api/parser-snapshots?**", (route) =>
    json(route, { items: [] }),
  );

  await page.goto("/#/sources");
  await expect(
    page.getByText("Сайт изменил принадлежность ID группы"),
  ).toBeVisible();
  await expect(page.getByText("2-ЭЭ-В", { exact: true })).toBeVisible();
  await expect(page.getByText("1-ЭЭ-В", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Считать переименованием" }).click();
  await expectViewportDialog(page);
  await expect(
    page.getByText(/Все её подписки.*останутся привязаны/),
  ).toBeVisible();
  await page.getByRole("button", { name: "Отмена" }).click();

  await page.getByRole("button", { name: "Создать новую группу" }).click();
  await expectViewportDialog(page);
  await page.getByRole("button", { name: "Да, создать новую" }).click();

  await expect.poll(() => resolution).toBe("new_group");
  await expect.poll(() => syncRequests).toBe(1);
  await expect(page.getByText(/создана отдельная группа/)).toBeVisible();
});

test("group directory manages active and archived groups safely", async ({
  page,
}) => {
  await mockAuthenticated(page);
  let activeGroup = {
    id: "ispu-active",
    name: "1-ЭЭ-В",
    university_id: "ispu",
    university_name: "ИГЭУ",
    is_active: true,
    source_active: true,
    manually_disabled: false,
    lesson_count: 18,
    subscription_count: 2,
    default_group_count: 1,
    chat_count: 1,
    override_count: 0,
    created_at: "2026-08-24T08:00:00Z",
    updated_at: "2026-08-24T08:10:00Z",
  };
  const archivedGroup = {
    id: "ispu-archived",
    name: "2-ЭЭ-В",
    university_id: "ispu",
    university_name: "ИГЭУ",
    is_active: false,
    source_active: false,
    manually_disabled: false,
    lesson_count: 0,
    subscription_count: 1,
    default_group_count: 1,
    chat_count: 0,
    override_count: 2,
    created_at: "2025-08-24T08:00:00Z",
    updated_at: "2026-08-23T08:10:00Z",
  };
  let deleted = false;
  let requestedActive: boolean | null = null;
  let requestedOrder = "";

  await page.route("**/api/universities", (route) =>
    json(route, {
      items: [
        {
          id: "ispu",
          name: "ИГЭУ",
          full_name: "Ивановский государственный энергетический университет",
          schedule_url: "http://schedule.ispu.ru",
          is_active: true,
        },
      ],
    }),
  );
  await page.route("**/api/groups?**", (route) => {
    const parameters = new URL(route.request().url()).searchParams;
    const status = parameters.get("status") ?? "active";
    requestedOrder = parameters.get("order") ?? "name";
    const candidates = [activeGroup, ...(deleted ? [] : [archivedGroup])];
    const items = candidates.filter((group) =>
      status === "all"
        ? true
        : status === "active"
          ? group.is_active
          : !group.is_active,
    );
    return json(route, {
      items,
      pagination: { page: 1, page_size: 30, total: items.length },
    });
  });
  await page.route("**/api/groups/ispu-active", async (route) => {
    requestedActive = (route.request().postDataJSON() as { is_active: boolean })
      .is_active;
    activeGroup = {
      ...activeGroup,
      is_active: requestedActive,
      manually_disabled: !requestedActive,
    };
    await json(route, activeGroup);
  });
  await page.route("**/api/groups/ispu-archived", async (route) => {
    deleted = true;
    await json(route, archivedGroup);
  });

  await page.goto("/#/data");
  await expect(
    page.getByRole("heading", { name: "Учебные группы" }),
  ).toBeVisible();
  await expect(page.getByText("1-ЭЭ-В", { exact: true })).toBeVisible();

  await page.getByLabel("Сортировка групп").selectOption("oldest");
  await expect.poll(() => requestedOrder).toBe("oldest");

  await page.getByRole("button", { name: "Отключить" }).click();
  await expectViewportDialog(page);
  await expect(
    page.getByText(/подписки, ручные правки.*сохранятся/),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Отключить", exact: true })
    .last()
    .click();
  await expect.poll(() => requestedActive).toBe(false);

  await page.getByLabel("Состояние групп").selectOption("inactive");
  const archivedRow = page
    .locator(".group-directory-row")
    .filter({ hasText: "2-ЭЭ-В" });
  await archivedRow.getByRole("button", { name: "Удалить" }).click();
  await expectViewportDialog(page);
  await expect(page.getByText("1 подписок", { exact: true })).toBeVisible();
  await expect(
    page.getByText("2 ручных правок", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Удалить навсегда" }).click();
  await expect.poll(() => deleted).toBe(true);
  await expect(page.getByText("Группа 2-ЭЭ-В удалена")).toBeVisible();
});
