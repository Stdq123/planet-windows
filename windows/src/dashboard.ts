import type { DashboardEntry, DashboardSnapshot } from "./mygo";

interface DashboardActions {
  load(): Promise<DashboardSnapshot>;
  serviceInfo(siteID: string, kind: string): Promise<{ configured: boolean }>;
  stats(siteID: string, period: string): Promise<{ visitors: number; pageviews: number }>;
  openSite(id: string): Promise<void>;
  openEntry(kind: "draft" | "article", siteID: string, id: string): Promise<void>;
  compose(siteID: string): Promise<void>;
  search(): void;
  createSite(): void;
  importSite(): void;
  unread(): void;
  node(): void;
}

export function createDashboard(root: HTMLElement, actions: DashboardActions) {
  const doc = root.ownerDocument;
  const $ = <T extends HTMLElement = HTMLElement>(selector: string) => root.querySelector<T>(selector)!;
  let active = false, revision = 0, analyticsRevision = 0;
  let snapshot: DashboardSnapshot | null = null, articleFilter = "";
  let publicationStates: ReadonlyMap<string, { running: boolean; status: string }> = new Map();
  const compact = doc.defaultView?.matchMedia("(max-width: 1100px)");
  const icons = {
    draft: new URL("../resources/workspace/edit.svg", import.meta.url).href,
    article: new URL("../resources/workspace/file.svg", import.meta.url).href,
    more: new URL("../resources/workspace/more.svg", import.meta.url).href,
    arrow: new URL("../resources/workspace/arrow.svg", import.meta.url).href,
  };
  function updatePublicationStatuses() {
    for (const status of root.querySelectorAll<HTMLElement>("[data-site-status]")) {
      const state = publicationStates.get(status.dataset.siteStatus!);
      status.textContent = state?.running ? "正在发布…" : state?.status === "failed" ? "发布未确认" : status.dataset.defaultStatus!;
      status.dataset.tone = state?.running ? "pending" : state?.status === "failed" ? "failed" : status.dataset.defaultTone!;
    }
    $("#dashboard-publication-link").hidden = !pendingSite();
  }
  const feedback = $("#dashboard-feedback"), content = $("#dashboard-content");
  const refreshButton = $<HTMLButtonElement>("#dashboard-refresh");
  const analyticsSite = $<HTMLSelectElement>("#dashboard-analytics-site"), period = $<HTMLSelectElement>("#dashboard-period");
  const query = $<HTMLButtonElement>("#dashboard-query"), result = $("#dashboard-analytics-result");
  const composeSite = $<HTMLSelectElement>("#dashboard-compose-site");
  const composeButton = $<HTMLButtonElement>("#dashboard-compose"), composeConfirm = $<HTMLButtonElement>("#dashboard-compose-confirm");
  const composeDialog = $<HTMLDialogElement>("#dashboard-compose-dialog");
  const number = new Intl.NumberFormat("zh-CN");
  const date = (value: string) => {
    const parsed = new Date(value);
    return Number.isNaN(parsed.valueOf()) ? "日期未知" : parsed.toLocaleDateString("zh-CN", { month: "long", day: "2-digit" });
  };
  function text(tag: string, value: string, className = "") {
    const element = doc.createElement(tag); element.className = className; element.textContent = value; return element;
  }
  function icon(name: keyof typeof icons, className: string) {
    const image = doc.createElement("img"); image.src = icons[name]; image.alt = ""; image.className = className; return image;
  }
  function pendingSite() {
    return snapshot?.sites.find(site => site.publishPending || publicationStates.get(site.id)?.status === "failed");
  }
  function closeCompose() { if (composeDialog.open) composeDialog.close(); }
  function message(value: string) { feedback.textContent = value; feedback.hidden = !value; }
  async function act(action: () => Promise<void>) {
    const current = revision;
    try { message(""); await action(); }
    catch { if (active && current === revision) message("无法打开此内容，请刷新写作台后重试。"); }
  }
  function entries(selector: string, items: DashboardEntry[], kind: "draft" | "article") {
    const list = $(selector); list.replaceChildren();
    if (!items.length) {
      const empty = text("p", kind === "draft" ? "还没有草稿，写下新的想法吧。" : articleFilter ? "最近记录中没有这个站点的文章。" : "还没有文章，保存后会出现在这里。", "dashboard-list-empty");
      if (kind === "article" && articleFilter) {
        const siteID = articleFilter, button = doc.createElement("button");
        button.type = "button"; button.className = "dashboard-text-button"; button.textContent = "查看站点文章 →";
        button.addEventListener("click", () => void act(() => actions.openSite(siteID)));
        empty.append(button);
      }
      list.append(empty);
      updateListToggle(kind, 0); return;
    }
    for (const entry of items) {
      const row = doc.createElement("button"); row.type = "button"; row.className = `dashboard-row dashboard-${kind}`;
      const title = entry.title || "无标题";
      const time = text("time", date(entry.modified), "dashboard-row-date"); time.setAttribute("datetime", entry.modified);
      const parsed = new Date(entry.modified); if (!Number.isNaN(parsed.valueOf())) time.title = parsed.toLocaleString("zh-CN");
      row.title = `${title} / ${entry.siteName}`;
      row.append(icon(kind, "dashboard-row-icon"));
      if (kind === "draft") {
        const details = text("span", "", "dashboard-row-body");
        const metadata = text("span", "", "dashboard-row-meta dashboard-caption");
        metadata.append(text("span", entry.siteName), text("span", "已保存在本地"));
        details.append(text("strong", title, "dashboard-row-title"), metadata);
        row.append(details, time, icon("more", "dashboard-row-arrow"));
      } else {
        row.append(text("strong", title, "dashboard-row-title"), text("span", entry.siteName, "dashboard-row-site dashboard-caption"), time);
      }
      row.addEventListener("click", () => void act(() => actions.openEntry(kind, entry.siteId, entry.id))); list.append(row);
    }
    updateListToggle(kind, items.length);
  }
  function updateListToggle(kind: "draft" | "article", count: number) {
    const list = $(kind === "draft" ? "#dashboard-drafts" : "#dashboard-articles");
    const button = $<HTMLButtonElement>(kind === "draft" ? "#dashboard-drafts-toggle" : "#dashboard-articles-toggle");
    const expanded = list.classList.contains("is-expanded");
    button.hidden = count <= (kind === "draft" || compact?.matches ? 3 : 4);
    button.textContent = expanded ? "收起 ↑" : kind === "draft" ? "更多草稿 →" : "更多文章 →";
    button.setAttribute("aria-expanded", String(expanded));
    button.setAttribute("aria-controls", list.id);
  }
  function filteredArticles() { return snapshot?.recentArticles.filter(entry => !articleFilter || entry.siteId === articleFilter) || []; }
  function renderArticleFilters() {
    const filters = $("#dashboard-article-filters"); filters.replaceChildren();
    for (const site of [{ id: "", name: "所有站点" }, ...snapshot!.sites]) {
      const button = doc.createElement("button"); button.type = "button"; button.textContent = site.name; button.title = site.name;
      button.className = "dashboard-filter"; button.dataset.siteFilter = site.id; button.setAttribute("aria-pressed", String(articleFilter === site.id));
      button.addEventListener("click", () => {
        articleFilter = site.id; $("#dashboard-articles").classList.remove("is-expanded");
        for (const filter of filters.querySelectorAll<HTMLButtonElement>("button")) filter.setAttribute("aria-pressed", String(filter === button));
        entries("#dashboard-articles", filteredArticles(), "article");
      });
      filters.append(button);
    }
  }
  function resetAnalytics() { analyticsRevision++; query.disabled = !analyticsSite.value; result.textContent = "尚未查询"; }
  function render(data: DashboardSnapshot) {
    snapshot = data;
    if (!data.sites.some(site => site.id === articleFilter)) articleFilter = "";
    for (const [key, value] of Object.entries({ sites: data.siteCount, articles: data.articleCount, drafts: data.draftCount, unread: data.unreadCount })) {
      $(`[data-count='${key}']`).textContent = number.format(value);
    }
    $("#dashboard-empty").hidden = data.siteCount !== 0;
    $("#dashboard-columns").hidden = data.siteCount === 0;
    $("#dashboard-compose-controls").hidden = data.siteCount === 0;
    $("#dashboard-first-site").hidden = data.siteCount !== 0;
    $(".dashboard-subtitle").textContent = data.siteCount ? "继续上次的思路，也开始新的记录。" : "让想法拥有自己的地方。";
    for (const select of [composeSite, analyticsSite]) {
      const selected = select.value; select.replaceChildren();
      for (const site of data.sites) { const option = doc.createElement("option"); option.value = site.id; option.textContent = site.name; select.append(option); }
      select.value = data.sites.some(site => site.id === selected) ? selected : data.sites[0]?.id || "";
    }
    resetAnalytics();
    composeButton.disabled = composeConfirm.disabled = !composeSite.value;
    entries("#dashboard-drafts", data.recentDrafts, "draft"); renderArticleFilters(); entries("#dashboard-articles", filteredArticles(), "article");
    const list = $("#dashboard-sites"); list.replaceChildren();
    for (const site of [...data.sites].sort((a, b) => Number(b.publishPending) - Number(a.publishPending))) {
      const row = doc.createElement("button"); row.type = "button"; row.className = "dashboard-site";
      row.title = site.name;
      const state = !site.hasPublished ? "尚未发布" : site.publishPending ? "有更改待发布" : "已发布";
      const status = text("span", state, "dashboard-site-status"); status.dataset.siteStatus = site.id; status.dataset.defaultStatus = state;
      status.dataset.defaultTone = !site.hasPublished ? "unpublished" : site.publishPending ? "pending" : "published";
      const mark = text("span", Array.from(site.name.trim())[0] || "P", "dashboard-site-icon"); mark.setAttribute("aria-hidden", "true");
      const identity = text("span", "", "dashboard-site-identity");
      identity.append(mark, text("strong", site.name, "dashboard-site-name"), icon("arrow", "dashboard-site-arrow"));
      const detail = site.lastPublished ? `最近发布 ${date(site.lastPublished)}` : `${number.format(site.articleCount)} 篇文章，${number.format(site.draftCount)} 篇草稿`;
      row.append(identity, status, text("span", detail, "dashboard-site-detail dashboard-caption"));
      row.addEventListener("click", () => void act(() => actions.openSite(site.id))); list.append(row);
    }
    updatePublicationStatuses();
  }
  async function refresh() {
    if (!active) return;
    const current = ++revision; resetAnalytics();
    closeCompose(); composeButton.disabled = composeConfirm.disabled = true;
    refreshButton.disabled = true; root.setAttribute("aria-busy", "true"); message("正在读取写作台…");
    try {
      const data = await actions.load();
      if (!active || current !== revision) return;
      render(data); content.hidden = false; message("");
    } catch {
      if (!active || current !== revision) return;
      snapshot = null;
      content.hidden = true; message("无法读取写作台。请重试，已有内容不会被更改。");
    } finally {
      if (active && current === revision) { refreshButton.disabled = false; root.setAttribute("aria-busy", "false"); }
    }
  }
  refreshButton.addEventListener("click", () => void refresh());
  for (const [id, action] of [["search", actions.search], ["create", actions.createSite], ["import", actions.importSite], ["footer-import", actions.importSite]] as const) $("#dashboard-" + id).addEventListener("click", action);
  for (const [id, action] of [["unread", actions.unread], ["node", actions.node]] as const) root.querySelector("#dashboard-" + id)?.addEventListener("click", action);
  $("#dashboard-first-site").addEventListener("click", actions.createSite);
  async function composeSelectedSite() {
    const id = composeSite.value, current = revision;
    if (!active || !snapshot?.sites.some(site => site.id === id) || root.getAttribute("aria-busy") === "true") return;
    closeCompose(); composeButton.disabled = composeConfirm.disabled = true;
    await act(() => actions.compose(id));
    if (active && current === revision && snapshot) composeButton.disabled = composeConfirm.disabled = !composeSite.value;
  }
  composeButton.addEventListener("click", () => {
    if (!active || !snapshot?.sites.length || composeButton.disabled) return;
    if (snapshot.sites.length === 1) void composeSelectedSite();
    else composeDialog.showModal();
  });
  composeConfirm.addEventListener("click", () => void composeSelectedSite());
  $("#dashboard-compose-cancel").addEventListener("click", closeCompose);
  $("#dashboard-publication-link").addEventListener("click", () => { const site = pendingSite(); if (site) void act(() => actions.openSite(site.id)); });
  for (const kind of ["draft", "article"] as const) {
    $(kind === "draft" ? "#dashboard-drafts-toggle" : "#dashboard-articles-toggle").addEventListener("click", () => {
      const list = $(kind === "draft" ? "#dashboard-drafts" : "#dashboard-articles"); list.classList.toggle("is-expanded");
      updateListToggle(kind, list.querySelectorAll(".dashboard-row").length);
    });
  }
  compact?.addEventListener("change", () => updateListToggle("article", filteredArticles().length));
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
      if (!active) closeCompose();
      if (active) void refresh();
    },
    setPublicationStatus(states: ReadonlyMap<string, { running: boolean; status: string }>) {
      publicationStates = states; updatePublicationStatuses();
    },
  };
}
