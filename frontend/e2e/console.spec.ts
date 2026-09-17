import { expect, test } from "@playwright/test";

test.beforeEach(async ({ request }) => {
  await request.post("/__mock/scenario", { data: "populated" });
});

test("every module renders without fetching unrelated account or settings data", async ({
  page,
}) => {
  const failures: string[] = [];
  page.on("pageerror", (error) => failures.push(error.message));
  for (const route of [
    "profiles",
    "recordings",
    "uploads",
    "songs",
    "system",
    "accounts",
    "me",
  ]) {
    await page.goto("/admin/" + route);
    await expect(page.locator(".feature-page")).toBeVisible();
    await expect(page.getByRole("alert")).toHaveCount(0);
  }
  const calls: string[] = [];
  page.on("request", (req) => {
    if (new URL(req.url()).pathname.startsWith("/api/")) calls.push(req.url());
  });
  await page.goto("/admin/recordings");
  await expect(
    page.getByRole("button", { name: "详情", exact: true }),
  ).toBeVisible();
  expect(
    calls.filter((url) =>
      /accounts|credentials|site-tls|song-settings/.test(url),
    ),
  ).toEqual([]);
  expect(failures).toEqual([]);
});

test("profile create/edit preserves draft across background refresh", async ({
  page,
  request,
}) => {
  await page.goto("/admin/profiles");
  await page.getByRole("button", { name: "新建", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("名称", { exact: true }).fill("测试录制");
  await dialog.getByLabel("直播间 ID", { exact: true }).fill("123456");
  await dialog.getByLabel("主播", { exact: true }).fill("测试主播");
  await dialog.getByRole("button", { name: "创建", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const row = page.getByRole("row").filter({ hasText: "测试录制" });
  await row.getByRole("button", { name: "编辑", exact: true }).click();
  await dialog.getByLabel("名称", { exact: true }).fill("未保存草稿");
  await request.patch("/api/v1/recording-profiles/2", {
    data: { streamer_name: "服务端更新" },
  });
  await page.waitForResponse(
    (res) =>
      res.url().endsWith("/api/v1/recording-profiles") &&
      res.request().method() === "GET",
    { timeout: 15000 },
  );
  await expect(dialog.getByLabel("名称", { exact: true })).toHaveValue(
    "未保存草稿",
  );
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(
    page.getByRole("row").filter({ hasText: "未保存草稿" }),
  ).toBeVisible();
});

test("review downloads live inside details; editing never approves a source", async ({
  page,
}) => {
  const actions: string[] = [];
  page.on("request", (req) => {
    if (req.method() === "POST") actions.push(req.url());
  });
  await page.goto("/admin/recordings");
  await expect(
    page.getByRole("button", { name: "本地下载", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "详情", exact: true }).click();
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "本地下载", exact: true }).click();
  expect((await download).suggestedFilename()).toBe("synthetic-review.txt");
  await page.getByLabel("删除区间", { exact: true }).fill("00:10-00:20");
  await page.getByRole("button", { name: "应用剪辑", exact: true }).click();
  await expect
    .poll(() => actions.filter((url) => url.endsWith("/apply-edit")).length)
    .toBe(1);
  expect(actions.some((url) => url.endsWith("/approve-review"))).toBe(false);
  await page.getByRole("button", { name: "审核完成", exact: true }).click();
  await expect(page.getByRole("alert")).toBeVisible();
});

test("account update omits empty password and cleanup needs confirmation", async ({
  page,
}) => {
  await page.goto("/admin/accounts");
  await page
    .getByRole("row")
    .filter({ hasText: "viewer" })
    .getByRole("button", { name: "编辑", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("新密码", { exact: true })).toHaveValue("");
  const update = page.waitForRequest(
    (req) => req.url().endsWith("/accounts/2") && req.method() === "PATCH",
  );
  await dialog.getByRole("button", { name: "保存账号", exact: true }).click();
  expect((await update).postDataJSON()).not.toHaveProperty("password");
  await expect(dialog).toHaveCount(0);
  await page.goto("/admin/system");
  const cleanups: string[] = [];
  page.on("request", (req) => {
    if (req.url().endsWith("/actions/cleanup")) cleanups.push(req.method());
  });
  page.once("dialog", (d) => d.dismiss());
  await page.getByRole("button", { name: "执行清理", exact: true }).click();
  expect(cleanups).toEqual([]);
  page.once("dialog", (d) => d.accept());
  await page.getByRole("button", { name: "执行清理", exact: true }).click();
  await expect(page.getByText(/清理完成：/)).toBeVisible();
  expect(cleanups).toEqual(["POST"]);
});

test("manager deep links cannot mount restricted modules", async ({
  page,
  request,
}) => {
  await request.post("/__mock/scenario", { data: "manager" });
  const calls: string[] = [];
  page.on("request", (req) => {
    if (new URL(req.url()).pathname.startsWith("/api/")) calls.push(req.url());
  });
  for (const route of ["accounts", "system", "songs", "uploads"]) {
    await page.goto("/admin/" + route);
    await expect(page.getByRole("alert")).toContainText(
      "你没有访问此页面的权限",
    );
  }
  expect(
    calls.filter(
      (url) => !url.endsWith("/me") && !url.endsWith("/system/health"),
    ),
  ).toEqual([]);
  await page.goto("/admin/profiles");
  await expect(
    page.getByRole("button", { name: "晚风电台", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "新建", exact: true }),
  ).toHaveCount(0);
});

test("migrated routes handle loading, failures, empty data and mobile theme", async ({
  page,
  request,
}) => {
  await request.post("/__mock/scenario", { data: "error" });
  await page.goto("/admin/profiles");
  await expect(page.getByRole("alert")).toContainText("页面数据加载失败");
  await request.post("/__mock/scenario", { data: "loading" });
  await page.reload();
  await expect(page.getByRole("status")).toContainText("正在加载");
  await request.post("/__mock/scenario", { data: "empty" });
  await page.goto("/admin/recordings");
  await expect(page.locator(".feature-page")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "切换主题" }).click();
  await expect(page.locator(".console-root")).toHaveAttribute(
    "data-theme",
    "dark",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
