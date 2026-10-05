import { expect, test, type Page, type Route } from "@playwright/test";

const source = {
  id: "isuct-main",
  university_id: "isuct",
  university_name: "ИГХТУ",
  university_full_name: "Тестовый университет",
  schedule_url: "https://example.test/schedule",
  adapter_type: "isuct",
  lifecycle_status: "active",
  is_enabled: true,
  update_interval: 3600,
  consecutive_failures: 0,
  last_error: "",
  group_count: 2,
  lesson_count: 20,
  quarantined_count: 0,
  health: "healthy",
  identity_conflicts: [],
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function authenticate(page: Page) {
  await page.route("**/api/auth/config", (route) =>
    json(route, { access_key_enabled: true }),
  );
  await page.route("**/api/auth/me", (route) =>
    json(route, {
      user: {
        id: "42",
        name: "test",
        role: "owner",
        auth_method: "telegram",
        csrf_token: "test-csrf",
      },
    }),
  );
}

test("sources remain usable while snapshot history is still loading", async ({
  page,
}) => {
  await authenticate(page);
  await page.route("**/api/sources", (route) =>
    json(route, { items: [source] }),
  );
  let pending: Route | undefined;
  await page.route("**/api/parser-snapshots?*", (route) => {
    pending = route;
  });
  await page.goto("/#/sources");
  await expect.poll(() => Boolean(pending)).toBe(true);
  await expect(
    page.getByRole("heading", { name: "ИГХТУ", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("status")).toContainText(
    "Снимки расписания загружаются",
  );
  await json(pending!, { items: [] });
  await expect(page.getByText("Снимки расписания загружаются…")).toHaveCount(0);
});

test("snapshot failure preserves sources and offers a separate retry", async ({
  page,
}) => {
  await authenticate(page);
  let sourceRequests = 0;
  await page.route("**/api/sources", (route) => {
    sourceRequests++;
    return json(route, { items: [source] });
  });
  let failed = true;
  await page.route("**/api/parser-snapshots?*", (route) =>
    failed
      ? json(route, { error: "unavailable", request_id: "snapshot-test" }, 503)
      : json(route, { items: [] }),
  );
  await page.goto("/#/sources");
  await expect(
    page.getByRole("heading", { name: "ИГХТУ", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(/Снимки расписания:.*snapshot-test/),
  ).toBeVisible();
  const requestsBeforeRetry = sourceRequests;
  failed = false;
  await page.getByRole("button", { name: "Повторить", exact: true }).click();
  await expect(page.getByText(/Снимки расписания:/)).toHaveCount(0);
  expect(sourceRequests).toBe(requestsBeforeRetry);
});

test("an unanswered API request times out and can be retried", async ({
  page,
}) => {
  await page.clock.install();
  await authenticate(page);
  let stalled = true;
  let requests = 0;
  await page.route("**/api/sources", (route) => {
    requests++;
    if (!stalled) return json(route, { items: [source] });
  });
  await page.route("**/api/parser-snapshots?*", (route) =>
    json(route, { items: [] }),
  );
  await page.goto("/#/sources");
  await expect.poll(() => requests).toBeGreaterThan(0);
  await page.clock.fastForward(26_000);
  await expect(page.getByText(/Сервер не ответил за 25 секунд/)).toBeVisible();
  stalled = false;
  await page.getByRole("button", { name: "Повторить", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "ИГХТУ", exact: true }),
  ).toBeVisible();
});
