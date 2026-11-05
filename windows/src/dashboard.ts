import type { DashboardEntry, DashboardSnapshot } from "./mygo";

interface DashboardActions {
  load(): Promise<DashboardSnapshot>;
  serviceInfo(siteID: string, kind: string): Promise<{ configured: boolean }>;
  stats(siteID: string, period: string): Promise<{ visitors: number; pageviews: number }>;
  openSite(id: string): Promise<void>;
  openEntry(kind: "draft" | "article", siteID: string, id: string): Promise<void>;
  compose(siteID: string): Promise<void>;
  createSite(): void;
  importSite(): void;
  unread(): void;
  node(): void;
}

export function createDashboard(root: HTMLElement, actions: DashboardActions) {
  const doc = root.ownerDocument;
  const $ = <T extends HTMLElement = HTMLElement>(selector: string) => root.querySelector<T>(selector)!;
  let active = false, revision = 0, analyticsRevision = 0;
  let publicationStates: ReadonlyMap<string, { running: boolean; status: string }> = new Map();
  function updatePublicationStatuses() {
    for (const status of root.querySelectorAll<HTMLElement>("[data-site-status]")) {
      const state = publicationStates.get(status.dataset.siteStatus!);
      status.textContent = state?.running ? "正在发布…" : state?.status === "failed" ? "发布未确认" : status.dataset.defaultStatus!;
      status.dataset.tone = state?.running ? "pending" : state?.status === "failed" ? "failed" : status.dataset.defaultTone!;
    }
  }
  const feedback = $("#dashboard-feedback"), content = $("#dashboard-content");
  const refreshButton = $<HTMLButtonElement>("#dashboard-refresh");
  const analyticsSite = $<HTMLSelectElement>("#dashboard-analytics-site"), period = $<HTMLSelectElement>("#dashboard-period");
  const query = $<HTMLButtonElement>("#dashboard-query"), result = $("#dashboard-analytics-result");
  const composeSite = $<HTMLSelectElement>("#dashboard-compose-site");
  const number = new Intl.NumberFormat("zh-CN");
  const date = (value: string) => {
    const parsed = new Date(value);
    return Number.isNaN(parsed.valueOf()) ? "日期未知" : parsed.toLocaleDateString("zh-CN", { month: "short", day: "numeric", year: "numeric" });
  };
  function text(tag: string, value: string, className = "") {
    const element = doc.createElement(tag); element.className = className; element.textContent = value; return element;
  }
  function message(value: string) { feedback.textContent = value; feedback.hidden = !value; }
  async function act(action: () => Promise<void>) {
    const current = revision;
    try { message(""); await action(); }
    catch { if (active && current === revision) message("无法打开此内容，请刷新概览后重试。"); }
  }
  function entries(selector: string, items: DashboardEntry[], kind: "draft" | "article") {
    const list = $(selector); list.replaceChildren();
    if (!items.length) { list.append(text("p", kind === "draft" ? "还没有草稿。开始写作后，可以保存草稿并随时继续。" : "还没有文章。保存的文章会出现在这里。", "dashboard-list-empty")); return; }
    for (const entry of items) {
      const row = doc.createElement("button"); row.type = "button"; row.className = `dashboard-row dashboard-${kind}`;
      const icon = text("span", Array.from(entry.siteName.trim())[0] || "P", "dashboard-row-icon"); icon.setAttribute("aria-hidden", "true");
      const details = text("span", "", "dashboard-row-body");
      const title = entry.title || "无标题";
      details.append(text("strong", title, "dashboard-row-title"), text("span", entry.siteName, "dashboard-caption"));
      const time = text("time", date(entry.modified), "dashboard-row-date"); time.setAttribute("datetime", entry.modified);
      const arrow = text("span", "↗", "dashboard-row-arrow"); arrow.setAttribute("aria-hidden", "true");
      row.title = `${title} / ${entry.siteName}`; row.append(icon, details, time, arrow);
      row.addEventListener("click", () => void act(() => actions.openEntry(kind, entry.siteId, entry.id))); list.append(row);
    }
  }
  function resetAnalytics() { analyticsRevision++; query.disabled = !analyticsSite.value; result.textContent = "尚未查询"; }
  function render(data: DashboardSnapshot) {
    for (const [key, value] of Object.entries({ sites: data.siteCount, articles: data.articleCount, drafts: data.draftCount, unread: data.unreadCount })) {
      $(`[data-count='${key}']`).textContent = number.format(value);
    }
    $("#dashboard-empty").hidden = data.siteCount !== 0;
    $("#dashboard-columns").hidden = data.siteCount === 0;
    $("#dashboard-compose-controls").hidden = data.siteCount === 0;
    $("#dashboard-first-site").hidden = data.siteCount !== 0;
    $("#dashboard-welcome-title").textContent = data.siteCount ? "今天，写点什么？" : "你的故事，从这里开始。";
    $("#dashboard-welcome-description").textContent = data.siteCount ? "继续未完成的故事，或记录一个新发现。" : "建一个属于自己的站点，让日常、灵感与热爱有处安放。";
    for (const select of [composeSite, analyticsSite]) {
      const selected = select.value; select.replaceChildren();
      for (const site of data.sites) { const option = doc.createElement("option"); option.value = site.id; option.textContent = site.name; select.append(option); }
      if (data.sites.some(site => site.id === selected)) select.value = selected;
    }
    resetAnalytics();
    $<HTMLButtonElement>("#dashboard-compose").disabled = !composeSite.value;
    entries("#dashboard-drafts", data.recentDrafts, "draft"); entries("#dashboard-articles", data.recentArticles, "article");
    const list = $("#dashboard-sites"); list.replaceChildren();
    for (const site of data.sites) {
      const row = doc.createElement("button"); row.type = "button"; row.className = "dashboard-site";
      const state = !site.hasPublished ? "尚未发布" : site.publishPending ? "有更改待发布" : "已发布";
      const status = text("span", state, "dashboard-site-status"); status.dataset.siteStatus = site.id; status.dataset.defaultStatus = state;
      status.dataset.defaultTone = !site.hasPublished ? "unpublished" : site.publishPending ? "pending" : "published";
      const icon = text("span", Array.from(site.name.trim())[0] || "P", "dashboard-site-icon"); icon.setAttribute("aria-hidden", "true");
      const details = text("span", "", "dashboard-site-body");
      details.append(text("strong", site.name), status, text("span", `${site.articleCount} 篇文章 · ${site.draftCount} 份草稿`, "dashboard-caption"));
      if (site.lastPublished) details.append(text("span", `最近发布 ${date(site.lastPublished)}`, "dashboard-caption"));
      row.append(icon, details);
      row.addEventListener("click", () => void act(() => actions.openSite(site.id))); list.append(row);
    }
    updatePublicationStatuses();
  }
  async function refresh() {
    if (!active) return;
    const current = ++revision; resetAnalytics();
    refreshButton.disabled = true; root.setAttribute("aria-busy", "true"); message("正在读取本地概览…");
    try {
      const data = await actions.load();
      if (!active || current !== revision) return;
      render(data); content.hidden = false; message("");
    } catch {
      if (!active || current !== revision) return;
      content.hidden = true; message("无法读取工作区概览。请重试，已有内容不会被更改。");
    } finally {
      if (active && current === revision) { refreshButton.disabled = false; root.setAttribute("aria-busy", "false"); }
    }
  }
  refreshButton.addEventListener("click", () => void refresh());
  for (const [id, action] of [["create", actions.createSite], ["import", actions.importSite], ["unread", actions.unread], ["node", actions.node]] as const) $("#dashboard-" + id).addEventListener("click", action);
  $("#dashboard-first-site").addEventListener("click", actions.createSite);
  $("#dashboard-compose").addEventListener("click", () => { if (composeSite.value) void act(() => actions.compose(composeSite.value)); });
  analyticsSite.addEventListener("change", resetAnalytics); period.addEventListener("change", resetAnalytics);
  query.addEventListener("click", async () => {
    const siteID = analyticsSite.value, range = period.value, current = ++analyticsRevision;
    if (!siteID) return;
    const ownsResult = () => active && current === analyticsRevision;
    query.disabled = true; result.textContent = "正在查询…";
    try {
      const info = await actions.serviceInfo(siteID, "plausible");
      if (!ownsResult()) return;
      if (!info.configured) { result.textContent = "此站点未配置统计 API，请在站点服务中配置。"; return; }
      const stats = await actions.stats(siteID, range);
      if (ownsResult()) result.textContent = `访客 ${number.format(stats.visitors)}，浏览量 ${number.format(stats.pageviews)}`;
    } catch { if (ownsResult()) result.textContent = "查询失败，请检查统计服务与网络后重试。"; }
    finally { if (ownsResult()) query.disabled = false; }
  });
  return {
    refresh,
    setActive(value: boolean) {
      root.hidden = !value;
      if (active === value) return;
      active = value; revision++; resetAnalytics();
      if (active) void refresh();
    },
    setPublicationStatus(states: ReadonlyMap<string, { running: boolean; status: string }>) {
      publicationStates = states; updatePublicationStatuses();
    },
  };
}
