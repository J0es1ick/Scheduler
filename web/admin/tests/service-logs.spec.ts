import { expect, test, type Page } from "@playwright/test";

async function setup(page: Page, role = "owner") {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith("/api/")) return route.continue();
    const body =
      path === "/api/auth/me"
        ? {
            user: {
              id: "42",
              name: "Operator",
              role,
              auth_method: "telegram",
              csrf_token: "csrf",
            },
          }
        : path === "/api/auth/config"
          ? { access_key_enabled: true }
          : path === "/api/sources"
            ? {
                items: [
                  {
                    id: "ispu-main",
                    university_name: "ИГЭУ",
                    adapter_type: "ispu",
                  },
                ],
              }
            : path === "/api/dashboard"
              ? {
                  stats: {
                    universities: 3,
                    groups: 10,
                    lessons: 100,
                    users: 20,
                    subscriptions: 30,
                    success_rate: 100,
                  },
                  sources: [],
                  recent_logs: [],
                  trend: [],
                  universities: [],
                  operations: {
                    pending_notifications: 0,
                    pending_outbox: 0,
                    failed_notifications: 0,
                    failed_outbox: 0,
                    oldest_pending_seconds: 0,
                    checked_at: "2026-09-09T10:01:00Z",
                  },
                }
              : { items: [] };
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
}

const result = {
  entries: [
    {
      id: "one",
      time: "2026-09-09T10:00:00Z",
      component: "parser-worker",
      module: "parser",
      source: "ispu-main",
      level: "ERROR",
      message: "parser: permission denied <script>alert(1)</script>",
      fields: {
        request_id: "request-42",
        error: "permission denied for table groups",
        token: "<redacted>",
      },
    },
  ],
  components: [
    { name: "bot", state: "журнал" },
    { name: "parser-worker", state: "журнал" },
    { name: "postgres", state: "журнал" },
  ],
  modules: ["parser", "notification"],
  warnings: [],
  checked_at: "2026-09-09T10:01:00Z",
  next_cursor: "older-page",
};

for (const width of [320, 390, 768, 1440])
  test(`service logs filter, expand and paginate at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 844 });
    await setup(page);
    const requests: URL[] = [];
    await page.route("**/api/service-logs?**", async (route) => {
      const url = new URL(route.request().url());
      requests.push(url);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(
          url.searchParams.get("cursor")
            ? {
                ...result,
                next_cursor: "",
                entries: [
                  {
                    ...result.entries[0],
                    id: "two",
                    message: "Earlier record",
                  },
                ],
              }
            : result,
        ),
      });
    });
    await page.goto("/#/logs");
    await expect(
      page.getByText("Журналы компонентов", { exact: true }),
    ).toBeVisible();
    await page
      .getByLabel("Компонент", { exact: true })
      .selectOption("parser-worker");
    await page.getByLabel("Парсер / источник").selectOption("ispu-main");
    await page.getByLabel("Уровень", { exact: true }).selectOption("ERROR");
    await page.getByLabel("Текст или request ID").fill("request-42");
    await page.getByRole("button", { name: "Найти", exact: true }).click();
    await expect
      .poll(() => requests.at(-1)?.searchParams.get("q"))
      .toBe("request-42");
    expect(requests.at(-1)?.searchParams.get("component")).toBe(
      "parser-worker",
    );
    expect(requests.at(-1)?.searchParams.get("source")).toBe("ispu-main");
    await page.locator(".service-log-entry summary").click();
    await expect(page.getByLabel("Поля записи")).toContainText(
      "permission denied for table groups",
    );
    expect(await page.locator(".service-log-entry script").count()).toBe(0);
    await page.getByRole("button", { name: "Раньше", exact: true }).click();
    await expect(
      page.getByText("Earlier record", { exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Позже", exact: true }).click();
    await expect(page.locator(".service-log-message")).toContainText(
      "permission denied",
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBeTruthy();
    for (const selector of [
      ".main-area",
      ".service-logs",
      ".service-log-filters",
    ]) {
      const bounds = await page.locator(selector).boundingBox();
      expect(bounds?.x, selector).toBeGreaterThanOrEqual(0);
      expect(bounds!.x + bounds!.width, selector).toBeLessThanOrEqual(
        width + 1,
      );
    }
  });

test("service logs keep previous data on refresh failure and retry", async ({
  page,
}) => {
  await setup(page);
  let failed = false;
  await page.route("**/api/service-logs?**", (route) =>
    route.fulfill({
      status: failed ? 503 : 200,
      contentType: "application/json",
      headers: { "X-Request-ID": "logs-outage-42" },
      body: JSON.stringify(failed ? { error: "unavailable" } : result),
    }),
  );
  await page.goto("/#/logs");
  await expect(page.locator(".service-log-message")).toBeVisible();
  failed = true;
  await page
    .getByRole("button", { name: "Обновить журнал", exact: true })
    .click();
  await expect(
    page.getByText("Ниже показаны данные последней успешной загрузки."),
  ).toBeVisible();
  await expect(page.getByText(/logs-outage-42/)).toBeVisible();
  await expect(page.locator(".service-log-message")).toBeVisible();
  failed = false;
  await page
    .getByRole("button", { name: "Обновить журнал", exact: true })
    .click();
  await expect(page.getByText(/logs-outage-42/)).toHaveCount(0);
});

test("read-only administrators see parser history without operational log access", async ({
  page,
}) => {
  await setup(page, "read_only");
  let calls = 0;
  await page.route("**/api/service-logs?**", (route) => {
    calls++;
    return route.abort();
  });
  await page.goto("/#/logs");
  await expect(
    page.getByText("Запуски парсеров", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Журналы компонентов", { exact: true }),
  ).toHaveCount(0);
  expect(calls).toBe(0);
});

test("malformed log response stays inside the log panel", async ({ page }) => {
  await setup(page);
  await page.route("**/api/service-logs?**", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ items: [] }),
    }),
  );
  await page.goto("/#/logs");
  await expect(
    page.getByText("Сервер вернул некорректный журнал. Повторите загрузку."),
  ).toBeVisible();
  await expect(page.getByRole("switch", { name: "Тёмная тема" })).toBeVisible();
  await expect(
    page.getByText("Запуски парсеров", { exact: true }),
  ).toBeVisible();
});
