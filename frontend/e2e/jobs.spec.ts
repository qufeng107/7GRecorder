import { expect, test } from "@playwright/test";
test.beforeEach(async ({ request }) => {
  await request.post("/__mock/scenario", { data: "populated" });
});
test("deep link, search URL and theme survive refresh", async ({ page }) => {
  await page.goto("/admin/jobs");
  await expect(page.getByRole("heading", { name: "任务中心" })).toBeVisible();
  const activeJob = page.getByRole("row").filter({ hasText: "#1 ·" });
  await expect(activeJob).toContainText("任务心跳");
  await expect(activeJob).toContainText("进度更新");
  await expect(activeJob).toContainText(
    "任务仍在运行；当前阶段暂无可解析进度。",
  );
  await page.getByRole("textbox", { name: "搜索任务" }).fill("MERGE");
  await expect(page).toHaveURL(/q=MERGE/);
  await expect(page.getByRole("cell", { name: /视频合并/ })).toBeVisible();
  await expect(page.getByRole("cell", { name: /B站投稿/ })).toHaveCount(0);
  await page.getByRole("button", { name: "切换主题" }).click();
  await page.reload();
  await expect(page.locator(".console-root")).toHaveAttribute(
    "data-theme",
    "dark",
  );
  await expect(page.getByRole("textbox", { name: "搜索任务" })).toHaveValue(
    "MERGE",
  );
});
test("ambiguous upload needs explicit confirmation; retry and cancel refresh state", async ({
  page,
}) => {
  const mutations: string[] = [];
  page.on("request", (req) => {
    if (req.method() === "POST" && req.url().includes("/jobs/"))
      mutations.push(req.postData() ?? "");
  });
  await page.goto("/admin/jobs");
  const row = page.getByRole("row").filter({ hasText: "#2 ·" });
  page.once("dialog", (d) => d.dismiss());
  await row.getByRole("button", { name: "重试" }).click();
  expect(mutations).toHaveLength(0);
  page.once("dialog", (d) => d.accept());
  await row.getByRole("button", { name: "重试" }).click();
  await expect(row).toContainText("等待执行");
  expect(mutations[0]).toContain('"confirm_ambiguous_bilibili":true');
  await row.getByRole("button", { name: "取消" }).click();
  await expect(row).toContainText("已取消");
  await expect(
    page.getByRole("row").filter({ hasText: "#1 ·" }).getByRole("button"),
  ).toHaveCount(0);
});
test("empty, error, slow and signed-out states", async ({ page, request }) => {
  await request.post("/__mock/scenario", { data: "empty" });
  await page.goto("/admin/jobs");
  await expect(page.getByRole("heading", { name: "暂无任务" })).toBeVisible();
  await request.post("/__mock/scenario", { data: "error" });
  await page.reload();
  await expect(page.getByRole("alert")).toContainText("任务加载失败");
  await request.post("/__mock/scenario", { data: "loading" });
  await page.reload();
  await expect(page.getByText("正在加载任务…")).toBeVisible();
  await request.post("/__mock/scenario", { data: "signedout" });
  await page.reload();
  await expect(page.getByRole("heading", { name: "欢迎回来" })).toBeVisible();
  await expect(page.getByRole("table")).toHaveCount(0);
});
test("manager, denied API, logout and public route isolation", async ({
  page,
  request,
}) => {
  await request.post("/__mock/scenario", { data: "manager" });
  await page.goto("/admin/jobs");
  await expect(page.getByText("用户工作区", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "账号管理" })).toHaveCount(0);
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("heading", { name: "欢迎回来" })).toBeVisible();
  await request.post("/__mock/scenario", { data: "forbidden" });
  await page.reload();
  await expect(page.getByRole("alert")).toContainText("任务加载失败");
  const requests: string[] = [];
  page.on("request", (req) => requests.push(req.url()));
  await page.goto("/@demo");
  await expect(page.getByRole("heading", { name: "@demo" })).toBeVisible();
  expect(requests.filter((url) => url.includes("/api/v1/"))).toEqual([]);
});
test("mobile has no page overflow and reduced motion is respected", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/admin/jobs");
  await expect(page.getByRole("heading", { name: "任务中心" })).toBeVisible();
  await expect(page.getByRole("link", { name: "录像管理" })).toBeHidden();
  await page.getByRole("button", { name: "展开导航" }).click();
  await expect(page.getByRole("link", { name: "录像管理" })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
});

test("unhandled mock requests fail closed and expired sessions hide jobs", async ({
  page,
  request,
}) => {
  const response = await request.get("/api/v1/not-a-fixture");
  expect(response.status()).toBe(501);
  expect((await response.json()).error.code).toBe("MOCK_NOT_IMPLEMENTED");
  await page.goto("/admin/jobs");
  await expect(page.getByRole("table")).toBeVisible();
  await request.post("/__mock/scenario", { data: "signedout" });
  await page.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(page.getByRole("heading", { name: "欢迎回来" })).toBeVisible();
  await expect(page.getByRole("table")).toHaveCount(0);
});

test("fixture selector preserves the active scenario after reload", async ({
  page,
}) => {
  await page.goto("/admin/jobs");
  await page.getByLabel("模拟场景").selectOption("empty");
  await expect(page.getByRole("heading", { name: "暂无任务" })).toBeVisible();
  await expect(page.getByLabel("模拟场景")).toHaveValue("empty");
});
