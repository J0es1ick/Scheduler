import { expect, test } from "@playwright/test";
import { setup } from "./personal-fixture";

test("ordinary user edits one day and resets it", async ({ page }) => {
  const state = await setup(page);
  await page.goto("/app");
  await expect(
    page.getByRole("heading", { name: "Ваши правки" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Куда перейдём?" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: /Изменить Теоретическая/ }).click();
  await page.getByLabel("Аудитория", { exact: true }).fill("А-101");
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(
    page.getByText("Аудитория А-101", { exact: true }),
  ).toBeVisible();
  expect(state.requests[0]).toMatchObject({
    scope: "day",
    lesson_id: "stable",
    date: "2026-10-05",
    patch: { room: "А-101" },
  });
  await page.getByText("Мои правки · 1").click();
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: /Удалить правку/ }).click();
  await expect(
    page.getByText("Аудитория Б-204", { exact: true }),
  ).toBeVisible();
});

test("administrator chooses personal app or admin", async ({ page }) => {
  await setup(page, true);
  await page.goto("/app");
  await expect(
    page.getByRole("heading", { name: "Куда перейдём?" }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: /Админка/ })).toHaveAttribute(
    "href",
    "/#/editor",
  );
  await page.getByRole("button", { name: /Личное расписание/ }).click();
  await page.getByRole("button", { name: "← К выбору сервиса" }).click();
  await expect(
    page.getByRole("heading", { name: "Куда перейдём?" }),
  ).toBeVisible();
});

test("semester cancellation and conflicts preserve the form", async ({
  page,
}) => {
  const state = await setup(page);
  await page.goto("/app");
  await page.getByRole("button", { name: /Изменить Теоретическая/ }).click();
  await page.getByLabel("Применить", { exact: true }).selectOption("semester");
  await page.getByLabel("Отменить занятие для меня").check();
  state.conflict = true;
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Правка уже изменилась");
  await expect(page.getByLabel("Отменить занятие для меня")).toBeChecked();
  state.conflict = false;
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(
    page.getByText("Отменено для вас", { exact: true }),
  ).toBeVisible();
  expect(state.requests.at(-1)).toMatchObject({
    scope: "semester",
    cancelled: true,
  });
});

for (const width of [360, 1280])
  test(`personal schedule layout at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 950 });
    await page.emulateMedia({ colorScheme: "dark" });
    await setup(page);
    await page.goto("/app");
    await expect(
      page.getByRole("heading", {
        name: "Теоретическая механика",
        exact: true,
      }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath("personal.png"),
      fullPage: true,
    });
    await page.getByRole("button", { name: /Изменить Теоретическая/ }).click();
    await expect(page.getByRole("dialog")).toBeVisible();
    const box = await page.getByRole("dialog").boundingBox();
    expect(box!.width).toBeLessThanOrEqual(width);
    await page.screenshot({
      path: testInfo.outputPath("editor.png"),
      fullPage: true,
    });
  });

test("session expiry tells user how to reopen", async ({ page }) => {
  await page.route("**/api/personal/me", (route) =>
    route.fulfill({ status: 401, json: { error: "expired" } }),
  );
  await page.goto("/app");
  await expect(
    page.getByText("Откройте приложение кнопкой «Расписание» в Telegram-боте."),
  ).toBeVisible();
});

test("select individual repeats", async ({ page }) => {
  const state = await setup(page);
  await page.goto("/app");
  await expect(
    page.getByText("Двухнедельное расписание", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: /Изменить Теоретическая/ }).click();
  await page.getByLabel("Применить", { exact: true }).selectOption("selected");
  await page.getByLabel("5 октября", { exact: true }).uncheck();
  await expect(
    page.getByRole("button", { name: "Сохранить", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("19 октября", { exact: true }).check();
  await page.getByLabel("Аудитория", { exact: true }).fill("Д-1");
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  expect(state.requests[0]).toMatchObject({
    scope: "selected",
    dates: ["2026-10-19"],
    publication: "snapshot-2",
  });
});

for (const action of ["keep", "discard"])
  test(`review updated publication: ${action}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 360, height: 950 });
    const state = await setup(page);
    state.review = true;
    await page.goto("/app");
    await expect(
      page.getByRole("heading", { name: "Вуз обновил расписание" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Изменить Теоретическая/ }),
    ).toBeDisabled();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath("review.png"),
      fullPage: true,
    });
    if (action === "discard") page.once("dialog", (dialog) => dialog.accept());
    await page
      .getByRole("button", {
        name:
          action === "keep"
            ? "Сохранить в совпавших слотах"
            : "Сбросить затронутые правки",
      })
      .click();
    await expect(
      page.getByRole("button", { name: /Изменить Теоретическая/ }),
    ).toBeEnabled();
    expect(state.reviewRequests[0]).toMatchObject({
      action,
      target_id: "group",
      publication: "snapshot-2",
      versions: { change: 2 },
    });
  });

test("administrator returns from admin to service chooser", async ({
  page,
}) => {
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith("/api/")) return route.continue();
    return route.fulfill({
      json:
        path === "/api/auth/me"
          ? {
              user: {
                id: "42",
                name: "Иван",
                role: "owner",
                auth_method: "telegram",
                csrf_token: "admin-csrf",
              },
            }
          : path === "/api/auth/config"
            ? { access_key_enabled: false }
            : { items: [] },
    });
  });
  await setup(page, true);
  await page.goto("/#/users");
  await page.getByRole("link", { name: "К выбору сервиса" }).click();
  await expect(
    page.getByRole("heading", { name: "Куда перейдём?" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /Личное расписание/ }),
  ).toBeVisible();
});
