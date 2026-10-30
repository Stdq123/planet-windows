import { Library, type Article, type Site, type Subscription, type Item } from "./mygo";

const $ = <T extends HTMLElement>(selector: string): T => {
  const element = document.querySelector<T>(selector);
  if (!element) throw new Error(`Missing ${selector}`);
  return element;
};
const sitesEl = $("#sites"), articlesEl = $("#articles");
const siteTitle = $("#site-title"), count = $("#post-count"), workspaceTitle = $("#workspace-title");
const empty = $("#empty"), editor = $("#editor"), errorEl = $("#error"), saveState = $("#save-state");
const title = $<HTMLInputElement>("#title"), content = $<HTMLTextAreaElement>("#content");
const preview = $<HTMLIFrameElement>("#preview"), saveButton = $<HTMLButtonElement>("#save");
const newArticle = $<HTMLButtonElement>("#new-article"), dialog = $<HTMLDialogElement>("#site-dialog");
const form = $<HTMLFormElement>("#site-form"), emptyAction = $<HTMLButtonElement>("#empty-action");
const subscriptionsEl = $("#subscriptions"), feedDialog = $<HTMLDialogElement>("#feed-dialog"), feedForm = $<HTMLFormElement>("#feed-form");
const reader = $("#reader"), readerTitle = $("#reader-title"), readerDate = $("#reader-date"), readerPreview = $<HTMLIFrameElement>("#reader-preview");
const refreshFeed = $<HTMLButtonElement>("#refresh-feed");
const exportSite = $<HTMLButtonElement>("#export-site");
const publishSite = $<HTMLButtonElement>("#publish-site");
const exportPlanet = $<HTMLButtonElement>("#export-planet"), importPlanet = $<HTMLButtonElement>("#import-planet");
const selectTemplate = $<HTMLButtonElement>("#select-template");
const deleteArticle = $<HTMLButtonElement>("#delete-article");
const siteAddress = $("#site-address");
const publishedAddress = $("#published-address"), publishedURL = $<HTMLInputElement>("#published-url");
const copyStatus = $("#copy-status");
const addAttachment = $<HTMLButtonElement>("#add-attachment"), attachmentList = $("#attachment-list"), attachmentHelp = $("#attachment-help");
const importedWarning = $("#imported-warning");
let sites: Site[] = [], articles: Article[] = [], site: Site | null = null, article: Article | null = null;
let subscriptions: Subscription[] = [], subscription: Subscription | null = null, item: Item | null = null;
let editing = false, dirty = false, saving = false, previewRevision = 0, previewTimer: ReturnType<typeof setTimeout> | undefined;
let navigationRevision = 0;
let busyMessage = "";
type PublicationState = { running: boolean; pending: boolean; status: "idle" | "published" | "failed"; error: string; changes: number; publishedChanges: number };
const publicationStates = new Map<string, PublicationState>();
function publicationState(siteID: string): PublicationState {
  let state = publicationStates.get(siteID);
  if (!state) { state = { running: false, pending: false, status: "idle", error: "", changes: 0, publishedChanges: 0 }; publicationStates.set(siteID, state); }
  return state;
}
function markLocalChange(siteID: string) { publicationState(siteID).changes++; }

function showError(error: unknown) {
  errorEl.textContent = error instanceof Error ? error.message : String(error);
  errorEl.hidden = false;
}
function clearError() { errorEl.hidden = true; errorEl.textContent = ""; }
function canLeave() { return !saving && (!dirty || window.confirm("当前文章有未保存的更改，确定放弃吗？")); }
function label(tag: string, value: string, className: string) {
  const el = document.createElement(tag); el.className = className; el.textContent = value; return el;
}
function renderSites() {
  sitesEl.replaceChildren();
  for (const item of sites) {
    const button = document.createElement("button"); button.className = `site-item${item.id === site?.id ? " active" : ""}`;
    button.append(label("span", "◉", "site-icon"), label("span", item.name, "site-name"));
    button.addEventListener("click", () => void selectSite(item)); sitesEl.append(button);
  }
}
function renderSubscriptions() {
  subscriptionsEl.replaceChildren();
  for (const entry of subscriptions) {
    const button = document.createElement("button"); button.className = `site-item${entry.url === subscription?.url ? " active" : ""}`;
    button.append(label("span", "◌", "site-icon"), label("span", entry.feed.title || entry.url, "site-name"));
    button.addEventListener("click", () => selectSubscription(entry)); subscriptionsEl.append(button);
  }
}
function renderArticles() {
  if (subscription) {
    articlesEl.replaceChildren(); count.textContent = String(subscription.feed.items.length);
    if (!subscription.feed.items.length) articlesEl.append(label("p", "订阅暂时没有文章", "list-empty"));
    for (const entry of subscription.feed.items) {
      const button = document.createElement("button"); button.className = `article-item${entry === item ? " active" : ""}`;
      button.append(label("span", entry.title || "无标题", "article-title"), label("span", entry.published || "", "article-date"));
      button.addEventListener("click", () => selectFeedItem(entry)); articlesEl.append(button);
    }
    return;
  }
  articlesEl.replaceChildren(); count.textContent = String(articles.length);
  if (!articles.length) { articlesEl.append(label("p", site ? "还没有文章" : "选择一个站点开始", "list-empty")); return; }
  for (const item of [...articles].sort((a, b) => b.modified.localeCompare(a.modified))) {
    const button = document.createElement("button"); button.className = `article-item${item.id === article?.id ? " active" : ""}`;
    button.append(label("span", item.title, "article-title"), label("span", new Date(item.modified).toLocaleDateString("zh-CN"), "article-date"));
    button.addEventListener("click", () => selectArticle(item)); articlesEl.append(button);
  }
}
function updateView() {
  const publication = site ? publicationStates.get(site.id) : undefined;
  renderSites(); renderSubscriptions(); renderArticles(); siteTitle.textContent = subscription?.feed.title || site?.name || "我的站点";
  newArticle.disabled = !site || !!subscription || saving; saveButton.disabled = !site || !editing || !!subscription || saving;
  saveButton.hidden = !!subscription; refreshFeed.hidden = !subscription;
  exportSite.hidden = !!subscription; exportSite.disabled = !site || saving;
  publishSite.hidden = !!subscription; publishSite.disabled = !site || saving || !!publication?.running;
  exportPlanet.hidden = !!subscription; exportPlanet.disabled = !site || saving; importPlanet.disabled = saving;
  selectTemplate.hidden = !!subscription; selectTemplate.disabled = !site || saving;
  deleteArticle.hidden = !editing || !article || !!subscription;
  addAttachment.disabled = !article || !site || !!site.imported || saving;
  attachmentList.replaceChildren();
  for (const name of article?.attachments ?? []) {
    const item = label("span", name, "attachment-item");
    if (!site?.imported) {
      const remove = document.createElement("button"); remove.className = "remove-attachment"; remove.textContent = "×"; remove.title = `删除附件 ${name}`; remove.setAttribute("aria-label", remove.title); remove.disabled = saving;
      remove.addEventListener("click", () => void removeAttachment(name)); item.append(remove);
    }
    attachmentList.append(item);
  }
  attachmentHelp.textContent = site?.imported ? "导入的原版站点暂不支持添加附件" : article ? "附件立即保存并随站点发布；插入的链接需保存文章" : "先保存文章，再添加附件";
  importedWarning.hidden = !site?.imported;
  importedWarning.textContent = site?.templateId ? "这是导入的原版站点。通过检查的简单 Plain 站点可重新生成网页并发布，保留原 IPNS 和已登记附件。" : "这是导入的原版站点。请先选择已验证的 Plain 模板，才能尝试重建或发布简单 Plain 站点。";
  empty.hidden = editing || !!item; editor.hidden = !editing; reader.hidden = !item;
  workspaceTitle.textContent = subscription ? "阅读订阅" : editing ? "撰写文章" : site?.name ?? "欢迎来到 Planet";
  siteAddress.hidden = !site?.ipns || !!subscription; siteAddress.textContent = site?.ipns ? `IPNS: ${site.ipns}` : ""; siteAddress.title = site?.ipns ?? "";
  publishedAddress.hidden = !site?.ipns || !site?.lastPublishedCID || !!subscription;
  const address = !publishedAddress.hidden ? `https://ipfs.io/ipns/${site!.ipns}/` : "";
  if (publishedURL.value !== address) { publishedURL.value = address; copyStatus.textContent = ""; }
  emptyAction.textContent = site ? "撰写文章" : "创建站点";
  emptyAction.hidden = !!subscription;
  empty.querySelector("h2")!.textContent = subscription ? "选择一篇文章阅读。" : site ? "开始记录你的故事。" : "让想法有一个地方安放。";
  empty.querySelector("p")!.textContent = subscription ? "订阅内容已保存在本地。" : site ? "新建文章，在这里写作与预览。" : "创建站点，开始撰写你的第一篇文章。";
  const current = dirty ? "有未保存更改" : editing ? "已保存" : "";
  const unpublished = !!site && ((site.contentRevision ?? 0) > (site.publishedRevision ?? 0) || !!site.publishPending || !!publication && publication.changes > publication.publishedChanges);
  saveState.textContent = saving ? busyMessage || "正在处理…" : publication?.running ? `${current}${current ? " · " : ""}正在发布已保存内容…` : publication?.status === "failed" ? `${current}${current ? " · " : ""}发布未确认` : unpublished ? `${current}${current ? " · " : ""}站点更改尚未发布` : publication?.status === "published" ? `${current}${current ? " · " : ""}${dirty ? "已发布上次保存的内容" : "已发布"}` : current;
  saveState.title = publication?.error ?? "";
}
async function selectSite(item: Site) {
  if (item.id === site?.id) return;
  if (!canLeave()) return;
  const revision = ++navigationRevision;
  try {
    const loaded = await Library.articles(item.id);
    if (revision !== navigationRevision) return;
    site = sites.find(entry => entry.id === item.id) ?? item; subscription = null; windowItemReset(); articles = loaded; article = null; editing = false; dirty = false; clearError(); updateView();
    const failure = publicationStates.get(item.id)?.error; if (failure) showError(failure);
  } catch (e) { showError(e); }
}
function windowItemReset() { item = null; }
function selectSubscription(entry: Subscription) {
  if (!canLeave()) return;
  navigationRevision++;
  site = null; subscription = entry; item = null; article = null; editing = false; dirty = false; clearError(); updateView();
}
function selectFeedItem(entry: Item) {
  item = entry; readerTitle.textContent = entry.title || "无标题"; readerDate.textContent = entry.published;
  readerPreview.srcdoc = `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><style>body{font:16px/1.8 'Segoe UI',sans-serif;color:#303239;margin:28px;overflow-wrap:anywhere}img{max-width:100%}pre{overflow:auto}</style></head><body>${entry.content || entry.description}</body></html>`;
  updateView();
}
function selectArticle(item: Article) {
  if (item.id === article?.id && editing) return;
  if (!canLeave()) return;
  navigationRevision++;
  article = item; editing = true; dirty = false; title.value = item.title; content.value = item.content; clearError(); updateView(); void renderPreview();
}
function startArticle() {
  if (!site || !canLeave()) return;
  navigationRevision++;
  article = null; editing = true; dirty = false; title.value = ""; content.value = ""; clearError(); updateView(); void renderPreview(); title.focus();
}
async function renderPreview() {
  const revision = ++previewRevision;
  try {
    const html = await Library.preview(content.value);
    if (revision !== previewRevision) return;
    preview.srcdoc = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><style>body{font:16px/1.8 'Segoe UI',sans-serif;color:#303239;margin:28px;overflow-wrap:anywhere}h1,h2,h3{line-height:1.3;color:#20252d}a{color:#4e5cbb}pre{background:#f1f2f5;padding:16px;overflow:auto;border-radius:8px}code{font-family:Consolas,monospace}blockquote{border-left:3px solid #a9b1ca;margin-left:0;padding-left:16px;color:#69707c}img{max-width:100%}</style></head><body>${html}</body></html>`;
  } catch (e) { showError(e); }
}
async function save() {
  if (!site || !editing || saveButton.disabled) return;
  navigationRevision++; saving = true; busyMessage = "正在保存…"; updateView(); clearError();
  const siteID = site.id, articleID = article?.id ?? "", nextTitle = title.value, nextContent = content.value;
  try {
    const saved = await Library.saveArticle(siteID, articleID, nextTitle, nextContent);
    markLocalChange(siteID);
    void queuePublication(siteID);
    if (site?.id !== siteID) return;
    article = saved;
    articles = [...articles.filter(item => item.id !== saved.id), saved];
    dirty = title.value !== nextTitle || content.value !== nextContent;
  } catch (e) { showError(e); }
  finally { saving = false; busyMessage = ""; updateView(); }
}

async function queuePublication(siteID: string, manual = false) {
  const state = publicationState(siteID);
  if (state.running) { state.pending = true; updateView(); return; }
  state.running = true; state.error = ""; updateView();
  try {
    do {
      state.pending = false;
      const changes = state.changes;
      try {
        const result = await Library.publishSite(siteID);
        if (result.pending) state.pending = true;
        sites = sites.map(entry => entry.id === siteID ? { ...entry, ipns: result.ipns, lastPublishedCID: result.cid, contentRevision: Math.max(entry.contentRevision ?? 0, result.revision), publishedRevision: result.revision, publishPending: result.pending } : entry);
        if (site?.id === siteID) site = sites.find(entry => entry.id === siteID) ?? site;
        if (!state.pending && state.changes === changes) state.publishedChanges = changes;
        state.status = "published"; state.error = "";
        if (manual && !state.pending) window.alert(`发布请求已完成。\n\nIPNS：${result.ipns}\nCID：${result.cid}\n\n网络传播可能需要时间；请保持应用运行以提供内容。`);
      } catch (e) {
        state.status = "failed";
        state.error = `文章已保存，但站点发布未确认：${e instanceof Error ? e.message : String(e)}。可点击“发布到 IPFS”重试。`;
        if (site?.id === siteID && !state.pending) showError(state.error);
      }
    } while (state.pending);
  } finally { state.running = false; updateView(); }
}
$("#new-site").addEventListener("click", () => { if (canLeave()) { navigationRevision++; form.reset(); dialog.showModal(); } });
$("#cancel-site").addEventListener("click", () => dialog.close());
dialog.addEventListener("close", () => { navigationRevision++; });
form.addEventListener("submit", async (event) => {
  event.preventDefault(); const button = form.querySelector<HTMLButtonElement>('button[type="submit"]')!; button.disabled = true;
  const revision = ++navigationRevision;
  try {
    const values = new FormData(form); const created = await Library.createSite(String(values.get("name") ?? ""), String(values.get("about") ?? ""));
    sites = await Library.sites(); if (revision !== navigationRevision) { updateView(); return; }
    site = created; subscription = null; item = null; articles = []; article = null; editing = false; dirty = false; dialog.close(); clearError(); updateView();
  } catch (e) { showError(e); dialog.close(); } finally { button.disabled = false; }
});
$("#new-feed").addEventListener("click", () => { if (canLeave()) { navigationRevision++; feedForm.reset(); feedDialog.showModal(); } });
$("#cancel-feed").addEventListener("click", () => feedDialog.close());
feedDialog.addEventListener("close", () => { navigationRevision++; });
refreshFeed.addEventListener("click", async () => {
  if (!subscription) return;
  const url = subscription.url; refreshFeed.disabled = true; clearError();
  try {
    const updated = await Library.follow(url); subscriptions = await Library.subscriptions();
    if (subscription?.url === url) { subscription = updated; item = null; }
    updateView();
  } catch (e) { showError(e); } finally { refreshFeed.disabled = false; }
});
exportSite.addEventListener("click", async () => {
  if (!site || dirty && !window.confirm("当前文章尚未保存，导出的网页只包含已保存的内容。继续吗？")) return;
  exportSite.disabled = true; clearError();
  try { const path = await Library.exportWebsite(site.id); if (path) window.alert(`网站已导出至：\n${path}`); }
  catch (e) { showError(e); } finally { exportSite.disabled = !site; }
});
publishSite.addEventListener("click", async () => {
  if (!site || saving || publicationStates.get(site.id)?.running) return;
  const files = articles.flatMap(a => (a.attachments ?? []).map(name => `${a.title}：${name}`));
  const fileNotice = files.length ? `\n\n将公开已保存的附件：\n${files.slice(0, 25).join("\n")}${files.length > 25 ? `\n以及另外 ${files.length - 25} 个附件` : ""}` : site.imported ? "\n\n原版备份中登记的附件也会随站点公开。" : "";
  if (!window.confirm(`${dirty ? "当前文章尚未保存，将发布已保存的内容。\n\n" : ""}发布后，站点内容将公开到 IPFS 网络。${fileNotice}\n\n继续吗？`)) return;
  clearError(); await queuePublication(site.id, true);
});
importPlanet.addEventListener("click", async () => {
  if (!canLeave()) return;
  navigationRevision++; saving = true; title.disabled = true; content.disabled = true; updateView(); clearError();
  try {
    const imported = await Library.importPlanet();
    if (imported.id) { sites = await Library.sites(); articles = await Library.articles(imported.id); site = imported; subscription = null; item = null; article = null; editing = false; dirty = false; navigationRevision++; }
  } catch (e) { showError(e); }
  finally { saving = false; title.disabled = false; content.disabled = false; updateView(); }
});
exportPlanet.addEventListener("click", async () => {
  if (!site || saving) return;
  if (!window.confirm(`${dirty ? "当前文章尚未保存，备份只包含已保存内容。\n\n" : ""}备份包含站点私钥，请妥善保管。继续吗？`)) return;
  saving = true; updateView(); clearError();
  try { const path = await Library.exportPlanet(site.id); if (path) window.alert(`备份已保存到：\n${path}`); }
  catch (e) { showError(e); } finally { saving = false; updateView(); }
});
selectTemplate.addEventListener("click", async () => {
  if (!site || saving) return;
  const siteID=site.id; navigationRevision++; saving=true; updateView(); clearError();
  try {
    const name=await Library.selectTemplate(siteID);
    if (name) { markLocalChange(siteID); sites=await Library.sites(); site=sites.find(entry=>entry.id===siteID) ?? site; window.alert(site?.imported ? `已选择模板：${name}\n可尝试导出备份；不支持的原站点设置会提示原因。` : `已选择模板：${name}\n建议先导出网页检查效果。`); if (site?.publishPending) void queuePublication(siteID); }
  } catch(e) { showError(e); } finally { saving=false; updateView(); }
});
deleteArticle.addEventListener("click", async () => {
  if (!site || !article || saving || !window.confirm("确定删除这篇文章？此操作无法撤销。")) return;
  const siteID = site.id; navigationRevision++; saving = true; deleteArticle.disabled = true; title.disabled = true; content.disabled = true; clearError();
  try { await Library.deleteArticle(siteID, article.id); markLocalChange(siteID); articles = await Library.articles(siteID); article = null; editing = false; dirty = false; updateView(); }
  catch (e) { showError(e); } finally { saving = false; deleteArticle.disabled = false; title.disabled = false; content.disabled = false; updateView(); }
});
feedForm.addEventListener("submit", async (event) => {
  event.preventDefault(); const button = feedForm.querySelector<HTMLButtonElement>('button[type="submit"]')!; button.disabled = true;
  const revision = ++navigationRevision;
  try {
    const address = String(new FormData(feedForm).get("url") ?? "");
    const entry = await Library.follow(address); subscriptions = await Library.subscriptions();
    if (revision !== navigationRevision) { updateView(); return; }
    site = null; subscription = entry; item = null; article = null; editing = false; dirty = false; feedDialog.close(); clearError(); updateView();
  } catch (e) { feedDialog.close(); showError(e); } finally { button.disabled = false; }
});
newArticle.addEventListener("click", startArticle);
async function removeAttachment(name: string) {
  if (!site || !article || saving || site.imported) return;
  if (!window.confirm(`删除附件“${name}”？文章中的链接不会自动删除。`)) return;
  const siteID = site.id, articleID = article.id;
  navigationRevision++; saving = true; busyMessage = "正在处理附件，可能需等待发布完成…"; updateView(); clearError();
  try {
    const updated = await Library.removeAttachment(siteID, articleID, name);
    markLocalChange(siteID);
    if (site?.id === siteID && article?.id === articleID) { article = updated; articles = await Library.articles(siteID); }
  } catch (e) { showError(e); }
  finally { saving = false; busyMessage = ""; updateView(); }
}
addAttachment.addEventListener("click", async () => {
  if (!site || !article || saving || site.imported) return;
  const siteID = site.id, articleID = article.id, previous = new Set(article.attachments ?? []);
  navigationRevision++; saving = true; busyMessage = "正在处理附件，可能需等待发布完成…"; updateView(); clearError();
  try {
    const updated = await Library.addAttachment(siteID, articleID);
    if (!updated.id || site?.id !== siteID || article?.id !== articleID) return;
    markLocalChange(siteID);
    const name = updated.attachments?.find(item => !previous.has(item));
    article = updated; articles = await Library.articles(siteID);
    if (name) {
      const label = name.replace(/[\\\[\]]/g, "\\$&");
      const url = encodeURIComponent(name).replace(/[()]/g, ch => `%${ch.charCodeAt(0).toString(16).toUpperCase()}`);
      const image = /\.(png|jpe?g|gif|webp|svg)$/i.test(name);
      content.value += `${content.value ? "\n\n" : ""}${image ? "!" : ""}[${label}](${url})`;
      dirty = true; navigationRevision++; void renderPreview();
    }
  } catch (e) { showError(e); }
  finally { saving = false; busyMessage = ""; updateView(); }
});
$("#copy-published-url").addEventListener("click", async () => {
  if (!publishedURL.value) return;
  try { await window.navigator.clipboard.writeText(publishedURL.value); copyStatus.textContent = "已复制"; }
  catch { publishedURL.focus(); publishedURL.select(); copyStatus.textContent = "请按 Ctrl+C 复制"; }
});
emptyAction.addEventListener("click", () => site ? startArticle() : $("#new-site").click());
saveButton.addEventListener("click", () => void save());
for (const input of [title, content]) input.addEventListener("input", () => { navigationRevision++; dirty = true; updateView(); clearTimeout(previewTimer); previewTimer = setTimeout(() => void renderPreview(), 180); });
window.addEventListener("keydown", (event) => { if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") { event.preventDefault(); void save(); } });
window.addEventListener("beforeunload", (event) => { if (dirty) event.preventDefault(); });
async function init() {
  const [loadedSites, loadedSubscriptions] = await Promise.allSettled([Library.sites(), Library.subscriptions()]);
  if (loadedSites.status === "fulfilled") sites = loadedSites.value; else showError(loadedSites.reason);
  if (loadedSubscriptions.status === "fulfilled") subscriptions = loadedSubscriptions.value; else showError(loadedSubscriptions.reason);
  if (sites[0]) {
    site = sites[0];
    try { articles = await Library.articles(site.id); } catch (e) { showError(e); }
  }
  updateView();
  for (const entry of sites) if (entry.publishPending) void queuePublication(entry.id);
}
void init();
