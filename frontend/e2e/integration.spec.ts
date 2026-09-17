import { expect, test } from "@playwright/test";
test("built frontend authenticates against a fresh isolated backend", async ({
  page,
  request,
}) => {
  expect((await request.get("/api/v1/jobs")).status()).toBe(401);
  await page.goto("/admin/jobs");
  await page.getByLabel("用户名", { exact: true }).fill("local-admin");
  await page
    .getByLabel("密码", { exact: true })
    .fill("local-test-only-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "任务中心" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "暂无任务" })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "任务中心" })).toBeVisible();
  await expect(page.getByText("本地模拟", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("heading", { name: "欢迎回来" })).toBeVisible();
});

test("migrated modules work against real contracts; profile and account changes persist", async ({
  page,
}) => {
  await page.goto("/admin/profiles");
  await page.getByLabel("用户名", { exact: true }).fill("local-admin");
  await page
    .getByLabel("密码", { exact: true })
    .fill("local-test-only-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.getByRole("button", { name: "新建", exact: true }).click();
  const profile = page.getByRole("dialog");
  await profile.getByLabel("名称", { exact: true }).fill("隔离环境录制配置");
  await profile.getByLabel("直播间 ID", { exact: true }).fill("12345678");
  await profile.getByLabel("主播", { exact: true }).fill("测试主播");
  await profile.getByLabel("自动录制", { exact: true }).uncheck();
  await profile.getByLabel("启用", { exact: true }).uncheck();
  await profile.getByRole("button", { name: "创建", exact: true }).click();
  await expect(profile).toHaveCount(0);
  await page.reload();
  await expect(
    page.getByRole("button", { name: "隔离环境录制配置", exact: true }),
  ).toBeVisible();
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  for (const route of [
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
  await page.goto("/admin/uploads");
  await page
    .getByLabel("视频标题模板", { exact: true })
    .fill("隔离测试 {title}");
  await page
    .getByRole("button", { name: "保存 Bilibili 配置", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "保存 Bilibili 配置", exact: true }),
  ).toBeEnabled();
  await page.reload();
  await expect(page.getByLabel("视频标题模板", { exact: true })).toHaveValue(
    "隔离测试 {title}",
  );
  const created = await page.request.post("/api/v1/accounts", {
    data: {
      username: "isolated-viewer",
      password: "synthetic-viewer-password",
      role: "MANAGER",
      enabled: true,
    },
  });
  expect(created.ok()).toBeTruthy();
  await page.goto("/admin/accounts");
  await page
    .getByRole("row")
    .filter({ hasText: "isolated-viewer" })
    .getByRole("button", { name: "编辑", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "保存账号", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("heading", { name: "欢迎回来" })).toBeVisible();
  await page.getByLabel("用户名", { exact: true }).fill("isolated-viewer");
  await page
    .getByLabel("密码", { exact: true })
    .fill("synthetic-viewer-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("你没有访问此页面的权限");
  expect((await page.request.get("/api/v1/accounts")).status()).toBe(403);
  expect(errors).toEqual([]);
});

test("storage draft saves and reloads against the isolated backend", async ({
  page,
}) => {
  await page.goto("/admin/system");
  await page.getByLabel("用户名", { exact: true }).fill("local-admin");
  await page
    .getByLabel("密码", { exact: true })
    .fill("local-test-only-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.getByLabel("录像上限 GB", { exact: true }).fill("10");
  await page.getByLabel("最低空闲 GB", { exact: true }).fill("2");
  await page.getByLabel("紧急保留 GB", { exact: true }).fill("1");
  await page.getByLabel("清理目标 %", { exact: true }).fill("85");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "存储设置" }),
  ).toContainText("已保存");
  await page.reload();
  await expect(page.getByLabel("录像上限 GB", { exact: true })).toHaveValue(
    "10",
  );
  await page.getByLabel("录像上限 GB", { exact: true }).fill("12");
  await page.getByRole("link", { name: "任务中心", exact: true }).click();
  await page.getByRole("button", { name: "继续编辑", exact: true }).click();
  await expect(page.getByLabel("录像上限 GB", { exact: true })).toHaveValue(
    "12",
  );
});
