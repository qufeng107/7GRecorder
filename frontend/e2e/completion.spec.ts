import { expect, test } from "@playwright/test";

test.beforeEach(async ({ request }) => {
  await request.post("/__mock/scenario", { data: "populated" });
});
test("upload modules save independently and switching profile cannot leak drafts", async ({
  page,
  request,
}) => {
  await request.post("/api/v1/recording-profiles", {
    data: { name: "第二配置", room_id: "20002", streamer_name: "第二主播" },
  });
  await page.goto("/admin/uploads");
  const profile = page.locator(".feature-page select").first();
  const title = page.getByLabel("视频标题模板", { exact: true });
  const prefix = page.getByLabel("COS 前缀", { exact: true });
  await title.fill("仅第一配置");
  await prefix.fill("first-profile/");
  await page
    .getByRole("button", { name: "保存 Bilibili 配置", exact: true })
    .click();
  await expect(
    page.getByRole("status").filter({ hasText: "已保存" }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("status").filter({ hasText: "有未保存" }),
  ).toHaveCount(1);
  await expect(prefix).toHaveValue("first-profile/");
  page.once("dialog", (dialog) => dialog.dismiss());
  await profile.selectOption("2");
  await expect(profile).toHaveValue("1");
  page.once("dialog", (dialog) => dialog.accept());
  await profile.selectOption("2");
  await expect(profile).toHaveValue("2");
  await expect(title).not.toHaveValue("仅第一配置");
  await expect(prefix).toHaveValue("demo/");
  await profile.selectOption("1");
  await expect(title).toHaveValue("仅第一配置");
  await expect(prefix).toHaveValue("demo/");
});
test("pending upload saves target the original profile and retain newer edits", async ({
  page,
}) => {
  await page.goto("/admin/uploads");
  const title = page.getByLabel("视频标题模板", { exact: true });
  await title.fill("提交快照");
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/publishing/bilibili", async (route) => {
    if (route.request().method() === "PUT") await pending;
    await route.continue();
  });
  const request = page.waitForRequest((req) => req.method() === "PUT");
  await page
    .getByRole("button", { name: "保存 Bilibili 配置", exact: true })
    .click();
  expect((await request).url()).toContain("/recording-profiles/1/");
  await expect(page.locator(".feature-page select").first()).toBeDisabled();
  await title.fill("后续编辑");
  release();
  await expect(
    page.getByRole("button", { name: "保存 Bilibili 配置", exact: true }),
  ).toBeEnabled();
  await expect(title).toHaveValue("后续编辑");
  await expect(page.getByRole("status")).toContainText("有未保存");
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
});
test("song settings retain failed saves and persist a successful draft", async ({
  page,
}) => {
  await page.goto("/admin/songs");
  const prefix = page.getByLabel("音频对象前缀", { exact: true });
  await expect(prefix).toHaveValue("songs");
  await prefix.fill("songs/local");
  await page.route("**/api/v1/song-settings", async (route) => {
    if (route.request().method() === "PUT")
      await route.fulfill({
        status: 503,
        json: { error: { code: "UNAVAILABLE" } },
      });
    else await route.continue();
  });
  await page.getByRole("button", { name: "保存识别设置", exact: true }).click();
  await expect(page.getByText(/保存识别设置失败/)).toBeVisible();
  await expect(prefix).toHaveValue("songs/local");
  await page.unroute("**/api/v1/song-settings");
  await page.getByRole("button", { name: "保存识别设置", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("已保存");
  await page.reload();
  await expect(prefix).toHaveValue("songs/local");
});
test("profile and account dialogs confirm discarding changes on Escape", async ({
  page,
}) => {
  await page.goto("/admin/profiles");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByLabel("名称", { exact: true })
    .fill("草稿");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByRole("dialog").getByLabel("名称", { exact: true }),
  ).toHaveValue("草稿");
  page.once("dialog", (dialog) => dialog.accept());
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("link", { name: "账号管理", exact: true }).click();
  await page
    .getByRole("row")
    .filter({ hasText: "viewer" })
    .getByRole("button", { name: "编辑", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByLabel("新密码", { exact: true })
    .fill("synthetic-password-draft");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("dialog").getByLabel("新密码", { exact: true }),
  ).toHaveValue("synthetic-password-draft");
  page.once("dialog", (dialog) => dialog.accept());
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
test("unapplied review cuts block approval and warn before leaving", async ({
  page,
}) => {
  await page.goto("/admin/recordings");
  await page.getByRole("button", { name: "详情", exact: true }).click();
  await page.getByLabel("删除区间", { exact: true }).fill("00:10-00:20");
  const approvals: string[] = [];
  page.on("request", (req) => {
    if (req.url().endsWith("/approve-review")) approvals.push(req.url());
  });
  await page.getByRole("button", { name: "审核完成", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("未应用的剪辑区间");
  expect(approvals).toHaveLength(0);
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
  await expect(page.getByLabel("删除区间", { exact: true })).toHaveValue(
    "00:10-00:20",
  );
});
test("credential drafts warn on navigation and clear only after successful creation", async ({
  page,
}) => {
  await page.goto("/admin/uploads");
  await page
    .getByLabel("凭证 JSON", { exact: true })
    .fill('{"synthetic":"only"}');
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
  await page.getByLabel("账号标识", { exact: true }).fill("仅供本地测试");
  await page.getByRole("button", { name: "保存凭证", exact: true }).click();
  await expect(page.getByLabel("凭证 JSON", { exact: true })).toHaveValue("{}");
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await expect(page).toHaveURL(/\/admin\/jobs$/);
});
test("all console modules fit a mobile viewport in dark mode", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/admin/jobs");
  await page.getByRole("button", { name: "切换主题" }).click();
  for (const route of [
    "profiles",
    "recordings",
    "uploads",
    "songs",
    "accounts",
    "system",
    "me",
  ]) {
    await page.goto("/admin/" + route);
    await expect(page.locator(".feature-page")).toBeVisible();
    await expect(page.locator(".console-root")).toHaveAttribute(
      "data-theme",
      "dark",
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      route,
    ).toBe(true);
  }
});
