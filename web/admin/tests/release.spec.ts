import { test, expect, type Page } from "@playwright/test";
const roleNames = [
  "read_only",
  "support",
  "editor",
  "reviewer",
  "operator",
  "owner",
];
const json = (body: unknown, status = 200) => ({
  status,
  contentType: "application/json",
  body: JSON.stringify(body),
});
async function shell(page: Page, role: string) {
  await page.route("**/api/auth/config", (r) =>
    r.fulfill(json({ access_key_enabled: false })),
  );
  await page.route("**/api/auth/me", (r) =>
    r.fulfill(
      json({
        user: {
          id: "42",
          name: "Synthetic",
          role,
          auth_method: "telegram",
          csrf_token: "synthetic-csrf",
        },
      }),
    ),
  );
  await page.route("**/api/dashboard", (r) =>
    r.fulfill(
      json({
        stats: {
          universities: 0,
          groups: 0,
          lessons: 0,
          users: 0,
          subscriptions: 0,
          success_rate: 0,
        },
        sources: [],
        recent_logs: [],
        trend: [],
        universities: [],
        operations: {
          status: "healthy",
          database: true,
          sources_total: 0,
          sources_healthy: 0,
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
          checked_at: new Date().toISOString(),
        },
      }),
    ),
  );
  await page.route("**/api/client-errors", (r) => r.fulfill({ status: 204 }));
}
for (const role of roleNames)
  for (const width of [320, 390, 768, 1440])
    test(`release shell ${role} ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      await shell(page, role);
      await page.goto("/#/overview");
      await expect(
        page.getByRole("button", { name: "Выйти", exact: true }),
      ).toBeVisible();
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
      ).toBe(true);
      if (width < 768)
        await expect(page.getByLabel("Раздел", { exact: true })).toBeVisible();
      if (role === "read_only" || role === "support")
        await expect(
          page.getByRole("button", { name: "Редактор", exact: true }),
        ).toHaveCount(0);
    });
test("failed logout keeps the session visible and successful logout prevents automatic re-entry", async ({
  page,
}) => {
  await shell(page, "owner");
  let signedIn = true,
    loginAttempts = 0,
    fail = true;
  await page.addInitScript(() => {
    Object.defineProperty(window, "Telegram", {
      value: {
        WebApp: {
          initData: "synthetic-init-data",
          ready() {},
          expand() {},
          colorScheme: "dark",
        },
      },
    });
  });
  await page.route("**/api/auth/me", (r) =>
    r.fulfill(
      signedIn
        ? json({
            user: {
              id: "42",
              name: "Synthetic",
              role: "owner",
              auth_method: "telegram",
              csrf_token: "synthetic-csrf",
            },
          })
        : json({ error: "expired" }, 401),
    ),
  );
  await page.route("**/api/auth/telegram", (r) => {
    loginAttempts++;
    return r.fulfill(json({ error: "synthetic login rejected" }, 401));
  });
  await page.route("**/api/auth/logout", (r) => {
    if (fail) return r.fulfill(json({ error: "temporary" }, 503));
    signedIn = false;
    return r.fulfill({ status: 204 });
  });
  await page.goto("/#/overview");
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(page.getByText(/Сервис временно недоступен/)).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Выйти", exact: true }),
  ).toBeVisible();
  fail = false;
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Вход в админку" }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Вход в админку" }),
  ).toBeVisible();
  expect(loginAttempts).toBe(0);
});

test("empty API log arrays do not crash the dashboard", async ({ page }) => {
  await shell(page, "owner");
  await page.route("**/api/dashboard", (r) =>
    r.fulfill(
      json({
        stats: {
          universities: 0,
          groups: 0,
          lessons: 0,
          users: 0,
          subscriptions: 0,
          success_rate: 0,
        },
        sources: [],
        recent_logs: null,
        trend: [],
        universities: [],
        operations: { status: "healthy" },
      }),
    ),
  );
  await page.goto("/#/overview");
  await expect(page.getByText("Запусков пока нет")).toBeVisible();
});
