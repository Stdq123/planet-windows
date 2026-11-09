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
    search: () => {}, createSite: () => {}, importSite: () => {}, unread: () => {}, node: () => {}, ...overrides,
  });
  return { root, controller, calls, window, $: <T extends Element = HTMLElement>(s: string) => root.querySelector<T>(s)! };
}

test("dashboard renders real totals and safe cross-site recent actions", async () => {
  const { controller, $, calls } = await fixture(); controller.setActive(true); await tick();
  expect($("[data-count='articles']").textContent).toBe("9");
  expect($("#dashboard-articles .dashboard-row-title").textContent).toBe(snapshot.recentArticles[0].title);
  expect($("#dashboard-articles img[onerror]")).toBeNull();
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

test("failed background refresh retains the last dashboard and offers a working retry", async () => {
  let tries = 0, rejectRefresh!: (reason: Error) => void;
  const { controller, root, $ } = await fixture({ load: () => {
    tries++;
    if (tries === 2) return new Promise<DashboardSnapshot>((_, reject) => { rejectRefresh = reject; });
    return Promise.resolve(tries === 1 ? snapshot : { ...snapshot, articleCount: 10 });
  } });
  controller.setActive(true); await tick();
  const draft = $("#dashboard-drafts button"), article = $("#dashboard-articles button"), site = $("#dashboard-sites button");
  const refresh = $<HTMLButtonElement>("#dashboard-refresh"), feedback = $("#dashboard-feedback");
  refresh.click();
  expect(root.getAttribute("aria-busy")).toBe("true");
  expect(refresh.disabled).toBe(true);
  expect($("#dashboard-content").hidden).toBe(false);

  rejectRefresh(new Error("offline")); await tick();
  expect($("#dashboard-content").hidden).toBe(false);
  expect($("#dashboard-drafts button")).toBe(draft);
  expect($("#dashboard-articles button")).toBe(article);
  expect($("#dashboard-sites button")).toBe(site);
  expect($("[data-count='articles']").textContent).toBe("9");
  expect(feedback.hidden).toBe(false);
  expect(feedback.getAttribute("role")).toBe("status");
  expect(feedback.textContent).toContain("请重试");
  expect(feedback.textContent).toContain("已有内容不会被更改");
  expect(root.getAttribute("aria-busy")).toBe("false");
  expect(refresh.disabled).toBe(false);
  // Filtering must still read the retained snapshot, not merely leave stale nodes on screen.
  $<HTMLButtonElement>("#dashboard-article-filters [data-site-filter='a']").click();
  expect($("#dashboard-articles .dashboard-row-title").textContent).toBe(snapshot.recentArticles[0].title);

  refresh.click(); await tick();
  expect(tries).toBe(3);
  expect($("[data-count='articles']").textContent).toBe("10");
  expect($("#dashboard-content").hidden).toBe(false);
  expect(feedback.hidden).toBe(true);
  expect(feedback.textContent).toBe("");
  expect(root.getAttribute("aria-busy")).toBe("false");
  expect(refresh.disabled).toBe(false);
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

test("writing with multiple sites requires a confirmed site and preserves the choice", async () => {
  const { controller, $, calls } = await fixture(); controller.setActive(true); await tick();
  const dialog = $<HTMLDialogElement>("#dashboard-compose-dialog");
  $<HTMLButtonElement>("#dashboard-compose").click();
  expect(dialog.open).toBe(true); expect(calls).toEqual([]);
  $<HTMLSelectElement>("#dashboard-compose-site").value = "b";
  $<HTMLButtonElement>("#dashboard-compose-cancel").click();
  expect(dialog.open).toBe(false); expect(calls).toEqual([]);
  $<HTMLButtonElement>("#dashboard-compose").click();
  expect($<HTMLSelectElement>("#dashboard-compose-site").value).toBe("b");
  $<HTMLButtonElement>("#dashboard-compose-confirm").click(); await tick();
  expect(dialog.open).toBe(false); expect(calls).toEqual([["compose", "b"]]);
});

test("writing with a single site opens the editor directly", async () => {
  const { controller, $, calls } = await fixture({ load: async () => ({ ...snapshot, siteCount: 1, sites: [snapshot.sites[0]] }) });
  controller.setActive(true); await tick(); $<HTMLButtonElement>("#dashboard-compose").click(); await tick();
  expect(calls).toEqual([["compose", "a"]]);
  expect($<HTMLDialogElement>("#dashboard-compose-dialog").open).toBe(false);
});

test("leaving the overview closes the site picker and a failed refresh disables stale writing", async () => {
  let failed = false;
  const { controller, $, calls } = await fixture({ load: async () => { if (failed) throw new Error("offline"); return snapshot; } });
  controller.setActive(true); await tick(); $<HTMLButtonElement>("#dashboard-compose").click();
  controller.setActive(false); expect($<HTMLDialogElement>("#dashboard-compose-dialog").open).toBe(false);
  failed = true; controller.setActive(true); await tick();
  expect($<HTMLButtonElement>("#dashboard-compose").disabled).toBe(true);
  $<HTMLButtonElement>("#dashboard-compose").click(); await tick(); expect(calls).toEqual([]);
});

test("site filters show only matching recent articles and recover when a site is removed", async () => {
  let data = { ...snapshot, recentArticles: [...snapshot.recentArticles, { id: "post-b", siteId: "b", siteName: "第二站", title: "另一篇文章", modified: "2026-11-01T00:00:00Z" }] };
  const { controller, $, calls } = await fixture({ load: async () => data }); controller.setActive(true); await tick();
  $<HTMLButtonElement>("#dashboard-article-filters [data-site-filter='b']").click();
  expect($("#dashboard-articles").textContent).toContain("另一篇文章");
  expect($("#dashboard-articles").textContent).not.toContain(snapshot.recentArticles[0].title);
  expect($("#dashboard-article-filters [data-site-filter='b']").getAttribute("aria-pressed")).toBe("true");
  $<HTMLButtonElement>("#dashboard-articles button").click(); await tick(); expect(calls).toEqual([["article", "b", "post-b"]]);
  data = { ...data, siteCount: 1, sites: [snapshot.sites[0]], recentArticles: snapshot.recentArticles };
  await controller.refresh();
  expect($("#dashboard-article-filters [data-site-filter='']").getAttribute("aria-pressed")).toBe("true");
  expect($("#dashboard-articles").textContent).toContain(snapshot.recentArticles[0].title);
});

test("recent lists can expand and collapse without discarding cross-site entries", async () => {
  const data = { ...snapshot, recentArticles: Array.from({ length: 6 }, (_, i) => ({ ...snapshot.recentArticles[0], id: `post-${i}`, title: `文章 ${i}` })), recentDrafts: Array.from({ length: 6 }, (_, i) => ({ ...snapshot.recentDrafts[0], id: `draft-${i}` })) };
  const { controller, $ } = await fixture({ load: async () => data }); controller.setActive(true); await tick();
  for (const kind of ["drafts", "articles"]) {
    const button = $<HTMLButtonElement>(`#dashboard-${kind}-toggle`), list = $(`#dashboard-${kind}`);
    expect(button.hidden).toBe(false); expect(list.children.length).toBe(6);
    button.click(); expect(list.classList.contains("is-expanded")).toBe(true); expect(button.getAttribute("aria-expanded")).toBe("true");
    button.click(); expect(list.classList.contains("is-expanded")).toBe(false);
  }
});

test("a site absent from the global recent snapshot still offers its full article list", async () => {
  const data = { ...snapshot, articleCount: 14, sites: [snapshot.sites[0], { ...snapshot.sites[1], articleCount: 5 }], recentArticles: Array.from({ length: 6 }, (_, i) => ({ ...snapshot.recentArticles[0], id: `recent-${i}` })) };
  const { controller, $, calls } = await fixture({ load: async () => data }); controller.setActive(true); await tick();
  $<HTMLButtonElement>("#dashboard-article-filters [data-site-filter='b']").click();
  expect($("#dashboard-articles .dashboard-list-empty").textContent).toContain("最近记录中没有这个站点的文章。");
  const openSite = $<HTMLButtonElement>("#dashboard-articles .dashboard-list-empty .dashboard-text-button");
  expect(openSite.textContent).toBe("查看站点文章 →");
  $<HTMLButtonElement>("#dashboard-article-filters [data-site-filter='']").click();
  openSite.click(); await tick();
  expect(calls).toEqual([["site", "b"]]);
});

test("publication feedback follows live state and pending action opens the correct site", async () => {
  const { controller, $, calls } = await fixture(); controller.setActive(true); await tick();
  controller.setPublicationStatus(new Map([["a", { running: true, status: "idle" }]]));
  expect($("[data-site-status='a']").textContent).toBe("正在发布…");
  controller.setPublicationStatus(new Map([["a", { running: false, status: "failed" }]]));
  expect($("[data-site-status='a']").textContent).toBe("发布未确认");
  expect($("[data-site-status='a']").getAttribute("data-tone")).toBe("failed");
  $<HTMLButtonElement>("#dashboard-publication-link").click(); await tick(); expect(calls).toEqual([["site", "a"]]);
  expect($("#dashboard-drafts").textContent).toContain("已保存在本地");
  expect($("#dashboard-drafts").textContent).not.toContain("自动保存");
});
