import { Library, type Article, type Draft, type Site, type Subscription, type Item } from "./mygo";
import { initUpdates } from "./updates";
import release from "../release.json";
import { refreshDue } from "./auto-refresh";

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
const refreshFeed = $<HTMLButtonElement>("#refresh-feed"), pinSubscription = $<HTMLButtonElement>("#pin-subscription");
const unfollow = $<HTMLButtonElement>("#unfollow");
const starItem = $<HTMLButtonElement>("#star-item"), shareItem = $<HTMLButtonElement>("#share-item"), shareURL = $<HTMLInputElement>("#share-url"), shareStatus = $("#share-status");
const exportSite = $<HTMLButtonElement>("#export-site");
const publishSite = $<HTMLButtonElement>("#publish-site");
const exportPlanet = $<HTMLButtonElement>("#export-planet"), importPlanet = $<HTMLButtonElement>("#import-planet");
const selectTemplate = $<HTMLButtonElement>("#select-template");
const templateSettingsButton = $<HTMLButtonElement>("#template-settings"), templateSettingsDialog = $<HTMLDialogElement>("#template-settings-dialog"), templateSettingsForm = $<HTMLFormElement>("#template-settings-form");
const pinnableButton=$<HTMLButtonElement>("#pinnable-settings"), pinnableDialog=$<HTMLDialogElement>("#pinnable-dialog"), pinnableForm=$<HTMLFormElement>("#pinnable-form"), pinnableResult=$("#pinnable-result");
const serviceButton=$<HTMLButtonElement>("#service-settings"),serviceDialog=$<HTMLDialogElement>("#service-dialog"),serviceForm=$<HTMLFormElement>("#service-form"),serviceResult=$("#service-result");
const editSite = $<HTMLButtonElement>("#edit-site");
const removeSite = $<HTMLButtonElement>("#remove-site");
const removedSitesDialog = $<HTMLDialogElement>("#removed-sites-dialog"), removedSiteList = $("#removed-site-list");
const deleteArticle = $<HTMLButtonElement>("#delete-article");
const showDrafts = $<HTMLButtonElement>("#show-drafts"), saveDraft = $<HTMLButtonElement>("#save-draft");
const siteAddress = $("#site-address");
const publishedAddress = $("#published-address"), publishedURL = $<HTMLInputElement>("#published-url");
const copyStatus = $("#copy-status");
const addAttachment = $<HTMLButtonElement>("#add-attachment"), attachmentList = $("#attachment-list"), attachmentHelp = $("#attachment-help");
const tagInput=$<HTMLInputElement>("#tag-input"), addTag=$<HTMLButtonElement>("#add-tag"), tagList=$("#tag-list");
const importedWarning = $("#imported-warning");
const playArticleAudio = $<HTMLButtonElement>("#play-article-audio"), audioPlayer = $("#audio-player"), audioTitle = $("#audio-title"), globalAudio = $<HTMLAudioElement>("#global-audio"), globalVideo = $<HTMLVideoElement>("#global-video");
let activeMedia: HTMLMediaElement = globalAudio;
let audioRevision = 0, playingSiteID = "", playingArticleID = "";
let audioRequest: Promise<void> = Promise.resolve();
let sites: Site[] = [], articles: Article[] = [], site: Site | null = null, article: Article | null = null;
let sitesRevision = 0;
const articleLoadFailures = new Set<string>();
let drafts: Draft[] = [], draft: Draft | null = null, draftsView = false;
let subscriptions: Subscription[] = [], subscription: Subscription | null = null, item: Item | null = null;
let subscriptionsRevision = 0;
let editing = false, dirty = false, saving = false, previewRevision = 0, previewTimer: ReturnType<typeof setTimeout> | undefined;
let editorTags=new Map<string,string>(), savedTagSignature="[]";

function tagSignature(): string { return JSON.stringify([...editorTags.entries()].sort(([a],[b])=>a.localeCompare(b))); }
function tagJSON(): string { return JSON.stringify(Object.fromEntries(editorTags)); }
function setEditorTags(tags?: Record<string,string>) { editorTags=new Map(Object.entries(tags ?? {}));savedTagSignature=tagSignature();tagInput.value=""; }
function normalizedTag(value:string):string {
  return value.toLowerCase().normalize("NFD").replace(/\p{M}/gu,"").replace(/[^\p{L}\p{N}-]+/gu,"-").replace(/-+/g,"-").replace(/^-|-$/g,"");
}
function addEditorTag() {
  if (!editing || saving || site?.tagsUnsupported) return;
  const value=tagInput.value.trim(), key=normalizedTag(value);
  if (!key || /^(?:index|tags|archive|archives|page[0-9]+|con|prn|aux|nul|(?:com|lpt)[1-9¹²³])$/i.test(key)) {showError("标签名称不能生成安全路径，请换一个名称。");return;}
  if ([...editorTags.keys()].some(old=>old.toLowerCase()===key.toLowerCase())) {showError("标签已存在。");return;}
  editorTags.set(key,value);tagInput.value="";dirty=true;navigationRevision++;clearError();updateView();
}
addTag.addEventListener("click",addEditorTag);
tagInput.addEventListener("keydown",event=>{if(event.key==="Enter"){event.preventDefault();addEditorTag();}});
tagInput.addEventListener("input",()=>{dirty=true;navigationRevision++;updateView();});
function pendingTagBlocksSave(): boolean {
  if (!tagInput.value.trim()) return false;
  showError("请先添加标签，或清空标签输入框后再保存。");tagInput.focus();return true;
}
let navigationRevision = 0, favoritesView = false, readerRevision = 0;
const refreshingSubscriptions = new Set<string>();
let autoRefreshing = false;
let busyMessage = "";
let editingSiteID = "";
let editingTemplateSiteID = "";
let pinnableSiteID="", pinnableConfigured=false;
const sepiaFields: Array<[string,string,string]> = [
  ["backgroundColor","背景色","#F4F1EC"], ["foregroundColor","文字颜色","#000000"], ["foregroundSecondaryColor","次要文字","#999999"],
  ["highlightColor","强调色","#FF4500"], ["linkColor","链接颜色","#778087"], ["linkHoverColor","链接悬停","#4D5256"],
  ["siteTitleColor","站点标题","#333333"], ["siteTitleHoverColor","标题悬停","#000000"], ["borderColor","边框颜色","#E7E5DF"],
  ["navigationLinkColor","导航文字","#333333"], ["navigationActiveItemColor","导航选中","#FCFCFA"],
];
type PublicationState = { running: boolean; pending: boolean; status: "idle" | "published" | "failed"; error: string; changes: number; publishedChanges: number; generation: number };
const publicationStates = new Map<string, PublicationState>();
function publicationState(siteID: string): PublicationState {
  let state = publicationStates.get(siteID);
  if (!state) { state = { running: false, pending: false, status: "idle", error: "", changes: 0, publishedChanges: 0, generation: 0 }; publicationStates.set(siteID, state); }
  return state;
}
function markLocalChange(siteID: string) { publicationState(siteID).changes++; }
function mergeSitePublication(current: Site | undefined, updated: Site, generation: number): Site {
  const newerObserved=publicationState(updated.id).generation>generation && !!current?.lastPublishedCID;
  if (!current || (!newerObserved && (current.publishedRevision ?? 0) <= (updated.publishedRevision ?? 0))) return updated;
  const revision=Math.max(current.contentRevision ?? 0,updated.contentRevision ?? 0);
  return {...updated,ipns:current.ipns,lastPublishedCID:current.lastPublishedCID,lastPublished:current.lastPublished,publishedRevision:current.publishedRevision,contentRevision:revision,publishPending:!!current.publishPending || revision>(current.publishedRevision ?? 0)};
}
async function reloadSites(): Promise<boolean> {
  const revision=++sitesRevision, loaded=await Library.sites();
  if (revision!==sitesRevision) return false;
  sites=loaded; return true;
}
async function reloadSubscriptions(): Promise<boolean> {
  const revision=++subscriptionsRevision, loaded=await Library.subscriptions();
  if (revision!==subscriptionsRevision) return false;
  subscriptions=loaded;
  if (subscription) {
    const updated=loaded.find(entry=>entry.url===subscription?.url);
    if (!updated) { subscription=null; item=null; readerRevision++; }
    else {
      subscription=updated;
      if (item) { const key=item.key || item.id || item.link; const current=updated.feed.items.find(entry=>(entry.key || entry.id || entry.link)===key); if(current) selectFeedItem(current); }
    }
  }
  return true;
}

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
    button.append(label("span", entry.lastError ? "⚠" : "◌", "site-icon"), label("span", entry.feed.title || entry.url, "site-name"));
    button.title = entry.lastError ? `更新失败：${entry.lastError}` : entry.pinnedCid ? "已固定到本机" : entry.url;
    button.addEventListener("click", () => selectSubscription(entry)); subscriptionsEl.append(button);
  }
}
function renderArticles() {
  if (favoritesView) {
    articlesEl.replaceChildren(); const favorites = subscriptions.flatMap(source => source.feed.items.filter(entry => entry.starred).map(entry => ({source,entry})));
    count.textContent = String(favorites.length);
    if (!favorites.length) articlesEl.append(label("p", "还没有收藏的文章", "list-empty"));
    for (const favorite of favorites) { const button=document.createElement("button"); button.className="article-item"; button.append(label("span",favorite.entry.title || "无标题","article-title"),label("span",favorite.source.feed.title || favorite.source.url,"article-date")); button.addEventListener("click",()=>{subscription=favorite.source; site=null; selectFeedItem(favorite.entry);}); articlesEl.append(button); }
    return;
  }
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
  if (draftsView) {
    articlesEl.replaceChildren(); count.textContent=String(drafts.length);
    if (!drafts.length) articlesEl.append(label("p","还没有草稿","list-empty"));
    for (const entry of drafts) {
      const button=document.createElement("button"); button.className=`article-item${entry.id===draft?.id ? " active" : ""}`;
      button.append(label("span",entry.title || "未命名草稿","article-title"),label("span",new Date(entry.modified).toLocaleDateString("zh-CN"),"article-date"));
      button.addEventListener("click",()=>selectDraft(entry)); articlesEl.append(button);
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
  showDrafts.hidden=!!subscription; showDrafts.disabled=!site || saving; showDrafts.textContent=draftsView ? "返回文章" : "草稿";
  saveDraft.hidden=!editing || !!subscription; saveDraft.disabled=!site || !editing || saving;
  saveButton.hidden = !!subscription; refreshFeed.hidden = !subscription;
  unfollow.hidden = !subscription; unfollow.disabled = !subscription || saving;
  refreshFeed.disabled = !!subscription && refreshingSubscriptions.has(subscription.url); refreshFeed.textContent = refreshFeed.disabled ? "正在刷新…" : "刷新订阅";
  exportSite.hidden = !!subscription; exportSite.disabled = !site || saving;
  publishSite.hidden = !!subscription; publishSite.disabled = !site || saving || !!publication?.running;
  exportPlanet.hidden = !!subscription; exportPlanet.disabled = !site || saving; importPlanet.disabled = saving;
  selectTemplate.hidden = !!subscription; selectTemplate.disabled = !site || saving;
  templateSettingsButton.hidden = !!subscription; templateSettingsButton.disabled = !site || site.templateName !== "Sepia" || saving;
  pinnableButton.hidden=!!subscription; pinnableButton.disabled=!site || saving;
  serviceButton.hidden=!!subscription; serviceButton.disabled=!site || saving;
  editSite.hidden = !!subscription; editSite.disabled = !site || saving;
  removeSite.hidden = !!subscription; removeSite.disabled = !site || saving || !!publication?.running;
  const fixed = !!subscription && (subscription.cid ? subscription.pinnedCid === subscription.cid : !!subscription.offlinePinned);
  pinSubscription.hidden = !subscription; pinSubscription.disabled = saving || !!subscription && ((fixed && !!subscription.cid) || refreshingSubscriptions.has(subscription.url));
  pinSubscription.textContent = fixed ? (subscription?.cid ? "已固定离线" : "重新固定离线") : "固定离线";
  $("#subscription-status").textContent = subscription && refreshingSubscriptions.has(subscription.url) ? "正在刷新订阅…" : subscription?.lastError ? `更新失败，正在显示已缓存内容：${subscription.lastError}` : subscription?.offlinePinned ? "已固定订阅媒体到本机" : subscription?.pinnedCid ? (subscription.pinnedCid === subscription.cid ? "已固定当前版本到本机" : "仅旧版本已固定") : "";
  starItem.textContent = item?.starred ? "取消收藏" : "收藏"; starItem.disabled = !item || !subscription; shareItem.disabled = !item?.link;
  deleteArticle.hidden = !editing || (!article && !draft) || !!subscription; deleteArticle.textContent=draft ? "删除草稿" : "删除文章";
  addAttachment.disabled = (!article && !draft) || !site || saving;
	 tagInput.disabled = !editing || saving || !!site?.tagsUnsupported; addTag.disabled = tagInput.disabled;
	 tagList.replaceChildren();
	 for (const [key,value] of editorTags) {
	   const item=label("span",value,"tag-item"), remove=document.createElement("button");
	   remove.type="button";remove.className="remove-tag";remove.textContent="×";remove.setAttribute("aria-label",`删除标签 ${value}`);remove.disabled=tagInput.disabled;
	   remove.addEventListener("click",()=>{if(saving || site?.tagsUnsupported)return;editorTags.delete(key);dirty=true;navigationRevision++;updateView();});item.append(remove);tagList.append(item);
	 }
  playArticleAudio.hidden = !article?.audioFilename || !site; playArticleAudio.disabled = saving;
  attachmentList.replaceChildren();
  for (const name of [...(article?.attachments ?? []), ...(draft?.attachments ?? [])]) {
    const marked = !!draft?.removedAttachments?.includes(name);
    const item = label("span", marked ? `${name}（发布时删除）` : name, "attachment-item");
    const remove = document.createElement("button"); remove.className = "remove-attachment"; remove.textContent = marked ? "撤销" : "×"; remove.title = marked ? `撤销删除附件 ${name}` : `删除附件 ${name}`; remove.setAttribute("aria-label", remove.title); remove.disabled = saving;
    remove.addEventListener("click", () => void removeAttachment(name)); item.append(remove);
    attachmentList.append(item);
  }
  attachmentHelp.textContent = draft ? "新增附件仅保存在本机；删除原文章附件会在发布草稿时生效，可随时撤销" : article ? "附件立即保存；保存文章或手动发布后生效，插入的链接需保存文章" : "先保存草稿或文章，再添加附件";
  importedWarning.hidden = !site?.imported;
  importedWarning.textContent = site?.templateId ? "这是导入的原版站点。通过检查的导入站点可重新生成网页并发布，保留原 IPNS 和已登记附件。" : "这是导入的原版站点。请先选择已验证的模板，才能尝试重建或发布兼容的站点。";
	 if(site?.tagsUnsupported) importedWarning.textContent += " 原备份标签格式不受支持，已保留原始数据；当前不能编辑标签或重建该站点。";
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
  const current = dirty ? "有未保存更改" : draft ? "草稿已保存（未发布）" : editing ? "已保存" : "";
  const unpublished = !!site && ((site.contentRevision ?? 0) > (site.publishedRevision ?? 0) || !!site.publishPending || !!publication && publication.changes > publication.publishedChanges);
  saveState.textContent = saving ? busyMessage || "正在处理…" : publication?.running ? `${current}${current ? " · " : ""}正在发布已保存内容…` : publication?.status === "failed" ? `${current}${current ? " · " : ""}发布未确认` : unpublished ? `${current}${current ? " · " : ""}站点更改尚未发布` : publication?.status === "published" ? `${current}${current ? " · " : ""}${dirty ? "已发布上次保存的内容" : "已发布"}` : current;
  saveState.title = publication?.error ?? "";
}
async function selectSite(item: Site) {
  if (item.id === site?.id && !articleLoadFailures.has(item.id)) return;
  if (!canLeave()) return;
  const revision = ++navigationRevision;
  try {
    const loaded = await Library.articles(item.id);
    if (revision !== navigationRevision) return;
    articleLoadFailures.delete(item.id); site = sites.find(entry => entry.id === item.id) ?? item; favoritesView = false; subscription = null; windowItemReset(); articles = loaded; article = null; draft=null; drafts=[]; draftsView=false; editing = false; dirty = false; clearError(); updateView();
    const failure = publicationStates.get(item.id)?.error; if (failure) showError(failure);
  } catch (e) { if (revision===navigationRevision) { articleLoadFailures.add(item.id); showError(e); } }
}
function windowItemReset() { item = null; }
function selectSubscription(entry: Subscription) {
  if (!canLeave()) return;
  navigationRevision++;
  site = null; favoritesView = false; subscription = entry; item = null; article = null; draft=null; draftsView=false; editing = false; dirty = false; clearError(); updateView();
}
function readerHTML(html: string, localImages: boolean) {
  const images = localImages ? "img-src data:; " : "";
  readerPreview.srcdoc = `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; ${images}style-src 'unsafe-inline'"><style>body{font:16px/1.8 'Segoe UI',sans-serif;color:#303239;margin:28px;overflow-wrap:anywhere}img{max-width:100%}pre{overflow:auto}</style></head><body>${html}</body></html>`;
}
function selectFeedItem(entry: Item) {
  const revision=++readerRevision, address=subscription?.url ?? "", key=entry.key || entry.id || entry.link;
  item = entry; shareURL.hidden = true; shareStatus.textContent = ""; readerTitle.textContent = entry.title || "无标题"; readerDate.textContent = entry.published;
  $("#play-feed-audio").hidden = !entry.audioUrl && !(entry.audioFilename && entry.contentCid);
  $("#play-feed-video").hidden = !entry.videoUrl;
  readerHTML(entry.content || entry.description,false);
  if((entry.contentCid || entry.offlineBase) && address && key) {
    void Library.subscriptionPreview(address,key).then(html=>{
      if(revision===readerRevision && subscription?.url===address && (item?.key || item?.id || item?.link)===key) readerHTML(html,true);
    }).catch(()=>{});
  }
  updateView();
}
function selectArticle(item: Article) {
  if (item.id === article?.id && editing) return;
  if (!canLeave()) return;
  navigationRevision++;
  article = item; draft=null; draftsView=false; editing = true; dirty = false; title.value = item.title; content.value = item.content; setEditorTags(item.tags); clearError(); updateView(); void renderPreview();
}
function selectDraft(item: Draft) {
  if (!canLeave()) return;
  navigationRevision++; draft=item; article=articles.find(entry=>entry.id===item.articleId) ?? null;
  editing=true; dirty=false; title.value=item.title; content.value=item.content; setEditorTags(item.tags); clearError(); updateView(); void renderPreview();
}
function startArticle() {
  if (!site || !canLeave()) return;
  navigationRevision++;
  article = null; draft=null; draftsView=false; editing = true; dirty = false; title.value = ""; content.value = ""; setEditorTags(); clearError(); updateView(); void renderPreview(); title.focus();
}
async function renderPreview() {
  const revision = ++previewRevision;
  try {
    const html = site && draft ? await Library.previewDraft(site.id, draft.id, content.value) : site && article ? await Library.previewArticle(site.id, article.id, content.value) : await Library.preview(content.value);
    if (revision !== previewRevision) return;
    preview.srcdoc = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'"><style>body{font:16px/1.8 'Segoe UI',sans-serif;color:#303239;margin:28px;overflow-wrap:anywhere}h1,h2,h3{line-height:1.3;color:#20252d}a{color:#4e5cbb}pre{background:#f1f2f5;padding:16px;overflow:auto;border-radius:8px}code{font-family:Consolas,monospace}blockquote{border-left:3px solid #a9b1ca;margin-left:0;padding-left:16px;color:#69707c}img{max-width:100%}</style></head><body>${html}</body></html>`;
  } catch (e) { showError(e); }
}
async function save() {
  if (!site || !editing || saveButton.disabled) return;
  if (pendingTagBlocksSave()) return;
  navigationRevision++; saving = true; busyMessage = "正在保存…"; updateView(); clearError();
  const siteID = site.id, articleID = article?.id ?? "", nextTitle = title.value, nextContent = content.value, nextTags=tagJSON(), nextSignature=tagSignature(), tagsChanged=nextSignature!==savedTagSignature;
  try {
    const saved = draft ? (tagsChanged ? await Library.publishDraftWithTags(siteID,draft.id,nextTitle,nextContent,nextTags) : await Library.publishDraft(siteID,draft.id,nextTitle,nextContent)) : (tagsChanged ? await Library.saveArticleWithTags(siteID,articleID,nextTitle,nextContent,nextTags) : await Library.saveArticle(siteID,articleID,nextTitle,nextContent));
    markLocalChange(siteID);
    void queuePublication(siteID);
    if (site?.id !== siteID) return;
    if (draft) drafts=drafts.filter(entry=>entry.id!==draft!.id);
    draft=null; draftsView=false; article = saved;
    articles = [...articles.filter(item => item.id !== saved.id), saved];
    void renderPreview();
    savedTagSignature=nextSignature;dirty = title.value !== nextTitle || content.value !== nextContent || tagSignature()!==nextSignature;
  } catch (e) { showError(e); }
  finally { saving = false; busyMessage = ""; updateView(); }
}

showDrafts.addEventListener("click", async () => {
  if (!site || !canLeave()) return;
  if (draftsView) { navigationRevision++; draftsView=false; draft=null; article=null; editing=false; dirty=false; updateView(); return; }
  const siteID=site.id, revision=++navigationRevision;
  try { const loaded=await Library.drafts(siteID); if(revision!==navigationRevision || site?.id!==siteID)return; drafts=loaded; draftsView=true; draft=null; article=null; editing=false; dirty=false; updateView(); }
  catch(e) { showError(e); }
});
saveDraft.addEventListener("click", async () => {
  if (!site || !editing || saving) return;
  if (pendingTagBlocksSave()) return;
  const siteID=site.id, draftID=draft?.id ?? "", articleID=draft?.articleId ?? article?.id ?? "", nextTitle=title.value, nextContent=content.value, nextTags=tagJSON(), nextSignature=tagSignature(), tagsChanged=nextSignature!==savedTagSignature;
  navigationRevision++; saving=true; busyMessage="正在保存草稿…"; updateView(); clearError();
  try { const existing=await Library.drafts(siteID); const saved=tagsChanged ? await Library.saveDraftWithTags(siteID,draftID,articleID,nextTitle,nextContent,nextTags) : await Library.saveDraft(siteID,draftID,articleID,nextTitle,nextContent); drafts=[...existing.filter(entry=>entry.id!==saved.id),saved]; draft=saved; draftsView=true; savedTagSignature=nextSignature;dirty=title.value!==nextTitle || content.value!==nextContent || tagSignature()!==nextSignature; }
  catch(e) { showError(e); } finally { saving=false; busyMessage=""; updateView(); }
});

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
        state.generation++;
        if (result.pending) state.pending = true;
        if (sites.some(entry=>entry.id===siteID)) { sitesRevision++; sites = sites.map(entry => entry.id === siteID ? { ...entry, ipns: result.ipns, lastPublishedCID: result.cid, contentRevision: Math.max(entry.contentRevision ?? 0, result.revision), publishedRevision: result.revision, publishPending: result.pending } : entry); }
        if (site?.id === siteID) site = sites.find(entry => entry.id === siteID) ?? site;
        if (!state.pending && state.changes === changes) state.publishedChanges = changes;
        state.status = "published"; state.error = result.pinnableWarning ?? "";
        if (result.pinnableWarning && site?.id===siteID) showError(result.pinnableWarning);
        if (manual && !state.pending) window.alert(`发布请求已完成。\n\nIPNS：${result.ipns}\nCID：${result.cid}\n\n网络传播可能需要时间；请保持应用运行以提供内容。${result.pinnableWarning ? `\n\n${result.pinnableWarning}` : ""}`);
      } catch (e) {
        state.status = "failed";
        state.error = `文章已保存，但站点发布未确认：${e instanceof Error ? e.message : String(e)}。可点击“发布到 IPFS”重试。`;
        if (site?.id === siteID && !state.pending) showError(state.error);
      }
    } while (state.pending);
  } finally { state.running = false; updateView(); }
}
$("#new-site").addEventListener("click", () => { if (canLeave()) { navigationRevision++; editingSiteID=""; form.reset(); form.querySelector("h2")!.textContent="创建站点"; form.querySelector<HTMLButtonElement>('button[type="submit"]')!.textContent="创建站点"; dialog.showModal(); } });
editSite.addEventListener("click", () => {
  if (!site || saving) return;
  editingSiteID=site.id; form.reset();
  form.querySelector<HTMLInputElement>('input[name="name"]')!.value=site.name;
  form.querySelector<HTMLTextAreaElement>('textarea[name="about"]')!.value=site.about;
  form.querySelector("h2")!.textContent="站点设置"; form.querySelector<HTMLButtonElement>('button[type="submit"]')!.textContent="保存设置";
  dialog.showModal();
});
removeSite.addEventListener("click", async () => {
  if (!site || removeSite.disabled || !canLeave()) return;
  const siteID=site.id;
  if (!window.confirm(`移除站点“${site.name}”？文章、草稿和 IPNS 密钥会保存在本机，可在“已移除的站点”中恢复。已公开的内容仍可能可访问。`)) return;
  navigationRevision++; saving=true; title.disabled=true; content.disabled=true; busyMessage="正在移除站点…"; updateView(); clearError();
  try {
    await Library.removeSite(siteID); sitesRevision++; sites=sites.filter(entry=>entry.id!==siteID); publicationStates.delete(siteID);
    if (site?.id===siteID) { if(playingSiteID===siteID) closeAudio(); site=null; articles=[]; drafts=[]; article=null; draft=null; draftsView=false; editing=false; dirty=false; }
    try { await reloadSites(); } catch(e) { showError(`站点已移除，但站点列表刷新失败：${String(e)}`); }
  } catch(e) { showError(e); } finally { saving=false; title.disabled=false; content.disabled=false; busyMessage=""; updateView(); }
});
$("#show-removed-sites").addEventListener("click", async () => {
  if (saving) return;
  removedSiteList.textContent="正在读取…"; removedSitesDialog.showModal();
  try {
    const entries=await Library.removedSites(); removedSiteList.replaceChildren();
    if (!entries.length) removedSiteList.append(label("p","没有已移除的站点","list-empty"));
    for (const entry of entries) {
      const row=document.createElement("div"); row.className="removed-site";
      const restore=document.createElement("button"); restore.className="secondary"; restore.textContent="恢复"; restore.setAttribute("aria-label",`恢复站点 ${entry.name}`);
      restore.hidden=!!entry.deletionPending;
      restore.addEventListener("click",async()=>{
        if (!canLeave()) return;
        navigationRevision++; saving=true; title.disabled=true; content.disabled=true; busyMessage="正在恢复站点…"; restore.disabled=true; updateView(); clearError();
        try {
          const restored=await Library.restoreSite(entry.id);
          sitesRevision++; sites=[...sites.filter(item=>item.id!==restored.id),restored];
          site=restored; subscription=null; item=null; favoritesView=false; article=null; draft=null; drafts=[]; draftsView=false; articles=[]; editing=false; dirty=false;
          removedSitesDialog.close();
          try { articles=await Library.articles(restored.id); articleLoadFailures.delete(restored.id); }
          catch(e) { articleLoadFailures.add(restored.id); showError(`站点已恢复，但文章加载失败；请点击站点重试：${String(e)}`); }
        } catch(e) { showError(e); }
        finally { saving=false; title.disabled=false; content.disabled=false; busyMessage=""; updateView(); }
      });
      const purge=document.createElement("button");purge.className="secondary";purge.textContent=entry.deletionPending ? "继续清理" : "永久删除";purge.setAttribute("aria-label",`${purge.textContent}站点 ${entry.name}`);
      const status=label("span",entry.deletionPending ? "删除未完成，不能恢复" : "","removed-site-status");
      const copyID=document.createElement("button");copyID.className="secondary";copyID.textContent="复制 ID";copyID.setAttribute("aria-label",`复制站点 ${entry.name} 的 ID`);
      copyID.addEventListener("click",async()=>{try{await window.navigator.clipboard.writeText(entry.id);status.textContent="站点 ID 已复制";}catch{window.prompt("请复制完整站点 ID：",entry.id);}});
      purge.addEventListener("click",async()=>{
        if(saving)return;
        if(!entry.deletionPending) {
          const answer=window.prompt(`永久删除站点“${entry.name}”及本机文章、草稿、附件和 IPNS 私钥？此操作无法撤销。独立备份与已公开到 IPFS 的内容不会自动删除。\n\n请输入此站点完整 ID 确认：\n${entry.id}`,"");
          if(answer!==entry.id)return;
        }
        navigationRevision++;saving=true;title.disabled=true;content.disabled=true;busyMessage="正在永久删除站点…";
        const controls=Array.from(removedSitesDialog.querySelectorAll<HTMLButtonElement>("button"));for(const control of controls)control.disabled=true;updateView();clearError();
        try {
          await Library.permanentlyDeleteSite(entry.id);row.remove();publicationStates.delete(entry.id);
          if(!removedSiteList.querySelector(".removed-site"))removedSiteList.append(label("p","没有已移除的站点","list-empty"));
        } catch(e) {
          status.textContent=`删除未完成：${e instanceof Error ? e.message : String(e)}`;
          try {const current=(await Library.removedSites()).find(item=>item.id===entry.id);if(current?.deletionPending){entry.deletionPending=true;restore.hidden=true;purge.textContent="继续清理";purge.setAttribute("aria-label",`继续清理站点 ${entry.name}`);}}catch{}
        } finally {saving=false;title.disabled=false;content.disabled=false;busyMessage="";for(const control of controls)control.disabled=false;updateView();}
      });
      const actions=document.createElement("span");actions.className="removed-site-actions";actions.append(restore,copyID,purge);
      const info=document.createElement("span");info.className="removed-site-info";info.append(label("span",entry.name,"site-name"),label("span",entry.id,"removed-site-id"),status);
      row.append(info,actions);removedSiteList.append(row);
    }
  } catch(e) { removedSiteList.textContent="无法读取已移除的站点"; showError(e); }
});
$("#close-removed-sites").addEventListener("click",()=>{if(!saving)removedSitesDialog.close();});
removedSitesDialog.addEventListener("cancel",event=>{if(saving)event.preventDefault();});
$("#cancel-site").addEventListener("click", () => dialog.close());
dialog.addEventListener("cancel", event => { if (saving) event.preventDefault(); });
dialog.addEventListener("close", () => { navigationRevision++; });
form.addEventListener("submit", async (event) => {
  event.preventDefault(); const button = form.querySelector<HTMLButtonElement>('button[type="submit"]')!; button.disabled = true;
  const revision = ++navigationRevision;
  const isUpdate=!!editingSiteID, cancel=$<HTMLButtonElement>("#cancel-site");
  if(isUpdate) { saving=true; cancel.disabled=true; busyMessage="正在保存站点设置…"; updateView(); }
  try {
    const values = new window.FormData(form); const name=String(values.get("name") ?? ""), about=String(values.get("about") ?? "");
    if (editingSiteID) {
      form.querySelector<HTMLInputElement>('input[name="name"]')!.disabled=true; form.querySelector<HTMLTextAreaElement>('textarea[name="about"]')!.disabled=true;
      const id=editingSiteID, generation=publicationState(id).generation, updated=await Library.updateSite(id,name,about), selected=mergeSitePublication(sites.find(entry=>entry.id===id),updated,generation);
      sitesRevision++; sites=sites.map(entry=>entry.id===id ? selected : entry);
      if (site?.id===id) site=selected;
      dialog.close(); clearError(); updateView(); return;
    }
    const created = await Library.createSite(name, about);
    sitesRevision++; sites=[...sites.filter(entry=>entry.id!==created.id),created]; updateView();
    let refreshError: unknown;
    try { await reloadSites(); } catch(e) { refreshError=e; }
    if (revision !== navigationRevision) { if(refreshError) showError(`站点已创建，但站点列表刷新失败：${String(refreshError)}`); updateView(); return; }
    site = created; favoritesView = false; subscription = null; item = null; articles = []; article = null; draft=null; drafts=[]; draftsView=false; editing = false; dirty = false; dialog.close();
    if (refreshError) showError(`站点已创建，但站点列表刷新失败：${String(refreshError)}`); else clearError(); updateView();
  } catch (e) { showError(e); dialog.close(); } finally { button.disabled = false; if(isUpdate) {saving=false;cancel.disabled=false;form.querySelector<HTMLInputElement>('input[name="name"]')!.disabled=false;form.querySelector<HTMLTextAreaElement>('textarea[name="about"]')!.disabled=false;busyMessage="";updateView();} }
});
$("#new-feed").addEventListener("click", () => { if (canLeave()) { navigationRevision++; feedForm.reset(); feedDialog.showModal(); } });
$("#cancel-feed").addEventListener("click", () => feedDialog.close());
feedDialog.addEventListener("close", () => { navigationRevision++; });
async function refreshSubscription(address: string): Promise<void> {
  if (refreshingSubscriptions.has(address)) return;
  refreshingSubscriptions.add(address); updateView();
  try { await Library.refreshSubscription(address); }
  finally {
    try { if (await reloadSubscriptions() && subscription?.url === address) { subscription = subscriptions.find(entry => entry.url === address) ?? subscription; if (item) { const current = subscription.feed.items.find(entry => entry.key === item?.key); if (current) selectFeedItem(current); } } }
    finally { refreshingSubscriptions.delete(address); updateView(); }
  }
}
async function refreshAutomatically() {
  if (autoRefreshing) return; autoRefreshing = true;
  try { await refreshDue([...subscriptions], refreshSubscription, Date.now(), 15 * 60 * 1000); } finally { autoRefreshing = false; }
}
refreshFeed.addEventListener("click", async () => {
  if (!subscription) return; clearError();
  try { await refreshSubscription(subscription.url); } catch (e) { showError(e); }
});
unfollow.addEventListener("click", async () => {
  if (!subscription || saving) return;
  const address = subscription.url;
  if (!window.confirm("取消订阅将移除此订阅及其收藏记录。已固定的 IPFS 内容和本地缓存不会立即删除。继续吗？")) return;
  navigationRevision++; readerRevision++; saving = true; busyMessage = "正在取消订阅…"; updateView(); clearError();
  try {
    await Library.unfollow(address); await reloadSubscriptions();
    if (subscription?.url === address) { subscription = null; item = null; favoritesView = false; }
  } catch (e) { showError(e); }
  finally { saving = false; busyMessage = ""; updateView(); }
});
pinSubscription.addEventListener("click", async () => {
  if (!subscription || pinSubscription.disabled) return; const address=subscription.url; pinSubscription.disabled=true; clearError();
  try { await Library.pinSubscription(address); await reloadSubscriptions(); if(subscription?.url===address) subscription=subscriptions.find(entry=>entry.url===address) ?? subscription; }
  catch(e) { showError(e); } finally { updateView(); }
});
const storageDialog = $<HTMLDialogElement>("#offline-storage-dialog");
const cleanupOffline = $<HTMLButtonElement>("#cleanup-offline");
function formatBytes(value:number):string { const units=["B","KiB","MiB","GiB","TiB"];let n=value,i=0;while(n>=1024 && i<units.length-1){n/=1024;i++;}return `${i===0 ? n.toFixed(0) : n.toFixed(1)} ${units[i]}`; }
async function updateOfflineUsage() {const usage=await Library.offlineMediaUsage();$("#offline-storage-usage").textContent=`已管理媒体 ${formatBytes(usage.totalBytes)} · 可清理 ${formatBytes(usage.reclaimableBytes)}（${usage.reclaimableFiles} 个文件）`;cleanupOffline.disabled=usage.reclaimableFiles===0;}
$("#offline-storage").addEventListener("click",async()=>{storageDialog.showModal();$("#offline-storage-result").textContent="";cleanupOffline.disabled=true;try{await updateOfflineUsage();}catch(e){$("#offline-storage-usage").textContent=`无法计算空间：${String(e)}`;}});
$("#close-offline-storage").addEventListener("click",()=>storageDialog.close());
cleanupOffline.addEventListener("click",async()=>{cleanupOffline.disabled=true;try{const result=await Library.cleanupOfflineMedia();await updateOfflineUsage();$("#offline-storage-result").textContent=`已清理 ${formatBytes(result.reclaimedBytes)}（${result.reclaimedFiles} 个文件）`;}catch(e){$("#offline-storage-result").textContent=`清理未完成，部分文件可能已清理：${String(e)}`;try{await updateOfflineUsage();}catch{}}});
$("#show-favorites").addEventListener("click",()=>{ if(!canLeave()) return; navigationRevision++; site=null; subscription=null; item=null; article=null; draft=null; draftsView=false; editing=false; dirty=false; favoritesView=true; clearError();updateView(); });
starItem.addEventListener("click",async()=>{
  if(!subscription || !item) return; const address=subscription.url, key=item.key || item.id || item.link; if(!key) { showError("文章缺少可用标识"); return; } const starred=!item.starred;
  starItem.disabled=true; clearError();
  try { await Library.starSubscription(address,key,starred); await reloadSubscriptions(); if(subscription?.url===address) { subscription=subscriptions.find(entry=>entry.url===address) ?? subscription; if(item && (item.key || item.id || item.link)===key) item=subscription.feed.items.find(entry=>(entry.key || entry.id || entry.link)===key) ?? item; } }
  catch(e) { showError(e); } finally {updateView();}
});
shareItem.addEventListener("click",async()=>{
  if(!item?.link) return; try { const u=new URL(item.link);if((u.protocol!=="http:" && u.protocol!=="https:") || u.username || u.password) throw new Error("文章没有可分享的网页链接"); shareURL.value=u.href; try {await window.navigator.clipboard.writeText(u.href);shareStatus.textContent="链接已复制";} catch {shareURL.hidden=false;shareURL.focus();shareURL.select();shareStatus.textContent="请按 Ctrl+C 复制";} } catch(e) {showError(e);}
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
    if (imported.id) {
      sitesRevision++; sites=[...sites.filter(entry=>entry.id!==imported.id),imported];
      site=imported; subscription=null; item=null; article=null; draft=null; drafts=[]; draftsView=false; articles=[]; editing=false; dirty=false; navigationRevision++;
      try { await reloadSites(); } catch(e) { showError(`站点已导入，但站点列表刷新失败：${String(e)}`); }
      try { articles=await Library.articles(imported.id); articleLoadFailures.delete(imported.id); }
      catch(e) { articleLoadFailures.add(imported.id); showError(`站点已导入，但文章加载失败；请点击站点重试：${String(e)}`); }
    }
  } catch (e) { showError(e); }
  finally { saving = false; title.disabled = false; content.disabled = false; updateView(); }
});
exportPlanet.addEventListener("click", async () => {
  if (!site || saving) return;
  if (!window.confirm(`${dirty ? "当前文章尚未保存，备份只包含已保存内容。\n\n" : ""}备份不包含本机草稿，并包含站点私钥，请妥善保管。继续吗？`)) return;
  saving = true; updateView(); clearError();
  try { const path = await Library.exportPlanet(site.id); if (path) window.alert(`备份已保存到：\n${path}`); }
  catch (e) { showError(e); } finally { saving = false; updateView(); }
});
selectTemplate.addEventListener("click", async () => {
  if (!site || saving) return;
  const siteID=site.id; navigationRevision++; saving=true; updateView(); clearError();
  try {
    const generation=publicationState(siteID).generation, updated=await Library.selectTemplate(siteID);
    if (updated.id) {
      const selected=mergeSitePublication(sites.find(entry=>entry.id===siteID),updated,generation);
      markLocalChange(siteID); sitesRevision++; sites=sites.map(entry=>entry.id===siteID ? selected : entry);
      if (site?.id===siteID) site=selected;
      window.alert(selected.imported ? `已选择模板：${selected.templateName}\n可尝试导出备份；不支持的原站点设置会提示原因。` : `已选择模板：${selected.templateName}\n建议先导出网页检查效果。`);
      if (selected.publishPending) void queuePublication(siteID);
    }
  } catch(e) { showError(e); } finally { saving=false; updateView(); }
});
templateSettingsButton.addEventListener("click", async () => {
  if (!site || site.templateName !== "Sepia" || saving) return;
  const siteID=site.id; navigationRevision++; saving=true; title.disabled=true; content.disabled=true; busyMessage="正在读取模板设置…"; updateView(); clearError();
  try {
    const settings=await Library.templateSettings(siteID);
    if(site?.id!==siteID || site.templateName!=="Sepia") return;
    const fields=$("#template-setting-fields"); fields.replaceChildren();
    for(const [key,caption,fallback] of sepiaFields) {
      const field=document.createElement("label"), input=document.createElement("input"); field.textContent=caption;
      input.name=key; input.type="text"; input.required=true; input.pattern="#[0-9a-fA-F]{6}"; input.maxLength=7; input.value=settings[key] ?? fallback;
      field.append(input); fields.append(field);
    }
    templateSettingsForm.querySelector<HTMLInputElement>('[name="showRef"]')!.checked=settings.showRef==="true";
    editingTemplateSiteID=siteID; templateSettingsDialog.showModal();
  } catch(e) { showError(e); } finally { saving=false; title.disabled=false; content.disabled=false; busyMessage=""; updateView(); }
});
$("#cancel-template-settings").addEventListener("click",()=>{if(!saving)templateSettingsDialog.close();});
templateSettingsDialog.addEventListener("cancel",event=>{if(saving)event.preventDefault();});
templateSettingsForm.addEventListener("submit",async event=>{
  event.preventDefault(); if(saving || !editingTemplateSiteID || site?.id!==editingTemplateSiteID || site.templateName!=="Sepia") return;
  const siteID=editingTemplateSiteID, values:Record<string,string>={};
  for(const [key] of sepiaFields) values[key]=templateSettingsForm.querySelector<HTMLInputElement>(`[name="${key}"]`)!.value;
  values.showRef=String(templateSettingsForm.querySelector<HTMLInputElement>('[name="showRef"]')!.checked);
  const controls=Array.from(templateSettingsForm.querySelectorAll<HTMLInputElement|HTMLButtonElement>("input,button"));
  const generation=publicationState(siteID).generation; navigationRevision++; saving=true; title.disabled=true; content.disabled=true; busyMessage="正在保存模板设置…"; for(const control of controls)control.disabled=true; updateView(); clearError();
  try {
    const updated=await Library.saveTemplateSettings(siteID,JSON.stringify(values));
    const selected=mergeSitePublication(sites.find(entry=>entry.id===siteID),updated,generation);
    markLocalChange(siteID); sitesRevision++; sites=sites.map(entry=>entry.id===siteID ? selected : entry); if(site?.id===siteID)site=selected;
    templateSettingsDialog.close();
  } catch(e) { showError(e); } finally { saving=false; title.disabled=false; content.disabled=false; busyMessage=""; for(const control of controls)control.disabled=false; updateView(); }
});
function updatePinnableControls() {
  for(const id of ["#pinnable-request","#pinnable-status","#pinnable-disable"]) $<HTMLButtonElement>(id).disabled=saving || !pinnableConfigured;
  pinnableForm.querySelector<HTMLInputElement>('[name="endpoint"]')!.disabled=saving;
  pinnableForm.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled=saving;
  $<HTMLButtonElement>("#pinnable-close").disabled=saving;
}
async function withPinnableOperation(action:()=>Promise<void>) {
  if(saving || !pinnableSiteID || site?.id!==pinnableSiteID)return;
  navigationRevision++; saving=true;title.disabled=true;content.disabled=true;updatePinnableControls();updateView();
  try { await action(); } catch(e) { pinnableResult.textContent=`操作失败：${e instanceof Error ? e.message : String(e)}`; }
  finally { saving=false;title.disabled=false;content.disabled=false;updatePinnableControls();updateView(); }
}
pinnableButton.addEventListener("click",async()=>{
  if(!site || saving)return;const siteID=site.id;navigationRevision++;saving=true;title.disabled=true;content.disabled=true;updateView();clearError();
  try {
    const configured=await Library.pinnableConfigured(siteID);if(site?.id!==siteID)return;
    pinnableSiteID=siteID;pinnableConfigured=configured;pinnableForm.reset();pinnableResult.textContent=configured ? "已配置固定服务地址。" : "尚未配置固定服务。";pinnableDialog.showModal();
  } catch(e){showError(e);} finally {saving=false;title.disabled=false;content.disabled=false;updatePinnableControls();updateView();}
});
$("#pinnable-close").addEventListener("click",()=>{if(!saving)pinnableDialog.close();});
pinnableDialog.addEventListener("cancel",event=>{if(saving)event.preventDefault();});
pinnableDialog.addEventListener("close",()=>{pinnableForm.reset();pinnableSiteID="";});
pinnableForm.addEventListener("submit",event=>{
  event.preventDefault();const input=pinnableForm.querySelector<HTMLInputElement>('[name="endpoint"]')!,endpoint=input.value.trim();
  if(!endpoint){pinnableResult.textContent="请输入固定服务地址。";return;}
  void withPinnableOperation(async()=>{await Library.configurePinnable(pinnableSiteID,endpoint);input.value="";pinnableConfigured=true;pinnableResult.textContent="已保存地址；下次站点发布成功后会请求固定。";});
});
$("#pinnable-disable").addEventListener("click",()=>{
  if(!pinnableConfigured || saving || !window.confirm("停用 Pinnable 固定服务并删除本机保存的地址？已固定的内容不会自动取消固定。"))return;
  void withPinnableOperation(async()=>{await Library.configurePinnable(pinnableSiteID,"");pinnableConfigured=false;pinnableResult.textContent="已停用固定服务。";});
});
$("#pinnable-request").addEventListener("click",()=>{
  if(!pinnableConfigured || !window.confirm("向配置的 Pinnable 地址请求固定当前站点？若服务尚未报告 IPNS，请先确认该地址属于当前站点。"))return;
  void withPinnableOperation(async()=>{await Library.requestPinnablePin(pinnableSiteID);pinnableResult.textContent="固定请求已接受，稍后可查询状态。";});
});
$("#pinnable-status").addEventListener("click",()=>{
  if(!pinnableConfigured)return;void withPinnableOperation(async()=>{
    const status=await Library.pinnableStatus(pinnableSiteID), reported=status.last_known_ipns?.replace(/^\/ipns\//,"").replace(/\/$/,"") ?? "";
    const identity=reported && site?.ipns ? (reported===site.ipns ? "IPNS 一致" : "IPNS 与当前站点不一致") : "尚无法核对站点 IPNS";
    const version=status.last_known_cid && site?.lastPublishedCID ? (status.last_known_cid===site.lastPublishedCID ? (reported && reported===site.ipns && status.status.toLowerCase()==="pinned" ? "当前版本已固定" : "服务报告了相同 CID，尚未确认当前站点已固定") : "服务报告的不是当前发布版本") : "尚无法确认当前版本";
    pinnableResult.textContent=`状态：${status.status} · ${identity} · ${version}${status.last_known_cid ? ` · CID：${status.last_known_cid}` : ""}`;
  });
});
let serviceSiteID="", serviceConfigured=false, serviceGeneration=0;
let serviceSavedName="",serviceSavedServer="",serviceSavedScript="";
const serviceKind=serviceForm.querySelector<HTMLSelectElement>('[name="kind"]')!,serviceName=serviceForm.querySelector<HTMLInputElement>('[name="name"]')!,serviceToken=serviceForm.querySelector<HTMLInputElement>('[name="token"]')!;
const serviceScript=serviceForm.querySelector<HTMLInputElement>('[name="scriptId"]')!,serviceScriptLabel=$("#service-script-label");
const serviceServer=serviceForm.querySelector<HTMLInputElement>('[name="server"]')!,serviceServerLabel=$("#service-server-label");
const serviceRange=serviceForm.querySelector<HTMLSelectElement>('[name="statsRange"]')!,serviceRangeLabel=$("#service-range-label");
function serviceHasUnsavedChanges(){return serviceName.value.trim()!==serviceSavedName || !!serviceToken.value.trim() || (serviceKind.value==="plausible" && (serviceServer.value.trim()!==serviceSavedServer || serviceScript.value.trim()!==serviceSavedScript));}
function updateServiceControls(){
 serviceScriptLabel.hidden=serviceKind.value!=="plausible";
 serviceServerLabel.hidden=serviceKind.value!=="plausible";
 serviceRangeLabel.hidden=serviceKind.value!=="plausible";
 serviceToken.required=serviceKind.value!=="plausible";
 for(const control of serviceForm.querySelectorAll<HTMLInputElement|HTMLButtonElement|HTMLSelectElement>("input,button,select")) control.disabled=saving;
 $<HTMLButtonElement>("#service-stats").disabled=saving || !serviceConfigured || serviceKind.value!=="plausible" || serviceHasUnsavedChanges();
 for(const id of ["#service-pin","#service-pin-status"]) $<HTMLButtonElement>(id).disabled=saving || !serviceConfigured || serviceKind.value!=="filebase" || serviceHasUnsavedChanges();
 for(const id of ["#service-stats","#service-pin","#service-pin-status"]) $<HTMLButtonElement>(id).title=serviceHasUnsavedChanges()?"请先保存服务配置":"";
 $<HTMLButtonElement>("#service-disable").disabled=saving || (!serviceConfigured && serviceKind.value!=="plausible");
}
async function loadService(){
 if(!serviceSiteID || saving)return;const id=serviceSiteID,kind=serviceKind.value,generation=++serviceGeneration;
 saving=true;updateServiceControls();updateView();
 try {const info=await Library.serviceInfo(id,kind);if(generation!==serviceGeneration || site?.id!==id)return;serviceConfigured=info.configured;serviceName.value=info.name ?? "";serviceScript.value=kind==="plausible" ? info.scriptId ?? "" : "";serviceServer.value=kind==="plausible" ? info.server ?? (info.trackingWarning ? "" : "https://plausible.io") : "";serviceSavedName=serviceName.value.trim();serviceSavedServer=serviceServer.value.trim();serviceSavedScript=serviceScript.value.trim();serviceToken.value="";serviceResult.textContent=info.trackingWarning || (info.pendingCid ? `上次请求结果不确定，请先在 Filebase 检查 CID：${info.pendingCid}。` : info.configured ? `已配置${info.lastCid ? `，最近提交 CID：${info.lastCid}` : ""}。` : info.trackingEnabled ? "跟踪已配置；统计查询需要 API 令牌。" : "尚未配置。");}
 catch(e){serviceResult.textContent=`读取失败：${e instanceof Error?e.message:String(e)}`;}
 finally{saving=false;updateServiceControls();updateView();}
}
serviceButton.addEventListener("click",()=>{if(!site || saving)return;serviceSiteID=site.id;serviceForm.reset();serviceDialog.showModal();void loadService();});
serviceKind.addEventListener("change",()=>void loadService());
for(const control of [serviceName,serviceServer,serviceScript,serviceToken])control.addEventListener("input",()=>{updateServiceControls();if(serviceKind.value==="plausible" && serviceHasUnsavedChanges())serviceResult.textContent="配置有未保存的更改，请先保存后再查询。";});
$("#service-close").addEventListener("click",()=>{if(!saving)serviceDialog.close();});
serviceDialog.addEventListener("cancel",e=>{if(saving)e.preventDefault();});
serviceDialog.addEventListener("close",()=>{serviceGeneration++;serviceSiteID="";serviceForm.reset();});
async function serviceOperation(action:()=>Promise<void>){if(saving || !serviceSiteID || site?.id!==serviceSiteID)return;navigationRevision++;saving=true;title.disabled=true;content.disabled=true;updateServiceControls();updateView();try{await action();}catch(e){serviceResult.textContent=`操作失败：${e instanceof Error?e.message:String(e)}`;}finally{saving=false;title.disabled=false;content.disabled=false;updateServiceControls();updateView();}}
serviceForm.addEventListener("submit",e=>{e.preventDefault();const name=serviceName.value.trim(),token=serviceToken.value.trim();if(!name || (!token && serviceKind.value==="filebase")){serviceResult.textContent="请输入名称；Filebase 需要令牌。";return;}if(serviceKind.value==="filebase" && serviceConfigured && !window.confirm("修改 Filebase 名称或令牌会清除本机请求记录，可能导致重复固定及费用。请确认服务端状态后继续。"))return;void serviceOperation(async()=>{if(serviceKind.value==="plausible")await Library.configurePlausibleAt(serviceSiteID,name,token,serviceScript.value.trim(),serviceServer.value.trim());else await Library.configureService(serviceSiteID,serviceKind.value,name,token);serviceSavedName=name;serviceSavedServer=serviceServer.value.trim();serviceSavedScript=serviceScript.value.trim();serviceToken.value="";serviceConfigured=serviceConfigured || !!token;if(serviceKind.value==="plausible")markLocalChange(serviceSiteID);serviceResult.textContent=serviceKind.value==="filebase"?"已保存；下次发布后提交固定请求。":"已保存统计配置；重新发布站点后跟踪脚本生效。";});});
$("#service-disable").addEventListener("click",()=>{if((!serviceConfigured && serviceKind.value!=="plausible") || !window.confirm("删除当前服务的本机配置？远程内容不会删除；停用 Plausible 后需重新发布站点。"))return;void serviceOperation(async()=>{if(serviceKind.value==="plausible")markLocalChange(serviceSiteID);await Library.configureService(serviceSiteID,serviceKind.value,"","");serviceConfigured=false;serviceName.value="";serviceToken.value="";serviceResult.textContent="已停用；如更改了跟踪脚本，请重新发布站点。";});});
$("#service-stats").addEventListener("click",()=>{if(!serviceConfigured || serviceKind.value!=="plausible" || serviceHasUnsavedChanges())return;const period=serviceRange.value;void serviceOperation(async()=>{const stats=await Library.plausibleStatsRange(serviceSiteID,period);const label=period==="day"?"今天":period==="30d"?"最近 30 天":"最近 7 天";serviceResult.textContent=`${label}：访客 ${stats.visitors}，浏览量 ${stats.pageviews}。`;});});
$("#service-pin").addEventListener("click",()=>{if(!serviceConfigured || serviceKind.value!=="filebase" || serviceHasUnsavedChanges() || !window.confirm("将为当前版本提交新的 Filebase 固定请求。若此前请求已被接受，可能产生重复固定和费用；请先检查 Filebase。继续吗？"))return;void serviceOperation(async()=>{await Library.requestFilebasePin(serviceSiteID);serviceResult.textContent="请求已提交；请查询固定状态。";});});
$("#service-pin-status").addEventListener("click",()=>{if(!serviceConfigured || serviceKind.value!=="filebase" || serviceHasUnsavedChanges())return;void serviceOperation(async()=>{const status=await Library.filebaseStatus(serviceSiteID);serviceResult.textContent=`状态：${status.status} · CID：${status.pin.cid}${status.pin.cid===site?.lastPublishedCID ? "（当前发布版本）" : "（不是当前发布版本）"}`;});});
deleteArticle.addEventListener("click", async () => {
  if (!site || (!article && !draft) || saving || !window.confirm(draft ? "确定删除这份草稿？此操作无法撤销。" : "确定删除这篇文章？此操作无法撤销。")) return;
  const siteID = site.id; navigationRevision++; saving = true; deleteArticle.disabled = true; title.disabled = true; content.disabled = true; clearError();
  try {
    if (draft) { const id=draft.id; await Library.deleteDraft(siteID,id); drafts=drafts.filter(entry=>entry.id!==id); draft=null; article=null; }
    else if (article) { const deletedID = article.id; await Library.deleteArticle(siteID, deletedID); if (playingSiteID === siteID && playingArticleID === deletedID) closeAudio(); markLocalChange(siteID); articles = await Library.articles(siteID); article = null; }
    editing = false; dirty = false; updateView();
  }
  catch (e) { showError(e); } finally { saving = false; deleteArticle.disabled = false; title.disabled = false; content.disabled = false; updateView(); }
});
feedForm.addEventListener("submit", async (event) => {
  event.preventDefault(); const button = feedForm.querySelector<HTMLButtonElement>('button[type="submit"]')!; button.disabled = true;
  const revision = ++navigationRevision;
  try {
    const address = String(new FormData(feedForm).get("url") ?? "");
    const entry = await Library.follow(address); await reloadSubscriptions();
    if (revision !== navigationRevision) { updateView(); return; }
    site = null; favoritesView = false; subscription = entry; item = null; article = null; draft=null; draftsView=false; editing = false; dirty = false; feedDialog.close(); clearError(); updateView();
  } catch (e) { feedDialog.close(); showError(e); } finally { button.disabled = false; }
});
newArticle.addEventListener("click", startArticle);
playArticleAudio.addEventListener("click", () => {
  if (!site || !article?.audioFilename) return;
  const siteID = site.id, articleID = article.id, label = article.title || article.audioFilename, revision = ++audioRevision;
  audioRequest = audioRequest.catch(() => {}).then(async () => {
    if (revision !== audioRevision) return;
    try {
      const address = await Library.audioURL(siteID, articleID);
      if (revision !== audioRevision) { await Library.stopAudio(address); return; }
      playingSiteID = siteID; playingArticleID = articleID; startPlayback(address,label,false);
    } catch (e) { if (revision === audioRevision) showError(e); }
  });
});
function startPlayback(address:string,label:string,video:boolean) { const next=video ? globalVideo : globalAudio; if(activeMedia!==next) {activeMedia.pause();activeMedia.removeAttribute("src");} activeMedia=next;globalAudio.hidden=video;globalVideo.hidden=!video;audioTitle.textContent=label;activeMedia.src=address;audioPlayer.hidden=false;void activeMedia.play().catch(()=>{}); }
function closeAudio() { audioRevision++; activeMedia.pause(); const previous = activeMedia.src; activeMedia.removeAttribute("src"); audioPlayer.hidden = true; playingSiteID = ""; playingArticleID = ""; void Library.stopAudio(previous).catch(showError); }
$("#play-feed-audio").addEventListener("click", () => {
  if (!item || !subscription) return;
  const current=item, address=subscription.url, key=current.key || current.id || current.link;
  if (current.audioFilename && current.contentCid && key) {
    const revision=++audioRevision, label=current.title || current.audioFilename;
    audioRequest=audioRequest.catch(()=>{}).then(async()=>{
      if(revision!==audioRevision) return;
      try { const url=await Library.subscriptionAudioURL(address,key); if(revision!==audioRevision) {await Library.stopAudio(url);return;} playingSiteID="";playingArticleID="";startPlayback(url,label,false); }
      catch(e) {if(revision===audioRevision) showError(e);}
    });
    return;
  }
  if (!current.audioUrl) return;
  if (current.offlineMedia?.[current.audioUrl] && key) {
    const revision=++audioRevision, label=current.title || "订阅音频";
    audioRequest=audioRequest.catch(()=>{}).then(async()=>{if(revision!==audioRevision)return;try {const url=await Library.subscriptionAudioURL(address,key);if(revision!==audioRevision){await Library.stopAudio(url);return;}playingSiteID="";playingArticleID="";startPlayback(url,label,false);}catch(e){if(revision===audioRevision)showError(e);}});return;
  }
  if (subscription.offlinePinned) { showError("离线音频不可用，请重新固定订阅。"); return; }
  try {
    const url = new URL(current.audioUrl); if ((url.protocol !== "https:" && url.protocol !== "http:") || url.username || url.password) throw new Error("订阅音频地址无效");
    const old = activeMedia.src; audioRevision++; playingSiteID = ""; playingArticleID = ""; startPlayback(url.href,current.title || "订阅音频",false);
    void Library.stopAudio(old).catch(showError);
  } catch (e) { showError(e); }
});
$("#play-feed-video").addEventListener("click",()=>{
  if(!item?.videoUrl || !subscription)return;const mediaURL=item.videoUrl,current=item,address=subscription.url,key=current.key || current.id || current.link;
  if(current.offlineMedia?.[mediaURL] && key) {const revision=++audioRevision;audioRequest=audioRequest.catch(()=>{}).then(async()=>{if(revision!==audioRevision)return;try {const url=await Library.subscriptionMediaURL(address,key,mediaURL);if(revision!==audioRevision){await Library.stopAudio(url);return;}playingSiteID="";playingArticleID="";startPlayback(url,current.title || "订阅视频",true);}catch(e){if(revision===audioRevision)showError(e);}});return;}
  if(subscription.offlinePinned){showError("离线视频不可用，请重新固定订阅。");return;}
  try {const url=new URL(mediaURL);if((url.protocol!=="http:" && url.protocol!=="https:") || url.username || url.password)throw new Error("订阅视频地址无效");const old=activeMedia.src;audioRevision++;playingSiteID="";playingArticleID="";startPlayback(url.href,current.title || "订阅视频",true);void Library.stopAudio(old).catch(showError);}catch(e){showError(e);}
});
$("#close-audio").addEventListener("click", closeAudio);
for(const media of [globalAudio,globalVideo]) media.addEventListener("error", () => { if (!audioPlayer.hidden && activeMedia===media) showError("无法播放媒体，文件可能不存在或格式不受 Windows 支持。"); });
$("#audio-back").addEventListener("click", () => { activeMedia.currentTime = Math.max(0, activeMedia.currentTime - 10); });
$("#audio-forward").addEventListener("click", () => { activeMedia.currentTime = Math.min(Number.isFinite(activeMedia.duration) ? activeMedia.duration : activeMedia.currentTime + 10, activeMedia.currentTime + 10); });
async function removeAttachment(name: string) {
  if (!site || (!article && !draft) || saving) return;
  const existingDraftAttachment = !!draft && !(draft.attachments ?? []).includes(name), marked = !!draft?.removedAttachments?.includes(name);
  if (!marked && !window.confirm(`删除附件“${name}”？文章中的链接不会自动删除。`)) return;
  const siteID = site.id, articleID = article?.id ?? "", draftID = draft?.id ?? "";
  navigationRevision++; saving = true; busyMessage = "正在处理附件，可能需等待发布完成…"; updateView(); clearError();
  try {
    if (draftID) {
      const updated = existingDraftAttachment ? await Library.setDraftExistingAttachmentRemoved(siteID, draftID, name, !marked) : await Library.removeDraftAttachment(siteID, draftID, name);
      if (site?.id === siteID && draft?.id === draftID) { draft = updated; drafts = drafts.map(entry => entry.id === draftID ? updated : entry); void renderPreview(); }
      return;
    }
    const updated = await Library.removeAttachment(siteID, articleID, name);
    if (playingSiteID === siteID && playingArticleID === articleID && article?.audioFilename === name) closeAudio();
    markLocalChange(siteID);
    if (site?.id === siteID && article?.id === articleID) { article = updated; articles = await Library.articles(siteID); void renderPreview(); }
  } catch (e) { showError(e); }
  finally { saving = false; busyMessage = ""; updateView(); }
}
addAttachment.addEventListener("click", async () => {
  if (!site || (!article && !draft) || saving) return;
  const siteID = site.id, articleID = article?.id ?? "", draftID = draft?.id ?? "", previous = new Set(draft ? draft.attachments ?? [] : article?.attachments ?? []);
  navigationRevision++; saving = true; busyMessage = "正在处理附件，可能需等待发布完成…"; updateView(); clearError();
  try {
    const updated = draftID ? await Library.addDraftAttachment(siteID, draftID) : await Library.addAttachment(siteID, articleID);
    if (!updated.id || site?.id !== siteID || (draftID ? draft?.id !== draftID : article?.id !== articleID)) return;
    const name = updated.attachments?.find(item => !previous.has(item));
    if (draftID) { draft = updated as Draft; drafts = drafts.map(entry => entry.id === draftID ? draft! : entry); }
    else { article = updated as Article; markLocalChange(siteID); articles = await Library.articles(siteID); }
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
  const initialNavigation=navigationRevision, initialSites=++sitesRevision, initialSubscriptions=++subscriptionsRevision;
  const [loadedSites, loadedSubscriptions] = await Promise.allSettled([Library.sites(), Library.subscriptions()]);
  if (loadedSites.status === "fulfilled") {
    if (sitesRevision===initialSites && (navigationRevision===initialNavigation || (!site && sites.length===0))) sites=loadedSites.value;
  } else showError(loadedSites.reason);
  if (loadedSubscriptions.status === "fulfilled") {
    if (subscriptionsRevision===initialSubscriptions) subscriptions=loadedSubscriptions.value;
  } else showError(loadedSubscriptions.reason);
  if (navigationRevision===initialNavigation && !site && sites[0]) {
    const initialSite=sites[0]; site=initialSite;
    try { const loaded=await Library.articles(initialSite.id); if (navigationRevision===initialNavigation && site?.id===initialSite.id && !editing && !dirty) articles=loaded; }
    catch (e) { if(navigationRevision===initialNavigation) { articleLoadFailures.add(initialSite.id); showError(e); } }
  }
  updateView();
  for (const entry of sites) if (entry.publishPending) void queuePublication(entry.id);
  void refreshAutomatically(); window.setInterval(() => void refreshAutomatically(), 15 * 60 * 1000);
}
initUpdates();
document.querySelector(".version")!.textContent=release.version;
void init();
