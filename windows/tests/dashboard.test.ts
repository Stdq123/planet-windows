import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { createDashboard } from "../src/dashboard";
import type { DashboardSnapshot } from "../src/mygo";

const snapshot: DashboardSnapshot = {
  siteCount: 2, articleCount: 9, draftCount: 3, unreadCount: 4,
  sites: [{ id: "a", name: "第一站", articleCount: 9, draftCount: 0, hasPublished: true, publishPending: true }, { id: "b", name: "第二站", articleCount: 0, draftCount: 3, hasPublished: false, publishPending: false }],
  recentArticles: [{ id: "post", siteId: "a", siteName: "第一站", title: "<img src=x onerror=alert(1)>", modified: "2026-11-05T00:00:00Z" }],
  recentDrafts: [{ id: "draft", siteId: "b", siteName: "第二站", title: "继续写作", modified: "2026-11-04T00:00:00Z" }],
};
const tick = () => Bun.sleep(5);
async function fixture(overrides: Record<string, unknown> = {}) {
  const window = new Window();
  window.document.write(await Bun.file("index.html").text());
  const root = window.document.querySelector<HTMLElement>("#dashboard")!;
  const calls: unknown[][] = [];
  const controller = createDashboard(root, {
    load: async () => snapshot, serviceInfo: async () => ({ configured: false }), stats: async () => ({ visitors: 0, pageviews: 0 }),
    openSite: async (id: string) => { calls.push(["site", id]); },
    openEntry: async (kind: string, site: string, id: string) => { calls.push([kind, site, id]); },
    compose: async (id: string) => { calls.push(["compose", id]); },
    createSite: () => {}, importSite: () => {}, unread: () => {}, node: () => {}, ...overrides,
  });
  return { root, controller, calls, window, $: <T extends Element = HTMLElement>(s: string) => root.querySelector<T>(s)! };
}

test("dashboard renders real totals and safe cross-site recent actions", async () => {
  const { controller, $, calls } = await fixture(); controller.setActive(true); await tick();
  expect($("[data-count='articles']").textContent).toBe("9");
  expect($("#dashboard-articles img")).toBeNull();
  $<HTMLButtonElement>("#dashboard-drafts button").click(); await tick();
  expect(calls).toEqual([["draft", "b", "draft"]]);
  expect($("#dashboard-sites").textContent).toContain("有更改待发布");
});

test("dashboard read failures stay distinct from empty data and retry", async () => {
  let tries = 0;
  const { controller, $ } = await fixture({ load: async () => { if (++tries === 1) throw new Error("unavailable"); return snapshot; } });
  controller.setActive(true); await tick();
  expect($("#dashboard-feedback").textContent).toContain("无法读取");
  expect($("#dashboard-content").hidden).toBe(true);
  $<HTMLButtonElement>("#dashboard-refresh").click(); await tick();
  expect($("#dashboard-content").hidden).toBe(false);
});

test("late dashboard snapshots cannot overwrite a newer entry", async () => {
  let resolve!: (value: DashboardSnapshot) => void;
  let calls = 0;
  const { controller, $ } = await fixture({ load: () => ++calls === 1 ? new Promise(r => resolve = r) : Promise.resolve(snapshot) });
  controller.setActive(true); controller.setActive(false); controller.setActive(true); await tick();
  resolve({ ...snapshot, articleCount: 999 }); await tick();
  expect($("[data-count='articles']").textContent).toBe("9");
});

test("unconfigured and failed analytics never render invented zeros", async () => {
  let configured = false, queries = 0;
  const { controller, $ } = await fixture({ serviceInfo: async () => ({ configured }), stats: async () => { queries++; throw new Error("offline"); } });
  controller.setActive(true); await tick();
  $<HTMLButtonElement>("#dashboard-query").click(); await tick();
  expect($("#dashboard-analytics-result").textContent).toContain("未配置"); expect(queries).toBe(0);
  configured = true; $<HTMLButtonElement>("#dashboard-query").click(); await tick();
  expect($("#dashboard-analytics-result").textContent).toContain("查询失败");
  expect($("#dashboard-analytics-result").textContent).not.toContain("访客 0");
});

test("switching analytics site discards the previous site's late result", async () => {
  let resolve!: (value: { visitors: number; pageviews: number }) => void;
  const { controller, $, window } = await fixture({ serviceInfo: async () => ({ configured: true }), stats: () => new Promise(r => resolve = r) });
  controller.setActive(true); await tick(); $<HTMLButtonElement>("#dashboard-query").click(); await tick();
  const select = $<HTMLSelectElement>("#dashboard-analytics-site"); select.value = "b"; select.dispatchEvent(new window.Event("change"));
  resolve({ visitors: 123, pageviews: 456 }); await tick();
  expect($("#dashboard-analytics-result").textContent).toBe("尚未查询");
  expect($<HTMLButtonElement>("#dashboard-query").disabled).toBe(false);
});

test("empty overview retains create and import actions without fake recent items", async () => {
  let created = 0;
  const { controller, $ } = await fixture({ createSite: () => { created++; }, load: async () => ({ siteCount: 0, articleCount: 0, draftCount: 0, unreadCount: 0, sites: [], recentArticles: [], recentDrafts: [] }) });
  controller.setActive(true); await tick();
  expect($("#dashboard-empty").hidden).toBe(false); expect($("#dashboard-columns").hidden).toBe(true);
  expect($("[data-count='articles']").textContent).toBe("0"); expect($("#dashboard-import")).not.toBeNull();
  expect($("#dashboard-compose-controls").hidden).toBe(true);
  expect($("#dashboard-first-site").hidden).toBe(false);
  $<HTMLButtonElement>("#dashboard-first-site").click(); expect(created).toBe(1);
});
