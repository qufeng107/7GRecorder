import { expect, test } from "@playwright/test";

const session = {
  id: 7, recording_profile_id: 1, source: "BILIBILI_OPEN_LIVE",
  status: "ENDED", room_id: "123", anchor_name: "测试主播",
  raw_status: "AVAILABLE", raw_size_bytes: 500, started_at: "2026-09-19T01:00:00Z",
  event_count: 3, unknown_event_count: 0, gap_count: 1,
  event_counts: { LIVE_OPEN_PLATFORM_DM: 3 },
};

test("capture evidence paginates and retains minute totals after raw cleanup", async ({ page }) => {
  let cleaned = false;
  await page.route("**/api/v1/live-analytics/sessions/7", async route => route.fulfill({
    json: { ...session, raw_status: cleaned ? "DELETED" : "AVAILABLE", raw_deleted_at: cleaned ? "2026-09-19T03:00:00Z" : "" },
  }));
  await page.route("**/api/v1/live-analytics/sessions/7/raw-files", async route => route.fulfill({
    json: { items: [
      { id: 2, status: cleaned ? "DELETED" : "AVAILABLE", size_bytes: 500, created_at: session.started_at },
      { id: 1, status: "AVAILABLE", size_bytes: 500, created_at: session.started_at },
    ], truncated: false },
  }));
  await page.route("**/api/v1/live-analytics/sessions/7/timeline", async route => route.fulfill({
    json: { items: [{ minute: "2026-09-19T01:00:00Z", cmd: "LIVE_OPEN_PLATFORM_DM", event_count: 3 }], truncated: false },
  }));
  await page.route("**/api/v1/live-analytics/sessions/7/events?*", async route => {
    const next = new URL(route.request().url()).searchParams.get("offset") === "200";
    await route.fulfill({ json: {
      items: [{ received_at: next ? "2026-09-19T01:00:20Z" : "2026-09-19T01:00:10Z", cmd: "LIVE_OPEN_PLATFORM_DM", payload: { data: { msg: next ? "第二页弹幕" : "第一条弹幕" } } }],
      next_offset: next ? 400 : 200, has_more: !next,
    } });
  });
  await page.goto("/admin/live-analytics/sessions/7");
  await expect(page.getByRole("heading", { name: "采集事件 #7" })).toBeVisible();
  await expect(page.getByRole("img", { name: "每分钟接收事件柱状图" })).toBeVisible();
  await page.locator("summary").filter({ hasText: "LIVE_OPEN_PLATFORM_DM" }).click();
  await expect(page.getByText(/第一条弹幕/)).toBeVisible();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await page.locator("summary").filter({ hasText: "LIVE_OPEN_PLATFORM_DM" }).click();
  await expect(page.getByText(/第二页弹幕/)).toBeVisible();
  await expect(page.getByRole("button", { name: "下一页", exact: true })).toBeDisabled();
  await page.getByLabel("原文分片", { exact: true }).selectOption("1");
  await expect(page.getByRole("button", { name: "上一页", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "下一页", exact: true })).toBeEnabled();
  cleaned = true;
  await page.reload();
  await expect(page.getByText(/原始证据尚未生成、已清理或已缺失/)).toBeVisible();
  await expect(page.getByText("DM: 3", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "下一页", exact: true })).toHaveCount(0);
});
