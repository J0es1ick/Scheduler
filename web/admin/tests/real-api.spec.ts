import { expect, test } from "@playwright/test";
test("real API: mobile edit preserves cycle, preview, conflict recovery and logout", async ({
  page,
}) => {
  test.skip(
    process.env.SCHEDULER_REAL_E2E !== "1",
    "requires isolated browser fixture",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#/editor");
  await page.getByLabel("Ключ доступа").fill("release-browser-fixture-key");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Группа", exact: true })
    .fill("ТЕСТ-101");
  await page.getByRole("option", { name: /ТЕСТ-101/ }).click();
  await expect(
    page.getByRole("heading", { name: "Группа ТЕСТ-101" }),
  ).toBeVisible();
  const before = await (
    await page.request.get("/api/editor/schedule?group=release-group")
  ).json();
  await page
    .getByRole("button", { name: "Редактировать", exact: true })
    .click();
  await expect(page.getByRole("combobox", { name: "Повторение" })).toHaveValue(
    "cycle",
  );
  await page.getByRole("button", { name: "Рассчитать даты занятий" }).click();
  await expect(page.getByRole("status")).toContainText("2026-09-16");
  const mine = `Тест-${Date.now()}`;
  await page.getByLabel("Аудитория", { exact: true }).fill(mine);
  const identity = await (await page.request.get("/api/auth/me")).json();
  const original = before.lessons[0];
  const fields = [
    "group_id",
    "semester_id",
    "day_of_week",
    "time_start",
    "time_end",
    "week_type",
    "subject",
    "type",
    "teacher",
    "room",
    "subgroup",
    "valid_from",
    "valid_to",
    "recurrence",
  ];
  const payload = Object.fromEntries(fields.map((key) => [key, original[key]]));
  for (const key of ["valid_from", "valid_to", "special_date"])
    payload[key] = original[key]?.slice(0, 10) ?? "";
  const concurrent = await page.request.put(
    `/api/editor/lessons/${encodeURIComponent(original.id)}`,
    {
      headers: { "X-CSRF-Token": identity.user.csrf_token },
      data: {
        ...payload,
        room: "Серверная аудитория",
        expected_updated_at: original.updated_at,
      },
    },
  );
  expect(concurrent.ok(), await concurrent.text()).toBe(true);
  await page.getByRole("button", { name: "Проверить изменения" }).click();
  await page.getByRole("button", { name: "Подтвердить и применить" }).click();
  await expect(
    page.getByText("Запись изменилась на сервере. Ваш ввод сохранён."),
  ).toBeVisible();
  await expect(
    page.getByRole("cell", { name: mine, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "Серверная аудитория", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: "Сравнение выполнено — продолжить с моим вводом",
    })
    .click();
  await expect(page.getByLabel("Аудитория", { exact: true })).toHaveValue(mine);
  await page.getByRole("button", { name: "Проверить изменения" }).click();
  await page.getByRole("button", { name: "Подтвердить и применить" }).click();
  await expect(page.getByText("Занятие обновлено")).toBeVisible();
  const after = await (
    await page.request.get("/api/editor/schedule?group=release-group")
  ).json();
  expect(after.lessons[0].recurrence).toEqual(before.lessons[0].recurrence);
  const calendar = await page.request.get(
    "/api/editor/calendar?group=release-group&from=2026-09-02&days=112",
  );
  expect(calendar.ok()).toBe(true);
  expect((await calendar.json()).content).toContain("20260916");
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Вход в админку" }),
  ).toBeVisible();
  expect((await page.request.get("/api/auth/me")).status()).toBe(401);
});
