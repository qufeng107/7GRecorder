import { expect, test } from "@playwright/test";
test.setTimeout(60000);
test.beforeEach(async ({ request }) => {
  await request.post("/__mock/scenario", { data: "populated" });
});
test("system draft survives polling, warns before leaving, and discards to fresh server state", async ({
  page,
  request,
}) => {
  await page.goto("/admin/system");
  const quota = page.getByLabel("录像上限 GB", { exact: true });
  await expect(quota).toHaveValue("10");
  await quota.fill("12");
  await request.put("/api/v1/storage/local/settings", {
    data: { max_recording_bytes: 20 * 1024 ** 3 },
  });
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "放弃未保存的修改？" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "继续编辑" }).click();
  await expect(quota).toHaveValue("12");
  expect(
    await page.evaluate(() => {
      const event = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    }),
  ).toBe(true);
  await page.waitForResponse(
    (res) =>
      res.url().endsWith("/api/v1/storage/local") &&
      res.request().method() === "GET",
    { timeout: 35000 },
  );
  await expect(quota).toHaveValue("12");
  await page.getByRole("button", { name: "放弃修改", exact: true }).click();
  await expect(quota).toHaveValue("20");
  await quota.fill("21");
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await dialog.getByRole("button", { name: "放弃并离开" }).click();
  await expect(page).toHaveURL(/\/admin\/jobs$/);
  await page.getByRole("link", { name: "系统设置", exact: true }).click();
  await expect(quota).toHaveValue("20");
});
test("failed saves retain drafts and successful older saves do not overwrite newer edits", async ({
  page,
}) => {
  await page.goto("/admin/system");
  const quota = page.getByLabel("录像上限 GB", { exact: true });
  await expect(quota).toHaveValue("10");
  await page.route("**/api/v1/storage/local/settings", (route) =>
    route.fulfill({
      status: 500,
      json: { error: { code: "SYNTHETIC_FAILURE" } },
    }),
  );
  await quota.fill("12");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByText("存储设置保存失败，请检查阈值。")).toBeVisible();
  await expect(quota).toHaveValue("12");
  await page.unroute("**/api/v1/storage/local/settings");
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/storage/local/settings", async (route) => {
    await gate;
    await route.continue();
  });
  const started = page.waitForRequest((req) =>
    req.url().endsWith("/storage/local/settings"),
  );
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await started;
  await quota.fill("14");
  release();
  await expect(
    page.getByRole("button", { name: "保存", exact: true }),
  ).toBeEnabled();
  await expect(quota).toHaveValue("14");
  await expect(page.getByRole("status")).toContainText("有未保存的修改");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("已保存");
  expect(
    await page.evaluate(() => {
      const event = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    }),
  ).toBe(false);
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page).toHaveURL(/\/admin\/jobs$/);
});

test("TLS and storage drafts save independently and secrets stay memory-only", async ({
  page,
  request,
}) => {
  await page.goto("/admin/system");
  await page.getByLabel("录像上限 GB", { exact: true }).fill("12");
  await page.getByLabel("启用", { exact: true }).check();
  await request.put("/api/v1/system/site-tls", {
    data: { primary_domain: "updated.invalid" },
  });
  await page.waitForResponse(
    (res) =>
      res.url().endsWith("/api/v1/system/site-tls") &&
      res.request().method() === "GET",
  );
  await expect(page.getByLabel("启用", { exact: true })).toBeChecked();
  await page
    .getByRole("button", { name: "保存并排队同步", exact: true })
    .click();
  await expect(
    page.getByRole("status").filter({ hasText: "已保存" }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("status").filter({ hasText: "有未保存的修改" }),
  ).toHaveCount(1);
  await expect(page.getByLabel("录像上限 GB", { exact: true })).toHaveValue(
    "12",
  );
  await page.getByRole("button", { name: "放弃修改", exact: true }).click();
  await page
    .locator("textarea")
    .last()
    .fill('{"secret_id":"synthetic-only","secret_key":"synthetic-only"}');
  const stored = await page.evaluate(() =>
    JSON.stringify({
      local: { ...localStorage },
      session: { ...sessionStorage },
    }),
  );
  expect(stored).not.toContain("synthetic-only");
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
});

test("background query failures retain editable drafts and the navigation guard", async ({
  page,
}) => {
  await page.goto("/admin/system");
  await page.getByLabel("录像上限 GB", { exact: true }).fill("12");
  await page.route("**/api/v1/system/site-tls", (route) =>
    route.fulfill({ status: 503, json: { error: { code: "UNAVAILABLE" } } }),
  );
  await expect(page.getByRole("alert")).toContainText("后台刷新失败", {
    timeout: 15000,
  });
  await expect(page.getByLabel("录像上限 GB", { exact: true })).toHaveValue(
    "12",
  );
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
});
