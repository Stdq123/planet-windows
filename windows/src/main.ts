import { Library, type Article, type Draft, type Site, type Subscription, type Item, type KuboStatus, type SearchResult, type Builtin } from "./mygo";
import { initUpdates } from "./updates";
import { initXSync } from "./x-sync";
import { createDashboard } from "./dashboard";
import release from "../release.json";
import { refreshDue } from "./auto-refresh";
import { gatewayKey, planetShareURL, selectedGateway, sitePublicURL } from "./gateway";
import { localGatewayResource } from "./local-gateway";
import { feedArticleBase, feedReaderDocument } from "./feed-reader";
import { hasDialogChanges, openDialog, prepareChoices, prepareDialogs, readingStyles, rememberDialog } from "./ui";

prepareChoices(document);
prepareDialogs(document);

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
const ownReader = $("#own-reader"), ownReaderPreview=$<HTMLIFrameElement>("#own-reader-preview");
const refreshFeed = $<HTMLButtonElement>("#refresh-feed"), pinSubscription = $<HTMLButtonElement>("#pin-subscription");
const unfollow = $<HTMLButtonElement>("#unfollow");
const starItem = $<HTMLButtonElement>("#star-item"), shareItem = $<HTMLButtonElement>("#share-item"), shareURL = $<HTMLInputElement>("#share-url"), shareStatus = $("#share-status");
const exportSite = $<HTMLButtonElement>("#export-site");
const publishSite = $<HTMLButtonElement>("#publish-site");
const exportPlanet = $<HTMLButtonElement>("#export-planet"), importPlanet = $<HTMLButtonElement>("#import-planet");
const selectTemplate = $<HTMLButtonElement>("#select-template");
const templateSettingsButton = $<HTMLButtonElement>("#template-settings"), templateSettingsDialog = $<HTMLDialogElement>("#template-settings-dialog"), templateSettingsForm = $<HTMLFormElement>("#template-settings-form");
const pinnableButton=$<HTMLButtonElement>("#pinnable-settings"), pinnableDialog=$<HTMLDialogElement>("#pinnable-dialog"), pinnableForm=$<HTMLFormElement>("#pinnable-form"), pinnableResult=$("#pinnable-result");
const telegramButton=$<HTMLButtonElement>("#telegram-settings"), telegramDialog=$<HTMLDialogElement>("#telegram-dialog"), telegramForm=$<HTMLFormElement>("#telegram-form"), telegramResult=$("#telegram-result");
const telegramChat=telegramForm.querySelector<HTMLInputElement>('[name="chatId"]')!, telegramToken=telegramForm.querySelector<HTMLInputElement>('[name="token"]')!;
const serviceButton=$<HTMLButtonElement>("#service-settings"),serviceDialog=$<HTMLDialogElement>("#service-dialog"),serviceForm=$<HTMLFormElement>("#service-form"),serviceResult=$("#service-result");
const editSite = $<HTMLButtonElement>("#edit-site");
const removeSite = $<HTMLButtonElement>("#remove-site");
const removedSitesDialog = $<HTMLDialogElement>("#removed-sites-dialog"), removedSiteList = $("#removed-site-list");
const deleteArticle = $<HTMLButtonElement>("#delete-article");
const showDrafts = $<HTMLButtonElement>("#show-drafts"), saveDraft = $<HTMLButtonElement>("#save-draft");
const siteAddress = $("#site-address");
const publishedAddress = $("#published-address"), publishedURL = $<HTMLInputElement>("#published-url");
const copyStatus = $("#copy-status");
const publicGateway = $<HTMLSelectElement>("#public-gateway"), copyPublishedURL = $<HTMLButtonElement>("#copy-published-url");
const workspaceMenu = $<HTMLDetailsElement>("#workspace-menu");
const postsPanel = $("#posts-panel"), shell = $(".shell"), togglePosts = $<HTMLButtonElement>("#toggle-posts");
const workspace = $(".workspace"), metadata = $("#editor-metadata"), metadataToggle = $("#toggle-metadata");
const skipToMain = $<HTMLAnchorElement>("#skip-to-main");
skipToMain.addEventListener("click", event => {
  event.preventDefault();
  (workspace.hidden ? $("#dashboard") : workspace).focus();
});
let editorView: "write" | "preview" | "split" = "write";
function syncMetadataLayout() {
  if (metadata.hidden) return;
  const bounds = editor.getBoundingClientRect();
  if (bounds.height <= 0) return;
  // Leave room for the popover shadow inside the editor's clipped surface.
  const available = bounds.bottom - metadata.getBoundingClientRect().top - 32;
  metadata.style.maxHeight = `${Math.max(0, Math.min(440, Math.floor(available)))}px`;
}
function syncEditorView() {
  const canSplit = workspace.clientWidth >= 960;
  const view = editorView === "split" && !canSplit ? "write" : editorView;
  editor.dataset.view = view;
  $("#writing-pane").hidden = view === "preview";
  $("#preview-pane").hidden = view === "write";
  $("#view-split").hidden = !canSplit;
  for (const choice of ["write", "preview", "split"]) $("#view-" + choice).setAttribute("aria-pressed", String(view === choice));
  syncMetadataLayout();
}
for (const choice of ["write", "preview", "split"] as const) $("#view-" + choice).addEventListener("click", () => {
  editorView = choice;
  if (choice === "preview") setMetadataOpen(false);
  syncEditorView();
});
function setMetadataOpen(open: boolean) {
  metadata.hidden = !open; metadataToggle.setAttribute("aria-expanded", String(open));
  if (open) syncMetadataLayout();
}
metadataToggle.addEventListener("click", () => setMetadataOpen(!!metadata.hidden));
$("#close-metadata").addEventListener("click", () => { setMetadataOpen(false); metadataToggle.focus(); });
document.addEventListener("click", event => {
  if (!metadata.hidden && !metadata.contains(event.target as Node) && !metadataToggle.contains(event.target as Node)) setMetadataOpen(false);
}, true);
window.addEventListener("keydown", event => {
  if (event.key !== "Escape" || event.defaultPrevented || event.isComposing || event.keyCode === 229 || metadata.hidden || document.querySelector("dialog[open]")) return;
  event.preventDefault(); setMetadataOpen(false); metadataToggle.focus();
});
new window.ResizeObserver(syncEditorView).observe(workspace);
function setPostsCollapsed(collapsed: boolean) {
  postsPanel.hidden = collapsed;
  shell.classList.toggle("posts-collapsed", collapsed);
  togglePosts.setAttribute("aria-expanded", String(!collapsed));
  const label = collapsed ? "展开文章列表" : "收起文章列表";
  togglePosts.setAttribute("aria-label", label); togglePosts.title = label;
  syncEditorView();
}
togglePosts.addEventListener("click", () => setPostsCollapsed(!postsPanel.hidden));
for (const menu of [workspaceMenu, $<HTMLDetailsElement>(".sidebar-tools")]) {
  menu.addEventListener("click", event => {
    if (event.target instanceof window.HTMLButtonElement && !event.target.disabled) menu.open = false;
  });
  document.addEventListener("click", event => {
    if (!menu.contains(event.target as Node)) menu.open = false;
  });
  window.addEventListener("keydown", event => {
    if (event.key === "Escape" && menu.open && !document.querySelector("dialog[open]")) {
      menu.open = false; menu.querySelector("summary")!.focus();
    }
  });
}
const publicationDialog = $<HTMLDialogElement>("#publication-dialog");
$("#show-publication").addEventListener("click", () => {
  if (publicationDialog.open) return;
  publicationDialog.addEventListener("close", () => {
    if (!workspace.hidden && !workspaceMenu.hidden && !document.querySelector("dialog[open]")) workspaceMenu.querySelector("summary")!.focus();
  }, { once: true });
  openDialog(publicationDialog);
});
$("#close-publication").addEventListener("click", () => publicationDialog.close());

const ipfsDialog = $<HTMLDialogElement>("#ipfs-status-dialog"), ipfsRefresh = $<HTMLButtonElement>("#refresh-ipfs-status");
const ipfsToggle = $<HTMLButtonElement>("#toggle-ipfs"), ipfsOpenSite = $<HTMLAnchorElement>("#ipfs-open-site");
const ipfsGC = $<HTMLButtonElement>("#ipfs-gc");
const ipfsResource = $<HTMLInputElement>("#ipfs-resource"), ipfsOpenResource = $<HTMLAnchorElement>("#ipfs-open-resource");
let ipfsStatusRevision = 0;
let ipfsStatus: KuboStatus | null = null, ipfsTimer: ReturnType<typeof setTimeout> | undefined;
let ipfsSamples: {time:number, input:number, output:number}[] = [];
function updateIPFSResource() {
  const href = ipfsStatus?.running ? localGatewayResource(ipfsStatus.gateway || "", ipfsResource.value) : null;
  ipfsOpenResource.setAttribute("aria-disabled",String(!href));
  if (href) ipfsOpenResource.href=href; else ipfsOpenResource.removeAttribute("href");
}
ipfsResource.addEventListener("input", updateIPFSResource);
function drawIPFSTraffic() {
  const now=Date.now(); ipfsSamples=ipfsSamples.filter(sample=>now-sample.time<=300000).slice(-60);
  $("#ipfs-chart-empty").hidden=ipfsSamples.length>0;
  for (const [selector,key] of [["#ipfs-chart-in","input"],["#ipfs-chart-out","output"]] as const) {
    const group=$(selector); group.replaceChildren();
    if (!ipfsSamples.length) continue;
    const max=Math.max(1,...ipfsSamples.map(sample=>sample[key]));
    const points=ipfsSamples.map(sample=>({x:Math.max(0,300-(now-sample.time)/1000), y:(key==="input" ? 49 : 101)-Math.min(1,sample[key]/max)*45}));
    const line=document.createElementNS("http://www.w3.org/2000/svg","polyline");
    line.setAttribute("points",points.map(point=>`${point.x},${point.y}`).join(" "));group.append(line);
    for(const point of points){const dot=document.createElementNS("http://www.w3.org/2000/svg","circle");dot.setAttribute("cx",String(point.x));dot.setAttribute("cy",String(point.y));dot.setAttribute("r","1.4");group.append(dot);}
  }
}
function formatIPFSBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++; }
  return `${unit ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
}
async function refreshIPFSStatus(includeRepo = false) {
  if (ipfsRefresh.disabled) return;
  clearTimeout(ipfsTimer);
  const revision = ++ipfsStatusRevision;
  ipfsRefresh.disabled = true; ipfsToggle.disabled = true; ipfsGC.disabled=true;
  $("#ipfs-status-message").textContent = "正在读取节点状态…";
  for (const id of ["#ipfs-version", "#ipfs-peer-count", "#ipfs-rate", "#ipfs-total"]) $(id).textContent = "—";
  try {
    const status = await Library.ipfsStatus();
    if (revision !== ipfsStatusRevision || !ipfsDialog.open) return;
    ipfsStatus = status;
    updateIPFSResource();
    $("#ipfs-status-message").textContent = status.disabled ? "节点已关闭，自动发布和续期已暂停。" : !status.running ? "节点尚未启动。发布站点或读取 IPFS 内容时会启动节点。" : status.online ? "节点在线" : "节点正在离线模式运行";
    ipfsToggle.textContent = status.disabled || !status.running ? "启动节点" : "关闭节点";
    $("#ipfs-version").textContent = status.version || "—";
    if (!status.running) $("#ipfs-repo-size").textContent="—";
    const gateway=localGatewayResource(status.gateway || "", "/ipfs/bafy") ? status.gateway : "";
    $("#ipfs-gateway").textContent = status.running && gateway ? gateway : "—";
    const siteURL=site?.ipns ? localGatewayResource(gateway, `/ipns/${site.ipns}/`) : null;
    ipfsOpenSite.hidden = !status.running || !siteURL;
    if (!ipfsOpenSite.hidden && siteURL) ipfsOpenSite.href = siteURL;
    else ipfsOpenSite.removeAttribute("href");
    if (status.running && status.online) {
      $("#ipfs-peer-count").textContent = String(status.peers);
      $("#ipfs-rate").textContent = `${formatIPFSBytes(status.rateIn)}/s / ${formatIPFSBytes(status.rateOut)}/s`;
      $("#ipfs-total").textContent = `${formatIPFSBytes(status.totalIn)} / ${formatIPFSBytes(status.totalOut)}`;
      if (Number.isFinite(status.rateIn) && status.rateIn >= 0 && Number.isFinite(status.rateOut) && status.rateOut >= 0) ipfsSamples.push({time:Date.now(),input:status.rateIn,output:status.rateOut});
    }
    drawIPFSTraffic();
    if (includeRepo && status.running) {
      $("#ipfs-repo-size").textContent="正在计算…";
      try { const size=await Library.ipfsRepoSize(); if(revision===ipfsStatusRevision && ipfsDialog.open) $("#ipfs-repo-size").textContent=formatIPFSBytes(size); }
      catch { if(revision===ipfsStatusRevision && ipfsDialog.open) $("#ipfs-repo-size").textContent="暂不可用"; }
    }
  } catch (error) {
    if (revision === ipfsStatusRevision && ipfsDialog.open) { ipfsStatus=null; updateIPFSResource(); $("#ipfs-gateway").textContent="—"; ipfsOpenSite.hidden=true; ipfsOpenSite.removeAttribute("href"); $("#ipfs-status-message").textContent = `读取失败：${error instanceof Error ? error.message : String(error)}`; }
  } finally { if (revision === ipfsStatusRevision) { ipfsRefresh.disabled = false; ipfsToggle.disabled = !ipfsStatus; ipfsGC.disabled=!ipfsStatus?.running; if(ipfsDialog.open) ipfsTimer=setTimeout(()=>void refreshIPFSStatus(),5000); } }
}
$("#show-ipfs-status").addEventListener("click", () => { openDialog(ipfsDialog); void refreshIPFSStatus(true); });
ipfsRefresh.addEventListener("click", () => void refreshIPFSStatus(true));
ipfsGC.addEventListener("click",async()=>{
  if(ipfsGC.disabled || !ipfsStatus?.running)return;
  if(!window.confirm("清理会删除本机 Kubo 中未固定的数据，包括可能用于离线访问的内容；已固定内容会保留。继续吗？"))return;
  clearTimeout(ipfsTimer);ipfsRefresh.disabled=true;ipfsToggle.disabled=true;ipfsGC.disabled=true;$("#ipfs-status-message").textContent="正在清理未固定数据…";
  let failure="";
  try {await Library.ipfsGarbageCollect();} catch(error){failure=`清理失败：${error instanceof Error ? error.message : String(error)}`;}
  finally {ipfsRefresh.disabled=false;if(ipfsDialog.open){await refreshIPFSStatus(true);if(failure)$("#ipfs-status-message").textContent=failure;else $("#ipfs-status-message").textContent="未固定数据清理完成。";}}
});
ipfsToggle.addEventListener("click", async () => {
  if (!ipfsStatus || ipfsToggle.disabled) return;
  const enable = ipfsStatus.disabled || !ipfsStatus.running;
  if (!enable && !window.confirm("关闭节点会暂停站点发布、IPNS 续期及 IPFS 订阅更新，直到重新开启。继续吗？")) return;
  clearTimeout(ipfsTimer); ipfsToggle.disabled=true; ipfsRefresh.disabled=true; ipfsGC.disabled=true;
  $("#ipfs-status-message").textContent=enable ? "正在启动节点…" : "正在关闭节点…";
  let operationError = "";
  let pendingSites: Site[] = [];
  try {
    await Library.setIPFSEnabled(enable);
    if (!enable) { ipfsSamples=[]; drawIPFSTraffic(); }
    if (enable) { try { pendingSites=await Library.sites(); } catch(error) { showError(`节点已启动，但无法检查待发布站点：${error instanceof Error ? error.message : String(error)}`); } }
  } catch(error) { operationError=`操作失败：${error instanceof Error ? error.message : String(error)}`; }
  finally { ipfsRefresh.disabled=false; if(ipfsDialog.open) { await refreshIPFSStatus(true); if(operationError) $("#ipfs-status-message").textContent=operationError; } }
  for (const entry of pendingSites) if (entry.publishPending) void queuePublication(entry.id);
});
$("#close-ipfs-status").addEventListener("click", () => ipfsDialog.close());
ipfsDialog.addEventListener("close", () => { ipfsStatusRevision++; clearTimeout(ipfsTimer); ipfsRefresh.disabled = false; ipfsStatus=null; updateIPFSResource(); ipfsOpenSite.hidden=true; ipfsOpenSite.removeAttribute("href"); });
try { publicGateway.value = selectedGateway(window.localStorage.getItem(gatewayKey)); }
catch { publicGateway.value = "ipfs.io"; }
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
let libraryLoading = false, libraryFailed: string[] = [];
let refreshTimer: ReturnType<typeof setInterval> | undefined;
let editorTags=new Map<string,string>(), savedTagSignature="[]";
const xSync = initXSync({
  getSiteID: () => subscription || site?.imported ? null : site?.id ?? null,
  isBusy: () => saving,
  setBusy: busy => { if (busy) navigationRevision++; saving = busy; title.disabled = busy; content.disabled = busy; updateView(); },
});

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
tagInput.addEventListener("keydown",event=>{if(event.key==="Enter" && !event.isComposing && event.keyCode!==229){event.preventDefault();addEditorTag();}});
tagInput.addEventListener("input",()=>{dirty=true;navigationRevision++;updateView();});
function pendingTagBlocksSave(): boolean {
  if (!tagInput.value.trim()) return false;
  setMetadataOpen(true); showError("请先添加标签，或清空标签输入框后再保存。");tagInput.focus();return true;
}
let navigationRevision = 0, favoritesView = false, readerRevision = 0;
let smartView: "today" | "unread" | null = null;
const pendingRead = new Set<string>();
const pendingStars = new Set<string>();
const bulkReading = new Set<string>();
function isToday(value:string): boolean { const date=new Date(value); return Number.isFinite(date.getTime()) && date.toDateString()===new Date().toDateString(); }
const refreshingSubscriptions = new Set<string>();
let autoRefreshing = false;
let busyMessage = "";
let editingSiteID = "";
let editingTemplateSiteID = "";
let pinnableSiteID="", pinnableConfigured=false;
let telegramSiteID="", telegramRevision=0, telegramLoading=false, telegramBusy=false, telegramLoadFailed=false;
let telegramCurrent: {configured:boolean;chatId:string;chatTitle:string;syncedCount:number;pending:boolean}|null=null;
const sepiaFields: Array<[string,string,string]> = [
  ["backgroundColor","背景色","#F4F1EC"], ["foregroundColor","文字颜色","#000000"], ["foregroundSecondaryColor","次要文字","#999999"],
  ["highlightColor","强调色","#FF4500"], ["linkColor","链接颜色","#778087"], ["linkHoverColor","链接悬停","#4D5256"],
  ["siteTitleColor","站点标题","#333333"], ["siteTitleHoverColor","标题悬停","#000000"], ["borderColor","边框颜色","#E7E5DF"],
  ["navigationLinkColor","导航文字","#333333"], ["navigationActiveItemColor","导航选中","#FCFCFA"],
];
type PublicationState = { running: boolean; pending: boolean; status: "idle" | "published" | "failed"; error: string; changes: number; publishedChanges: number; generation: number };
const publicationStates = new Map<string, PublicationState>();
function isDashboard() { return !site && !subscription && !favoritesView && !smartView && !editing; }
const dashboard = createDashboard($("#dashboard"), {
  load: Library.dashboard, serviceInfo: Library.serviceInfo, stats: Library.plausibleStatsRange,
  openSite: id => openDashboardTarget(id),
  openEntry: (kind, id, entryID) => openDashboardTarget(id, kind, entryID),
  compose: id => openDashboardTarget(id, "compose"),
  search: () => $("#show-search").click(),
  createSite: () => $("#new-site").click(), importSite: () => importPlanet.click(),
  unread: () => $("#show-unread").click(), node: () => $("#show-ipfs-status").click(),
});
async function openDashboardTarget(siteID: string, kind?: "draft" | "article" | "compose", entryID = "") {
  if (!canLeave()) return;
  const revision = ++navigationRevision;
  try {
    let target = sites.find(entry => entry.id === siteID);
    if (!target) {
      const siteRevision = ++sitesRevision;
      const loaded = await Library.sites();
      if (revision !== navigationRevision || siteRevision !== sitesRevision) return;
      sites = loaded; target = sites.find(entry => entry.id === siteID);
    }
    if (!target) throw new Error("站点已移除");
    const [loadedArticles, loadedDrafts] = await Promise.all([Library.articles(siteID), kind === "draft" ? Library.drafts(siteID) : Promise.resolve([] as Draft[])]);
    if (revision !== navigationRevision) return;
    const selectedDraft = loadedDrafts.find(entry => entry.id === entryID), selectedArticle = loadedArticles.find(entry => entry.id === entryID);
    if (kind === "draft" && !selectedDraft || kind === "article" && !selectedArticle) throw new Error("内容已移除");
    site = target; articles = loadedArticles; drafts = loadedDrafts; article = null; draft = null;
    subscription = null; item = null; favoritesView = false; smartView = null; editing = false; dirty = false; draftsView = kind === "draft";
    clearError(); updateView();
    if (kind === "draft") { selectDraft(selectedDraft!); title.focus(); }
    else if (kind === "article") { selectArticle(selectedArticle!); workspaceTitle.tabIndex = -1; workspaceTitle.focus(); }
    else if (kind === "compose") startArticle();
    else { workspaceTitle.tabIndex = -1; workspaceTitle.focus(); }
  } catch (error) { if (revision === navigationRevision) throw error; }
}
$("#show-dashboard").addEventListener("click", () => {
  if (!canLeave()) return;
  const alreadyHome = isDashboard();
  navigationRevision++; readerRevision++; previewRevision++;
  site = null; subscription = null; item = null; article = null; draft = null; articles = []; drafts = [];
  draftsView = false; favoritesView = false; smartView = null; editing = false; dirty = false;
  clearError(); updateView(); if (alreadyHome) void dashboard.refresh(); $("#dashboard-title").focus();
});
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
  if (isDashboard()) void dashboard.refresh();
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
function articleRow(title: string, date: string, content: string, selected = false, source = ""): HTMLButtonElement {
  const button=document.createElement("button"); button.className=`article-item${selected ? " active" : ""}`;
  button.setAttribute("aria-pressed", String(selected));
  const heading=document.createElement("span"); heading.className="article-row-heading";
  if (source) { const icon=label("span",Array.from(source.trim())[0] || "P","article-source-icon"); icon.setAttribute("aria-hidden","true"); heading.append(icon,label("span",source,"article-source")); }
  heading.append(label("span",date,"article-date"));
  const excerpt=content.slice(0,2000).replace(/<[^>]*>/g," ").replace(/!?\[([^\]]*)\]\([^)]*\)/g,"$1").replace(/(^|\s)[#>*_`~-]+/g,"$1").replace(/\s+/g," ").trim();
  button.append(heading, label("span",title || "无标题","article-title"));
  if (excerpt) button.append(label("span",excerpt.slice(0,280),"article-excerpt"));
  return button;
}
function renderSites() {
  sitesEl.replaceChildren();
  for (const item of sites) {
    const button = document.createElement("button"); button.className = `site-item${item.id === site?.id ? " active" : ""}`;
    button.setAttribute("aria-pressed", String(item.id === site?.id));
    button.title = item.name;
    if(item.avatarPng){const avatar=document.createElement("img");avatar.className="site-icon site-avatar";avatar.alt="";avatar.src=`data:image/png;base64,${item.avatarPng}`;button.append(avatar);}else { const icon=label("span", Array.from(item.name.trim())[0] || "P", "site-icon"); icon.setAttribute("aria-hidden","true"); button.append(icon); }
    button.append(label("span", item.name, "site-name"));
    button.addEventListener("click", () => void selectSite(item)); sitesEl.append(button);
  }
}
function renderSubscriptions() {
  subscriptionsEl.replaceChildren();
  for (const entry of subscriptions) {
    const button = document.createElement("button"); button.className = `site-item${!favoritesView && !smartView && entry.url === subscription?.url ? " active" : ""}`;
    button.setAttribute("aria-pressed", String(!favoritesView && !smartView && entry.url === subscription?.url));
    const sourceName=entry.feed.title || entry.url;
    const icon=label("span", Array.from(sourceName.trim())[0] || "P", "site-icon"); icon.setAttribute("aria-hidden","true");
    button.append(icon, label("span", sourceName, "site-name"));
    const unread = entry.feed.items.filter(item => !item.read).length;
    if (unread) { const badge = label("span", String(unread), "subscription-count"); badge.setAttribute("aria-label", `${unread} 篇未读`); button.append(badge); }
    if (entry.lastError) { const warning=label("span", "!", "site-error"); warning.setAttribute("aria-label", "更新失败"); button.append(warning); }
    button.title = entry.lastError ? `更新失败：${entry.lastError}` : entry.pinnedCid ? "已固定到本机" : entry.url;
    button.addEventListener("click", () => selectSubscription(entry)); subscriptionsEl.append(button);
  }
}
function renderArticles() {
  if (smartView) {
    articlesEl.replaceChildren();
    const entries=subscriptions.flatMap(source=>source.feed.items.filter(entry=>smartView==="unread" ? !entry.read : isToday(entry.published)).map(entry=>({source,entry})));
    entries.sort((a,b)=>(Date.parse(b.entry.published)||0)-(Date.parse(a.entry.published)||0));
    count.textContent=String(entries.length);
    if(!entries.length) articlesEl.append(label("p",smartView==="unread" ? "没有未读文章" : "今天没有文章","list-empty"));
    for(const {source,entry} of entries) {
      const button=articleRow(entry.title,entry.published || "",entry.description || entry.content || "",source.url===subscription?.url && (entry.key || entry.id || entry.link)===(item?.key || item?.id || item?.link),source.feed.title || source.url);
      button.classList.toggle("unread",!entry.read);
      button.addEventListener("click",()=>{subscription=source;site=null;selectFeedItem(entry);});articlesEl.append(button);
    }
    return;
  }
  if (favoritesView) {
    articlesEl.replaceChildren(); const favorites = subscriptions.flatMap(source => source.feed.items.filter(entry => entry.starred).map(entry => ({source,entry})));
    count.textContent = String(favorites.length);
    if (!favorites.length) articlesEl.append(label("p", "还没有收藏的文章", "list-empty"));
    for (const favorite of favorites) { const button=articleRow(favorite.entry.title, favorite.entry.published || "", favorite.entry.description || favorite.entry.content || "", favorite.source.url===subscription?.url && favorite.entry===item, favorite.source.feed.title || favorite.source.url); button.addEventListener("click",()=>{subscription=favorite.source; site=null; selectFeedItem(favorite.entry);}); articlesEl.append(button); }
    return;
  }
  if (subscription) {
    articlesEl.replaceChildren(); count.textContent = String(subscription.feed.items.length);
    if (!subscription.feed.items.length) articlesEl.append(label("p", "订阅暂时没有文章", "list-empty"));
    for (const entry of subscription.feed.items) {
      const button = articleRow(entry.title, entry.published || "", entry.description || entry.content || "", entry === item, subscription.feed.title || subscription.url);
      button.classList.toggle("unread",!entry.read);
      button.addEventListener("click", () => selectFeedItem(entry)); articlesEl.append(button);
    }
    return;
  }
  if (draftsView) {
    articlesEl.replaceChildren(); count.textContent=String(drafts.length);
    if (!drafts.length) articlesEl.append(label("p","还没有草稿","list-empty"));
    for (const entry of drafts) {
      const button=articleRow(entry.title || "未命名草稿",new Date(entry.modified).toLocaleDateString("zh-CN"),entry.content,entry.id===draft?.id,site?.name || "草稿");
      button.addEventListener("click",()=>selectDraft(entry)); articlesEl.append(button);
    }
    return;
  }
  articlesEl.replaceChildren(); count.textContent = String(articles.length);
  if (!articles.length) { articlesEl.append(label("p", site ? "还没有文章" : "选择一个站点开始", "list-empty")); return; }
  for (const item of [...articles].sort((a, b) => b.modified.localeCompare(a.modified))) {
    const button = articleRow(item.title, `${item.articleType===1 ? "页面  " : ""}${new Date(item.modified).toLocaleDateString("zh-CN")}`, item.content, item.id === article?.id, site?.name || "我的站点");
    button.addEventListener("click", () => selectArticle(item)); articlesEl.append(button);
  }
}
let previousMode = "browsing";
function layoutMode() { return isDashboard() ? "dashboard" : editing ? "editing" : article || item ? "reading" : "browsing"; }
window.addEventListener("resize", () => {
  if (editing) syncEditorView();
  else setPostsCollapsed(layoutMode() === "reading" && window.innerWidth < 1100);
});
function updateView() {
  const mode = layoutMode();
  const overview = mode === "dashboard";
  shell.classList.toggle("dashboard-mode", overview);
  workspace.hidden = overview;
  skipToMain.setAttribute("href", overview ? "#dashboard" : "#workspace-main");
  dashboard.setActive(overview);
  dashboard.setPublicationStatus(publicationStates);
  $("#show-dashboard").classList.toggle("active", overview);
  $("#show-dashboard").setAttribute("aria-pressed", String(overview));
  if (mode !== previousMode) {
    setPostsCollapsed(editing || mode === "reading" && window.innerWidth < 1100);
    setMetadataOpen(false);
    if (editing) editorView = "write";
    previousMode = mode;
  }
  shell.classList.toggle("editing-mode", editing);
  shell.classList.toggle("local-site", !!site && !subscription && !smartView && !favoritesView);
  syncEditorView();
  const publication = site ? publicationStates.get(site.id) : undefined;
  renderSites(); renderSubscriptions(); renderArticles(); siteTitle.textContent = smartView === "today" ? "今天" : smartView === "unread" ? "未读文章" : favoritesView ? "收藏的文章" : subscription?.feed.title || site?.name || "我的站点";
  $("#show-favorites").classList.toggle("active",favoritesView);
  $("#show-favorites").setAttribute("aria-pressed",String(favoritesView));
  for(const [id,view] of [["#show-today","today"],["#show-unread","unread"]] as const){$(id).classList.toggle("active",smartView===view);$(id).setAttribute("aria-pressed",String(smartView===view));}
  const allItems=subscriptions.flatMap(sub=>sub.feed.items);
  $("#today-count").textContent=String(allItems.filter(entry=>isToday(entry.published)).length);
  $("#unread-count").textContent=String(allItems.filter(entry=>!entry.read).length);
  $("#favorites-count").textContent=String(allItems.filter(entry=>entry.starred).length);
  workspaceMenu.hidden = !site && !subscription;
  newArticle.disabled = !site || !!subscription || saving;
  const headerCompose=$<HTMLButtonElement>("#new-article-header"); headerCompose.hidden=!site || !!subscription || editing; headerCompose.disabled=newArticle.disabled;
  saveButton.disabled = !site || !editing || !!subscription || saving;
  showDrafts.hidden=!!subscription || favoritesView || !!smartView; showDrafts.disabled=!site || saving; showDrafts.textContent=draftsView ? "返回文章" : "草稿";
  $("#posts-kind").textContent = smartView === "unread" ? "未读" : smartView === "today" ? "今天" : favoritesView ? "收藏" : draftsView ? "草稿" : "文章";
  saveDraft.hidden=!editing || !!subscription; saveDraft.disabled=!site || !editing || saving;
  saveButton.hidden = !!subscription || !editing; refreshFeed.hidden = !subscription;
  unfollow.hidden = !subscription; unfollow.disabled = !subscription || saving;
  refreshFeed.disabled = !!subscription && refreshingSubscriptions.has(subscription.url); refreshFeed.textContent = refreshFeed.disabled ? "正在刷新…" : "刷新订阅";
  exportSite.hidden = !!subscription; exportSite.disabled = !site || saving;
  publishSite.hidden = !!subscription || !site; publishSite.disabled = !site || saving || !!publication?.running;
  exportPlanet.hidden = !!subscription; exportPlanet.disabled = !site || saving; importPlanet.disabled = saving;
  selectTemplate.hidden = !!subscription; selectTemplate.disabled = !site || saving;
  templateSettingsButton.hidden = !!subscription; templateSettingsButton.disabled = !site || site.templateName !== "Sepia" || saving;
  pinnableButton.hidden=!!subscription; pinnableButton.disabled=!site || saving;
  telegramButton.hidden=!!subscription; telegramButton.disabled=!site || saving;
  if(telegramDialog.open && site?.id!==telegramSiteID)telegramDialog.close();
  updateTelegramControls();
  serviceButton.hidden=!!subscription; serviceButton.disabled=!site || saving;
  xSync.update();
  editSite.hidden = !!subscription; editSite.disabled = !site || saving;
  const profileButton=$<HTMLButtonElement>("#site-profile");profileButton.hidden=!!subscription;profileButton.disabled=!site || !!site.imported || saving;profileButton.title=site?.imported ? "导入站点暂不支持修改公开资料" : "";
  const podcastButton=$<HTMLButtonElement>("#podcast-settings");podcastButton.hidden=!!subscription;podcastButton.disabled=!site || !!site.imported || saving;
  const deploymentButton=$<HTMLButtonElement>("#deployment-settings");deploymentButton.hidden=!!subscription;deploymentButton.disabled=!site || !!site.imported || saving;
  removeSite.hidden = !!subscription; removeSite.disabled = !site || saving || !!publication?.running;
  const fixed = !!subscription && (subscription.cid ? subscription.pinnedCid === subscription.cid : !!subscription.offlinePinned);
  pinSubscription.hidden = !subscription; pinSubscription.disabled = saving || !!subscription && ((fixed && !!subscription.cid) || refreshingSubscriptions.has(subscription.url));
  pinSubscription.textContent = fixed ? (subscription?.cid ? "已固定离线" : "重新固定离线") : "固定离线";
  $("#subscription-status").textContent = subscription && refreshingSubscriptions.has(subscription.url) ? "正在刷新订阅…" : subscription?.lastError ? `更新失败，正在显示已缓存内容：${subscription.lastError}` : subscription?.offlinePinned ? "已固定订阅媒体到本机" : subscription?.pinnedCid ? (subscription.pinnedCid === subscription.cid ? "已固定当前版本到本机" : "仅旧版本已固定") : "";
  const favoriteKey=subscription && item ? `${subscription.url}\u0000${item.key || item.id || item.link}` : "";
  starItem.textContent = item?.starred ? "取消收藏" : "收藏"; starItem.title=starItem.textContent; starItem.setAttribute("aria-label",starItem.textContent); starItem.disabled = !item || !subscription || pendingStars.has(favoriteKey); shareItem.disabled = !item?.link;
  const readButton=$<HTMLButtonElement>("#toggle-read"), readKey=subscription && item ? `${subscription.url}\u0000${item.key || item.id || item.link}` : "";
  readButton.textContent=item?.read ? "标为未读" : "标为已读";readButton.title=readButton.textContent;readButton.setAttribute("aria-label",readButton.textContent);readButton.disabled=!item || !subscription || pendingRead.has(readKey) || bulkReading.has(subscription.url);
  const markFeed=$<HTMLButtonElement>("#mark-feed-read");markFeed.hidden=!subscription;markFeed.disabled=!subscription || bulkReading.has(subscription.url) || [...pendingRead].some(key=>key.startsWith(`${subscription!.url}\u0000`)) || !subscription.feed.items.some(entry=>!entry.read);
  deleteArticle.hidden = (!article && !draft) || !!subscription; deleteArticle.textContent=draft ? "删除草稿" : "删除文章";
  addAttachment.disabled = (!article && !draft) || !site || saving;
	 tagInput.disabled = !editing || saving || !!site?.tagsUnsupported; addTag.disabled = tagInput.disabled;
	 tagList.replaceChildren();
	 for (const [key,value] of editorTags) {
	   const item=label("span",value,"tag-item"), remove=document.createElement("button");
	   remove.type="button";remove.className="remove-tag";remove.textContent="×";remove.setAttribute("aria-label",`删除标签 ${value}`);remove.disabled=tagInput.disabled;
	   remove.addEventListener("click",()=>{if(saving || site?.tagsUnsupported)return;editorTags.delete(key);dirty=true;navigationRevision++;updateView();});item.append(remove);tagList.append(item);
	 }
  playArticleAudio.hidden = !article?.audioFilename || !site; playArticleAudio.disabled = saving;
  const ownAudio=$<HTMLButtonElement>("#play-own-audio");ownAudio.hidden=!article?.audioFilename || !site;ownAudio.disabled=saving;
  const articleSettings=$<HTMLButtonElement>("#article-settings");articleSettings.disabled=!article || !site || !!site.imported || saving;articleSettings.title=site?.imported ? "导入站点的文章设置暂不可修改" : "文章路径、类型与导航设置";
  $<HTMLButtonElement>("#share-own-article").disabled=!site?.ipns || !site.lastPublishedCID || !article;
  const ownShareInput=$<HTMLInputElement>("#own-share-url");
  let currentOwnShare="";
  if(site?.ipns && site.lastPublishedCID && article){try{currentOwnShare=sitePublicURL(selectedGateway(publicGateway.value),site.ipns,site.domain,`/${encodeURIComponent(article.slug || article.id)}/`);}catch{ /* The copy action reports unsupported gateway names. */ }}
  if(ownShareInput.value && ownShareInput.value!==currentOwnShare){ownShareInput.value="";ownShareInput.hidden=true;$("#own-share-status").textContent="";}
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
  const ownReading=!!site && !!article && !editing && !subscription && !draftsView;
  empty.hidden = editing || !!item || ownReading; editor.hidden = !editing; reader.hidden = !item; ownReader.hidden=!ownReading;
  if(ownReading && article){$("#own-reader-title").textContent=article.title || "无标题";$("#own-reader-date").textContent=new Date(article.modified).toLocaleDateString("zh-CN");$("#own-reader-source").textContent=site?.name || "我的站点";$("#own-reader-author").textContent=site?.authorName || site?.name || "我的站点";}
  if(subscription){$("#reader-source").textContent=subscription.feed.title || subscription.url;$("#reader-author").textContent=subscription.feed.title || subscription.url;}
  for (const container of [ownReader, reader]) {
    const source=container.querySelector<HTMLElement>(".reader-category > span:last-child")?.textContent || "P";
    const initial=Array.from(source.trim())[0] || "P";
    container.querySelectorAll<HTMLElement>(".reader-source-icon, .reader-author-avatar").forEach(icon=>{icon.textContent=initial;});
  }
  workspaceTitle.textContent = subscription ? "阅读订阅" : smartView === "today" ? "今天" : smartView === "unread" ? "未读文章" : favoritesView ? "收藏的文章" : editing ? "撰写文章" : ownReading ? "阅读文章" : site?.name ?? "欢迎来到 Planet";
  siteAddress.hidden = !site?.ipns || !!subscription; siteAddress.textContent = site?.ipns ? `IPNS: ${site.ipns}` : ""; siteAddress.title = site?.ipns ?? "";
  publishedAddress.hidden = !site?.ipns || !site?.lastPublishedCID || !!subscription;
  let address = "", gatewayError = "";
  if (!publishedAddress.hidden) {
    try { address = sitePublicURL(selectedGateway(publicGateway.value), site!.ipns!,site!.domain); }
    catch (error) { gatewayError = error instanceof Error ? error.message : String(error); }
  }
  if (publishedURL.value !== address) { publishedURL.value = address; copyStatus.textContent = ""; }
  copyPublishedURL.disabled = !address;
  if (gatewayError) copyStatus.textContent = gatewayError;
  $("#show-publication").hidden = !site || !!subscription;
  $("#publication-status").textContent = publishedAddress.hidden ? "站点尚未公开发布。发布完成后，可在这里复制访问地址。" : "站点已公开发布。网络传播可能需要时间。";
  emptyAction.textContent = site ? "撰写文章" : "创建站点";
  emptyAction.hidden = !!subscription || favoritesView || !!smartView;
  empty.querySelector("h2")!.textContent = smartView ? "选择一篇文章阅读" : favoritesView ? "选择一篇收藏文章" : subscription ? "选择一篇文章阅读" : site && articles.length ? "选择一篇文章" : site ? "开始记录你的故事" : "让想法有一个地方安放";
  empty.querySelector("p")!.textContent = smartView || favoritesView || subscription ? "从左侧列表选择文章。" : site && articles.length ? "从左侧列表选择文章，或新建一篇。" : site ? "新建文章，在这里写作与预览。" : "创建站点，开始撰写你的第一篇文章。";
  const current = dirty ? "有未保存更改" : draft ? "草稿已保存（未发布）" : editing ? article ? "已保存" : "新文章" : "";
  const unpublished = !!site && ((site.contentRevision ?? 0) > (site.publishedRevision ?? 0) || !!site.publishPending || !!publication && publication.changes > publication.publishedChanges);
  saveState.textContent = saving ? busyMessage || "正在处理…" : publication?.running ? `${current}${current ? "，" : ""}正在发布已保存内容…` : publication?.status === "failed" ? `${current}${current ? "，" : ""}发布未确认` : unpublished ? `${current}${current ? "，" : ""}站点更改尚未发布` : publication?.status === "published" ? `${current}${current ? "，" : ""}${dirty ? "已发布上次保存的内容" : "已发布"}` : current;
  saveState.title = publication?.error || saveState.textContent;
  const attachmentCount = attachmentList.childElementCount;
  $("#metadata-count").textContent = editorTags.size || attachmentCount ? `${editorTags.size} 个标签 / ${attachmentCount} 个附件` : "";
  const notice = $("#library-notice"), retry = $<HTMLButtonElement>("#retry-library");
  if (overview) $(".dashboard-header").after(notice, errorEl);
  else { $(".workspace-header").after(notice); workspace.append(errorEl); }
  notice.hidden = !libraryLoading && libraryFailed.length === 0;
  notice.dataset.state = libraryLoading ? "loading" : "error";
  $("#library-message").textContent = libraryLoading ? "正在读取本地内容…" : `${libraryFailed.join("、")}未能加载。已读取的内容仍可使用。`;
  retry.hidden = libraryLoading; retry.disabled = libraryLoading;
  if (!site && !subscription && !item && !editing && !favoritesView && !smartView && (libraryLoading || libraryFailed.includes("站点"))) {
    empty.querySelector("h2")!.textContent = libraryLoading ? "正在打开工作区" : "暂时无法读取站点";
    empty.querySelector("p")!.textContent = libraryLoading ? "你的站点和订阅正在加载。" : "请重新加载；已有内容不会被更改。";
    emptyAction.hidden = true;
  }
  syncMetadataLayout();
}
async function selectSite(item: Site) {
  if (item.id === site?.id && !articleLoadFailures.has(item.id)) return;
  if (!canLeave()) return;
  const revision = ++navigationRevision;
  try {
    const loaded = await Library.articles(item.id);
    if (revision !== navigationRevision) return;
    articleLoadFailures.delete(item.id); site = sites.find(entry => entry.id === item.id) ?? item; favoritesView = false; smartView=null; subscription = null; windowItemReset(); articles = loaded; article = null; draft=null; drafts=[]; draftsView=false; editing = false; dirty = false; clearError(); updateView();
    const failure = publicationStates.get(item.id)?.error; if (failure) showError(failure);
  } catch (e) { if (revision===navigationRevision) { articleLoadFailures.add(item.id); showError(e); } }
}
function windowItemReset() { item = null; }
function selectSubscription(entry: Subscription) {
  if (!canLeave()) return;
  navigationRevision++;
  site = null; favoritesView = false; smartView=null; subscription = entry; item = null; article = null; draft=null; draftsView=false; editing = false; dirty = false; clearError(); updateView();
}
function readerHTML(html: string, localImages: boolean, remoteBase = "") {
  readerPreview.srcdoc = feedReaderDocument(html, localImages, remoteBase);
}
function selectFeedItem(entry: Item) {
  const revision=++readerRevision, address=subscription?.url ?? "", key=entry.key || entry.id || entry.link;
  item = entry; shareURL.hidden = true; shareStatus.textContent = ""; readerTitle.textContent = entry.title || "无标题"; readerDate.textContent = entry.published;
  $("#play-feed-audio").hidden = !entry.audioUrl && !(entry.audioFilename && entry.contentCid);
  $("#play-feed-video").hidden = !entry.videoUrl;
  const localPreview = !!(entry.contentCid || entry.offlineBase || subscription?.offlinePinned);
  const remoteBase = localPreview ? "" : feedArticleBase(entry.link, subscription?.feed.link ?? "", address);
  readerHTML(entry.content || entry.description,false,remoteBase);
  if((entry.contentCid || entry.offlineBase) && address && key) {
    void Library.subscriptionPreview(address,key).then(html=>{
      if(revision===readerRevision && subscription?.url===address && (item?.key || item?.id || item?.link)===key) readerHTML(html,true);
    }).catch(()=>{});
  }
  updateView();
  if (!entry.read && address && key) void setSubscriptionRead(address,key,true);
}
async function setSubscriptionRead(address:string,key:string,value:boolean) {
  const operation=`${address}\u0000${key}`;
  if (!key || pendingRead.has(operation) || bulkReading.has(address)) return;
  pendingRead.add(operation);updateView();
  try {
    await Library.setSubscriptionRead(address,key,value);
    subscriptionsRevision++;
    const current=subscriptions.find(entry=>entry.url===address)?.feed.items.find(entry=>(entry.key || entry.id || entry.link)===key);
    if(current) current.read=value;
    if(subscription?.url===address && item && (item.key || item.id || item.link)===key) item.read=value;
    if (isDashboard()) void dashboard.refresh();
  } catch(error) { showError(`保存阅读状态失败：${error instanceof Error ? error.message : String(error)}`); }
  finally {pendingRead.delete(operation);updateView();}
}
function selectArticle(item: Article) {
  if (item.id === article?.id && !editing) return;
  if (!canLeave()) return;
  navigationRevision++;
  article = item; draft=null; draftsView=false; editing = false; dirty = false; title.value = item.title; content.value = item.content; setEditorTags(item.tags); clearError(); updateView(); void renderOwnArticle();
}
$("#edit-article").addEventListener("click",()=>{if(!site || !article || saving)return;navigationRevision++;editing=true;dirty=false;title.value=article.title;content.value=article.content;setEditorTags(article.tags);clearError();updateView();void renderPreview();});
const articleSettingsDialog=$<HTMLDialogElement>("#article-settings-dialog"),articleSettingsForm=$<HTMLFormElement>("#article-settings-form");
$("#article-settings").addEventListener("click",()=>{
  if(!site || site.imported || !article || saving)return;
  articleSettingsForm.querySelector<HTMLInputElement>('[name="slug"]')!.value=article.slug || "";
  articleSettingsForm.querySelector<HTMLSelectElement>('[name="articleType"]')!.value=String(article.articleType || 0);
  articleSettingsForm.querySelector<HTMLInputElement>('[name="externalLink"]')!.value=article.externalLink || "";
  articleSettingsForm.querySelector<HTMLInputElement>('[name="inNavigation"]')!.checked=!!article.isIncludedInNavigation;
  articleSettingsForm.querySelector<HTMLInputElement>('[name="weight"]')!.value=String(article.navigationWeight ?? 1);
  $("#article-settings-result").textContent="";openDialog(articleSettingsDialog);
});
$("#close-article-settings").addEventListener("click",()=>{if(!saving)articleSettingsDialog.close();});
articleSettingsDialog.addEventListener("cancel",event=>{if(saving)event.preventDefault();});
articleSettingsForm.addEventListener("submit",async event=>{
  event.preventDefault();if(!site || site.imported || !article || saving)return;
  const siteID=site.id,articleID=article.id,slug=articleSettingsForm.querySelector<HTMLInputElement>('[name="slug"]')!.value.trim();
  const type=Number(articleSettingsForm.querySelector<HTMLSelectElement>('[name="articleType"]')!.value),external=articleSettingsForm.querySelector<HTMLInputElement>('[name="externalLink"]')!.value.trim();
  const included=articleSettingsForm.querySelector<HTMLInputElement>('[name="inNavigation"]')!.checked,weight=Number(articleSettingsForm.querySelector<HTMLInputElement>('[name="weight"]')!.value);
  if(!Number.isSafeInteger(weight) || weight < -2147483648 || weight > 2147483647){$("#article-settings-result").textContent="请输入有效的整数权重。";return;}
  const controls=Array.from(articleSettingsForm.querySelectorAll<HTMLInputElement|HTMLSelectElement|HTMLButtonElement>("input,select,button"));controls.forEach(control=>control.disabled=true);saving=true;updateView();
  try {
    const saved=await Library.updateArticleSettings(siteID,articleID,slug,type,external,included,weight);
    articles=articles.map(entry=>entry.id===articleID ? saved : entry);if(site?.id===siteID && article?.id===articleID)article=saved;
    markLocalChange(siteID);articleSettingsDialog.close();clearError();void queuePublication(siteID);
  } catch(error){$("#article-settings-result").textContent=`保存失败：${error instanceof Error ? error.message : String(error)}`;}
  finally{controls.forEach(control=>control.disabled=false);saving=false;updateView();}
});
$("#share-own-article").addEventListener("click",async()=>{
  if(!site?.ipns || !site.lastPublishedCID || !article)return;
  const shareInput=$<HTMLInputElement>("#own-share-url"), status=$("#own-share-status");
  try {
    shareInput.value=sitePublicURL(selectedGateway(publicGateway.value),site.ipns,site.domain,`/${encodeURIComponent(article.slug || article.id)}/`);
    const publication=publicationStates.get(site.id);
    const unpublished=(site.contentRevision ?? 0)>(site.publishedRevision ?? 0) || site.publishPending || !!publication && (publication.running || publication.changes>publication.publishedChanges);
    const warning=unpublished ? "；站点尚有更改未发布，新文章发布后才可访问" : "";
    try {await window.navigator.clipboard.writeText(shareInput.value);shareInput.hidden=true;status.textContent=`链接已复制${warning}`;}
    catch {shareInput.hidden=false;shareInput.focus();shareInput.select();status.textContent=`请按 Ctrl+C 复制${warning}`;}
  } catch(error){showError(error);}
});
$("#play-own-audio").addEventListener("click",()=>playArticleAudio.click());
function selectDraft(item: Draft) {
  if (!canLeave()) return;
  editorView="write"; setMetadataOpen(false);
  navigationRevision++; draft=item; article=articles.find(entry=>entry.id===item.articleId) ?? null;
  editing=true; dirty=false; title.value=item.title; content.value=item.content; setEditorTags(item.tags); clearError(); updateView(); void renderPreview();
}
function startArticle() {
  if (!site || !canLeave()) return;
  editorView="write"; setMetadataOpen(false);
  navigationRevision++;
  article = null; draft=null; draftsView=false; editing = true; dirty = false; title.value = ""; content.value = ""; setEditorTags(); clearError(); updateView(); void renderPreview(); title.focus();
}
async function renderPreview() {
  const revision = ++previewRevision;
  try {
    const html = site && draft ? await Library.previewDraft(site.id, draft.id, content.value) : site && article ? await Library.previewArticle(site.id, article.id, content.value) : await Library.preview(content.value);
    if (revision !== previewRevision) return;
    const safeTitle=title.value.replace(/[&<>]/g, char => ({"&":"&amp;","<":"&lt;",">":"&gt;"}[char]!));
    preview.srcdoc = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'"><style>${readingStyles}body{max-width:none;padding:12px 0 24px}.preview-title{font-size:24px;margin:0 0 20px}</style></head><body>${safeTitle ? `<h1 class="preview-title">${safeTitle}</h1>` : ""}${html}</body></html>`;
  } catch (e) { showError(e); }
}
async function renderOwnArticle() {
  if(!site || !article)return;
  const revision=++previewRevision,siteID=site.id,articleID=article.id;
  try {
    const html=await Library.previewArticle(siteID,articleID,article.content);
    if(revision!==previewRevision || site?.id!==siteID || article?.id!==articleID || editing)return;
    ownReaderPreview.srcdoc=`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'"><style>${readingStyles}</style></head><body>${html}</body></html>`;
  } catch(error){if(revision===previewRevision)showError(error);}
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
        const warning = [result.pinnableWarning, result.telegramWarning].filter(Boolean).join("\n\n");
        state.status = "published"; state.error = warning;
        if (warning && site?.id===siteID) showError(warning);
        if (manual && !state.pending) window.alert(`发布请求已完成。\n\nIPNS：${result.ipns}\nCID：${result.cid}\n\n网络传播可能需要时间；请保持应用运行以提供内容。${warning ? `\n\n${warning}` : ""}`);
      } catch (e) {
        state.status = "failed";
        state.error = `文章已保存，但站点发布未确认：${e instanceof Error ? e.message : String(e)}。可点击“发布到 IPFS”重试。`;
        if (site?.id === siteID && !state.pending) showError(state.error);
      }
    } while (state.pending);
  } finally { state.running = false; updateView(); if (isDashboard()) void dashboard.refresh(); }
}
$("#new-site").addEventListener("click", () => { if (canLeave()) { navigationRevision++; editingSiteID=""; form.reset(); $("#site-index-setting").hidden=true;$("#site-avatar-setting").hidden=true;$("#site-identity-settings").hidden=true;$("#site-avatar-result").textContent="";$("#site-settings-result").textContent=""; form.querySelector("h2")!.textContent="创建站点";$("#site-dialog-description").textContent="为你的新空间起个名字。"; form.querySelector<HTMLButtonElement>('button[type="submit"]')!.textContent="创建站点"; openDialog(dialog); } });
function updateSiteAvatarPreview(value?:string) {
  const image=$<HTMLImageElement>("#site-avatar-preview");image.hidden=!value;$("#site-avatar-placeholder").hidden=!!value;
  if(value)image.src=`data:image/png;base64,${value}`;else image.removeAttribute("src");
  $<HTMLButtonElement>("#remove-site-avatar").disabled=!value || saving;
}
editSite.addEventListener("click", () => {
  if (!site || saving) return;
  editingSiteID=site.id; form.reset();$("#site-settings-result").textContent="";
  form.querySelector<HTMLInputElement>('input[name="name"]')!.value=site.name;
  form.querySelector<HTMLTextAreaElement>('textarea[name="about"]')!.value=site.about;
  form.querySelector<HTMLInputElement>('input[name="doNotIndex"]')!.checked=!!site.doNotIndex;
  form.querySelector<HTMLInputElement>('input[name="siteSlug"]')!.value=site.slug || "";
  form.querySelector<HTMLInputElement>('input[name="authorName"]')!.value=site.authorName || "";
  $("#site-index-setting").hidden=!!site.imported;
  $("#site-identity-settings").hidden=!!site.imported;
  $("#site-avatar-setting").hidden=!!site.imported;$("#site-avatar-result").textContent="";updateSiteAvatarPreview(site.avatarPng);
  form.querySelector("h2")!.textContent="站点设置";$("#site-dialog-description").textContent="站点信息保存后需重新发布；头像选择后会自动发布。"; form.querySelector<HTMLButtonElement>('button[type="submit"]')!.textContent="保存设置";
  openDialog(dialog);
});
async function changeSiteAvatar(remove:boolean) {
  if(!editingSiteID || !site || site.id!==editingSiteID || site.imported || saving)return;
  const siteID=editingSiteID,generation=publicationState(siteID).generation;
  const controls=Array.from(form.querySelectorAll<HTMLInputElement|HTMLTextAreaElement|HTMLButtonElement>("input,textarea,button"));
  const disabled=controls.map(control=>control.disabled);controls.forEach(control=>control.disabled=true);saving=true;updateView();$("#site-avatar-result").textContent="";
  try {
    const updated=remove ? await Library.removeSiteAvatar(siteID) : await Library.selectSiteAvatar(siteID);
    if(!updated.id)return;
    const selected=mergeSitePublication(sites.find(entry=>entry.id===siteID),updated,generation);
    sitesRevision++;sites=sites.map(entry=>entry.id===siteID ? selected : entry);if(site?.id===siteID)site=selected;
    updateSiteAvatarPreview(selected.avatarPng);markLocalChange(siteID);void queuePublication(siteID);
    $("#site-avatar-result").textContent="头像已保存，正在发布站点。";
  } catch(error){$("#site-avatar-result").textContent=`头像保存失败：${error instanceof Error ? error.message : String(error)}`;}
  finally{controls.forEach((control,i)=>control.disabled=disabled[i]!);saving=false;updateSiteAvatarPreview(site?.id===siteID ? site.avatarPng : undefined);updateView();}
}
$("#choose-site-avatar").addEventListener("click",()=>void changeSiteAvatar(false));
$("#remove-site-avatar").addEventListener("click",()=>void changeSiteAvatar(true));
const siteProfileDialog=$<HTMLDialogElement>("#site-profile-dialog"),siteProfileForm=$<HTMLFormElement>("#site-profile-form");
const profileFields=["domain","twitterUsername","githubUsername","telegramUsername","mastodonUsername","discordLink"] as const;
$("#site-profile").addEventListener("click",()=>{
  if(!site || site.imported || saving)return;
  for(const name of profileFields)siteProfileForm.querySelector<HTMLInputElement>(`[name="${name}"]`)!.value=site[name] || "";
  $("#site-profile-result").textContent="";openDialog(siteProfileDialog);
});
$("#close-site-profile").addEventListener("click",()=>{if(!saving)siteProfileDialog.close();});
siteProfileDialog.addEventListener("cancel",event=>{if(saving)event.preventDefault();});
siteProfileForm.addEventListener("submit",async event=>{
  event.preventDefault();if(!site || site.imported || saving)return;
  const siteID=site.id,generation=publicationState(siteID).generation,profile=Object.fromEntries(profileFields.map(name=>[name,siteProfileForm.querySelector<HTMLInputElement>(`[name="${name}"]`)!.value.trim()]));
  const controls=Array.from(siteProfileForm.querySelectorAll<HTMLInputElement|HTMLButtonElement>("input,button"));controls.forEach(control=>control.disabled=true);saving=true;updateView();
  try {const updated=await Library.updateSiteProfile(siteID,JSON.stringify(profile));const selected=mergeSitePublication(sites.find(entry=>entry.id===siteID),updated,generation);sitesRevision++;sites=sites.map(entry=>entry.id===siteID?selected:entry);if(site?.id===siteID)site=selected;siteProfileDialog.close();markLocalChange(siteID);void queuePublication(siteID);}
  catch(error){$("#site-profile-result").textContent=`保存失败：${error instanceof Error ? error.message : String(error)}`;}
  finally{controls.forEach(control=>control.disabled=false);saving=false;updateView();}
});
const podcastDialog=$<HTMLDialogElement>("#podcast-settings-dialog"),podcastForm=$<HTMLFormElement>("#podcast-settings-form");
const podcastCategories=["Arts","Business","Comedy","Education","Fiction","Government","History","Health & Fitness","Kids & Family","Leisure","Music","News","Religion & Spirituality","Science","Society & Culture","Sports","Technology","True Crime","TV & Film"];
for(const category of podcastCategories){const item=document.createElement("label"),input=document.createElement("input");input.type="checkbox";input.value=category;input.name="category";item.append(input,document.createTextNode(category));$("#podcast-categories").append(item);}
function updatePodcastCover(value?:string){const preview=$<HTMLImageElement>("#podcast-cover-preview");preview.hidden=!value;$("#podcast-cover-placeholder").hidden=!!value;if(value)preview.src=`data:image/png;base64,${value}`;else preview.removeAttribute("src");$<HTMLButtonElement>("#remove-podcast-cover").disabled=!value || saving;}
$("#podcast-settings").addEventListener("click",()=>{if(!site || site.imported || saving)return;podcastForm.querySelector<HTMLInputElement>('[name="language"]')!.value=site.podcastLanguage || "en";podcastForm.querySelector<HTMLInputElement>('[name="explicit"]')!.checked=!!site.podcastExplicit;for(const input of podcastForm.querySelectorAll<HTMLInputElement>('[name="category"]'))input.checked=!!site.podcastCategories?.includes(input.value);updatePodcastCover(site.podcastCoverPng);$("#podcast-settings-result").textContent="";$("#podcast-cover-result").textContent="";openDialog(podcastDialog);});
$("#close-podcast-settings").addEventListener("click",()=>{if(!saving)podcastDialog.close();});podcastDialog.addEventListener("cancel",event=>{if(saving)event.preventDefault();});
podcastForm.addEventListener("submit",async event=>{
  event.preventDefault();if(!site || site.imported || saving)return;const siteID=site.id,generation=publicationState(siteID).generation;
  const language=podcastForm.querySelector<HTMLInputElement>('[name="language"]')!.value.trim(),explicit=podcastForm.querySelector<HTMLInputElement>('[name="explicit"]')!.checked,categories=Array.from(podcastForm.querySelectorAll<HTMLInputElement>('[name="category"]:checked')).map(input=>input.value);
  const controls=Array.from(podcastForm.querySelectorAll<HTMLInputElement|HTMLButtonElement>("input,button"));controls.forEach(control=>control.disabled=true);saving=true;updateView();
  try{const updated=await Library.updatePodcastSettings(siteID,language,explicit,JSON.stringify(categories));const selected=mergeSitePublication(sites.find(entry=>entry.id===siteID),updated,generation);sitesRevision++;sites=sites.map(entry=>entry.id===siteID?selected:entry);if(site?.id===siteID)site=selected;podcastDialog.close();markLocalChange(siteID);void queuePublication(siteID);}
  catch(error){$("#podcast-settings-result").textContent=`保存失败：${error instanceof Error ? error.message : String(error)}`;}
  finally{controls.forEach(control=>control.disabled=false);saving=false;updateView();}
});
async function changePodcastCover(remove:boolean){
  if(!site || site.imported || saving)return;const siteID=site.id,generation=publicationState(siteID).generation;
  const controls=Array.from(podcastForm.querySelectorAll<HTMLInputElement|HTMLButtonElement>("input,button")),disabled=controls.map(control=>control.disabled);controls.forEach(control=>control.disabled=true);saving=true;updateView();
  try{const updated=remove?await Library.removePodcastCover(siteID):await Library.selectPodcastCover(siteID);if(!updated.id)return;const selected=mergeSitePublication(sites.find(entry=>entry.id===siteID),updated,generation);sitesRevision++;sites=sites.map(entry=>entry.id===siteID?selected:entry);if(site?.id===siteID)site=selected;updatePodcastCover(selected.podcastCoverPng);markLocalChange(siteID);void queuePublication(siteID);$("#podcast-cover-result").textContent="封面已保存，正在发布站点。";}
  catch(error){$("#podcast-cover-result").textContent=`封面保存失败：${error instanceof Error ? error.message : String(error)}`;}
  finally{controls.forEach((control,i)=>control.disabled=disabled[i]!);saving=false;updatePodcastCover(site?.id===siteID?site.podcastCoverPng:undefined);updateView();}
}
$("#choose-podcast-cover").addEventListener("click",()=>void changePodcastCover(false));$("#remove-podcast-cover").addEventListener("click",()=>void changePodcastCover(true));
const deploymentDialog=$<HTMLDialogElement>("#deployment-dialog"),deploymentForm=$<HTMLFormElement>("#deployment-form"),deploymentKind=deploymentForm.querySelector<HTMLSelectElement>('[name="kind"]')!;
let deploymentSiteID="",deploymentRevision=0,deploymentBusy=false,deploymentCurrent:{configured:boolean;name:string;server:string;autoDeploy:boolean;deleteRemote:boolean;pending:boolean;lastUrl:string}|null=null;
function deploymentField(name:string){return deploymentForm.querySelector<HTMLInputElement>(`[name="${name}"]`)!;}
function updateDeploymentControls(){
  const cloud=deploymentKind.value==="cloudflare";$("#deployment-token-label").hidden=!cloud;$("#deployment-delete-label").hidden=cloud;$("#select-ssh-key").hidden=cloud;
  $("#deployment-name-label").firstChild!.textContent=cloud ? "Pages 项目名称" : "目标地址";$("#deployment-server-label").firstChild!.textContent=cloud ? "Cloudflare 账号 ID" : "SSH 私钥路径（可留空使用 ssh-agent）";
  deploymentField("name").placeholder=cloud ? "my-site" : "user@example.com:/var/www/site";
  $("#deployment-note").textContent=cloud ? "需要 Cloudflare Pages Edit 令牌。项目不存在时会创建；单文件不得超过 25 MiB。" : "需要已安装的 Cygwin 兼容 rsync 与 Windows OpenSSH；请先在系统 known_hosts 中确认服务器主机密钥。";
  for(const el of deploymentForm.querySelectorAll<HTMLInputElement|HTMLButtonElement|HTMLSelectElement>("input,button,select"))el.disabled=deploymentBusy;
  $<HTMLButtonElement>("#deployment-run").disabled=deploymentBusy || !deploymentCurrent?.configured;$<HTMLButtonElement>("#deployment-disable").disabled=deploymentBusy || !deploymentCurrent?.configured;
}
async function loadDeployment(){
  const id=deploymentSiteID,kind=deploymentKind.value,revision=++deploymentRevision;deploymentCurrent=null;deploymentBusy=true;updateDeploymentControls();$("#deployment-result").textContent="正在读取配置…";
  try{const info=await Library.deploymentInfo(id,kind);if(revision!==deploymentRevision || !deploymentDialog.open)return;deploymentCurrent=info;deploymentField("name").value=info.name || "";deploymentField("server").value=info.server || "";deploymentField("token").value="";deploymentField("autoDeploy").checked=info.autoDeploy;deploymentField("deleteRemote").checked=info.deleteRemote;$("#deployment-result").textContent=info.pending ? "上次发布结果不确定，请检查目标后手动重试。" : info.lastUrl ? `最近部署：${info.lastUrl}` : info.configured ? "已保存配置。" : "尚未配置。";}
  catch(error){if(revision===deploymentRevision)$("#deployment-result").textContent=`读取失败：${error instanceof Error ? error.message : String(error)}`;}
  finally{if(revision===deploymentRevision){deploymentBusy=false;updateDeploymentControls();rememberDialog(deploymentDialog);}}
}
$("#deployment-settings").addEventListener("click",()=>{if(!site || site.imported || saving)return;deploymentSiteID=site.id;deploymentKind.value="ssh";openDialog(deploymentDialog);void loadDeployment();});
deploymentKind.addEventListener("change",()=>void loadDeployment());
$("#deployment-close").addEventListener("click",()=>{if(!deploymentBusy)deploymentDialog.close();});deploymentDialog.addEventListener("cancel",event=>{if(deploymentBusy)event.preventDefault();});deploymentDialog.addEventListener("close",()=>{deploymentRevision++;deploymentField("token").value="";});
$("#select-ssh-key").addEventListener("click",async()=>{if(deploymentBusy)return;deploymentBusy=true;updateDeploymentControls();try{const path=await Library.selectSSHKey();if(path)deploymentField("server").value=path;}catch(error){$("#deployment-result").textContent=String(error);}finally{deploymentBusy=false;updateDeploymentControls();}});
deploymentForm.addEventListener("submit",async event=>{
  event.preventDefault();if(deploymentBusy || !deploymentCurrent)return;const kind=deploymentKind.value,id=deploymentSiteID;
  const name=deploymentField("name").value.trim(),server=deploymentField("server").value.trim(),token=deploymentField("token").value,auto=deploymentField("autoDeploy").checked,remove=kind==="ssh" && deploymentField("deleteRemote").checked;
  deploymentBusy=true;updateDeploymentControls();try{await Library.configureDeployment(id,kind,name,server,token,auto,remove);deploymentField("token").value="";await loadDeployment();rememberDialog(deploymentDialog);}
  catch(error){$("#deployment-result").textContent=`保存失败：${error instanceof Error ? error.message : String(error)}`;}finally{deploymentBusy=false;updateDeploymentControls();}
});
$("#deployment-run").addEventListener("click",async()=>{
  if(deploymentBusy || !deploymentCurrent?.configured)return;const kind=deploymentKind.value,id=deploymentSiteID,info=deploymentCurrent;
  if(deploymentField("name").value.trim()!==info.name || deploymentField("server").value.trim()!==info.server || deploymentField("token").value || deploymentField("autoDeploy").checked!==info.autoDeploy || (kind==="ssh" && deploymentField("deleteRemote").checked!==info.deleteRemote)){$("#deployment-result").textContent="请先保存当前配置。";return;}
  const warning=kind==="ssh" && info.deleteRemote ? "\n远端多余文件将被删除。" : "";if(!window.confirm(`将已保存的站点公开发布到 ${info.name}？${warning}${dirty ? "\n当前编辑内容尚未保存，不会包含在此次发布中。" : ""}`))return;
  deploymentBusy=true;updateDeploymentControls();$("#deployment-result").textContent="正在发布…";
  try{const url=await Library.deploySite(id,kind);await loadDeployment();$("#deployment-result").textContent=url ? `发布完成：${url}` : "发布完成。";}
  catch(error){$("#deployment-result").textContent=`发布未确认：${error instanceof Error ? error.message : String(error)}；请检查目标后重试。`;}
  finally{deploymentBusy=false;updateDeploymentControls();}
});
$("#deployment-disable").addEventListener("click",async()=>{if(deploymentBusy || !deploymentCurrent?.configured)return;deploymentBusy=true;updateDeploymentControls();try{await Library.disableDeployment(deploymentSiteID,deploymentKind.value);await loadDeployment();}catch(error){$("#deployment-result").textContent=`停用失败：${String(error)}`;}finally{deploymentBusy=false;updateDeploymentControls();}});
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
  removedSiteList.textContent="正在读取…"; openDialog(removedSitesDialog);
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
          site=restored; subscription=null; item=null; favoritesView=false; smartView=null; article=null; draft=null; drafts=[]; draftsView=false; articles=[]; editing=false; dirty=false;
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
$("#cancel-site").addEventListener("click", () => { if (!saving) dialog.close(); });
dialog.addEventListener("cancel", event => { if (saving) event.preventDefault(); });
dialog.addEventListener("close", () => { navigationRevision++; });
form.addEventListener("submit", async (event) => {
  event.preventDefault(); if (saving) return;
  $("#site-settings-result").textContent="";const button = form.querySelector<HTMLButtonElement>('button[type="submit"]')!; button.disabled = true;
  const revision = ++navigationRevision;
  const isUpdate=!!editingSiteID, cancel=$<HTMLButtonElement>("#cancel-site");
  saving=true; cancel.disabled=true; busyMessage=isUpdate ? "正在保存站点设置…" : "正在创建站点…"; updateView();
  const submitLabel = button.textContent; button.textContent = isUpdate ? "正在保存…" : "正在创建…";
  try {
    const values = new window.FormData(form); const name=String(values.get("name") ?? ""), about=String(values.get("about") ?? "");
    for (const field of form.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>("input,textarea")) field.disabled = true;
    if (editingSiteID) {
      form.querySelector<HTMLInputElement>('input[name="name"]')!.disabled=true; form.querySelector<HTMLTextAreaElement>('textarea[name="about"]')!.disabled=true; form.querySelector<HTMLInputElement>('input[name="doNotIndex"]')!.disabled=true;
      form.querySelector<HTMLInputElement>('input[name="siteSlug"]')!.disabled=true;form.querySelector<HTMLInputElement>('input[name="authorName"]')!.disabled=true;
      const id=editingSiteID, generation=publicationState(id).generation, existing=sites.find(entry=>entry.id===id), noIndex=form.querySelector<HTMLInputElement>('input[name="doNotIndex"]')!.checked;
      const slug=String(values.get("siteSlug") ?? "").trim(),author=String(values.get("authorName") ?? "").trim();
      const identityChanged=existing && !existing.imported && (slug!==(existing.slug || "") || author!==(existing.authorName || ""));
      const updated=identityChanged ? await Library.updateSiteDetails(id,name,about,noIndex,slug,author) : existing && !existing.imported && noIndex!==!!existing.doNotIndex ? await Library.updateSiteSettings(id,name,about,noIndex) : await Library.updateSite(id,name,about);
      const selected=mergeSitePublication(sites.find(entry=>entry.id===id),updated,generation);
      sitesRevision++; sites=sites.map(entry=>entry.id===id ? selected : entry);
      if (site?.id===id) site=selected;
      dialog.close(); clearError(); updateView(); return;
    }
    const created = await Library.createSite(name, about);
    sitesRevision++; sites=[...sites.filter(entry=>entry.id!==created.id),created]; updateView();
    let refreshError: unknown;
    try { await reloadSites(); } catch(e) { refreshError=e; }
    if (revision !== navigationRevision) { if(refreshError) showError(`站点已创建，但站点列表刷新失败：${String(refreshError)}`); updateView(); return; }
    site = created; favoritesView = false; smartView=null; subscription = null; item = null; articles = []; article = null; draft=null; drafts=[]; draftsView=false; editing = false; dirty = false; dialog.close();
    if (refreshError) showError(`站点已创建，但站点列表刷新失败：${String(refreshError)}`); else clearError(); updateView();
  } catch (e) { $("#site-settings-result").textContent=`${isUpdate ? "保存" : "创建"}失败：${e instanceof Error ? e.message : String(e)}。请检查后重试。`; }
  finally {
    button.disabled = false; button.textContent = submitLabel; saving=false; cancel.disabled=false;
    for (const field of form.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>("input,textarea")) field.disabled = false;
    busyMessage=""; updateView();
  }
});
$("#new-feed").addEventListener("click", () => { if (canLeave()) { navigationRevision++; feedForm.reset(); $("#feed-result").textContent=""; openDialog(feedDialog); } });
$("#cancel-feed").addEventListener("click", () => { if (!saving) feedDialog.close(); });
feedDialog.addEventListener("cancel", event => { if (saving) event.preventDefault(); });
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
async function updateOfflineUsage() {const usage=await Library.offlineMediaUsage();$("#offline-storage-usage").textContent=`已管理媒体 ${formatBytes(usage.totalBytes)}，可清理 ${formatBytes(usage.reclaimableBytes)}（${usage.reclaimableFiles} 个文件）`;cleanupOffline.disabled=usage.reclaimableFiles===0;}
$("#offline-storage").addEventListener("click",async()=>{openDialog(storageDialog);$("#offline-storage-result").textContent="";cleanupOffline.disabled=true;try{await updateOfflineUsage();}catch(e){$("#offline-storage-usage").textContent=`无法计算空间：${String(e)}`;}});
$("#close-offline-storage").addEventListener("click",()=>storageDialog.close());
cleanupOffline.addEventListener("click",async()=>{cleanupOffline.disabled=true;try{const result=await Library.cleanupOfflineMedia();await updateOfflineUsage();$("#offline-storage-result").textContent=`已清理 ${formatBytes(result.reclaimedBytes)}（${result.reclaimedFiles} 个文件）`;}catch(e){$("#offline-storage-result").textContent=`清理未完成，部分文件可能已清理：${String(e)}`;try{await updateOfflineUsage();}catch{}}});
$("#show-favorites").addEventListener("click",()=>{ if(!canLeave()) return; navigationRevision++; site=null; subscription=null; item=null; article=null; draft=null; draftsView=false; editing=false; dirty=false; favoritesView=true; smartView=null; clearError();updateView(); });
for(const [selector,view] of [["#show-today","today"],["#show-unread","unread"]] as const) $(selector).addEventListener("click",()=>{if(!canLeave())return;navigationRevision++;readerRevision++;site=null;subscription=null;item=null;article=null;draft=null;draftsView=false;editing=false;dirty=false;favoritesView=false;smartView=view;clearError();updateView();});
$("#toggle-read").addEventListener("click",()=>{if(!subscription || !item)return;const key=item.key || item.id || item.link;if(key) void setSubscriptionRead(subscription.url,key,!item.read);});
$("#mark-feed-read").addEventListener("click",async()=>{
  if(!subscription)return;const address=subscription.url;
  if(bulkReading.has(address) || [...pendingRead].some(key=>key.startsWith(`${address}\u0000`)))return;
  bulkReading.add(address);updateView();
  try {
    await Library.markSubscriptionRead(address);subscriptionsRevision++;
    const current=subscriptions.find(entry=>entry.url===address);current?.feed.items.forEach(entry=>{entry.read=true;});
    if(subscription?.url===address && item)item.read=true;
    try{await reloadSubscriptions();}catch(error){showError(`已标为已读，但刷新列表失败：${error instanceof Error ? error.message : String(error)}`);}
  } catch(error){showError(`标记已读失败：${error instanceof Error ? error.message : String(error)}`);}
  finally{bulkReading.delete(address);updateView();}
});
const searchDialog=$<HTMLDialogElement>("#search-dialog"),searchInput=$<HTMLInputElement>("#search-input"),searchResults=$("#search-results"),searchStatus=$("#search-status");
let searchRevision=0,searchTimer:ReturnType<typeof setTimeout>|undefined;
async function openSearchResult(result:SearchResult) {
  if(!canLeave())return;
  const revision=++navigationRevision;
  try {
    if(result.siteId && result.articleId){
      const selected=sites.find(entry=>entry.id===result.siteId);
      if(!selected)throw new Error("站点已不可用，请重新搜索");
      const loaded=await Library.articles(result.siteId);
      if(revision!==navigationRevision || !searchDialog.open)return;
      const target=loaded.find(entry=>entry.id===result.articleId);
      if(!target)throw new Error("文章已不可用，请重新搜索");
      site=selected;subscription=null;item=null;favoritesView=false;smartView=null;articles=loaded;article=null;draft=null;draftsView=false;editing=false;dirty=false;
      searchDialog.close();selectArticle(target);return;
    }
    if(result.subscriptionUrl && result.itemKey){
      const selected=subscriptions.find(entry=>entry.url===result.subscriptionUrl);
      const target=selected?.feed.items.find(entry=>(entry.key || entry.id || entry.link)===result.itemKey);
      if(!selected || !target)throw new Error("订阅文章已不可用，请重新搜索");
      dirty=false;selectSubscription(selected);searchDialog.close();selectFeedItem(target);return;
    }
    throw new Error("搜索结果缺少来源");
  } catch(error){if(searchDialog.open)searchStatus.textContent=`无法打开：${error instanceof Error ? error.message : String(error)}`;}
}
async function performSearch() {
  const revision=++searchRevision,query=searchInput.value.trim();searchResults.replaceChildren();
  if(!query){searchStatus.textContent="输入关键词开始搜索。";return;}
  searchStatus.textContent="正在搜索…";
  try {
    const results=await Library.searchArticles(query);
    if(revision!==searchRevision || !searchDialog.open)return;
    searchStatus.textContent=results.length ? `找到 ${results.length} 篇文章${results.length>=200 ? "（最多显示 200 篇）" : ""}` : "没有找到匹配的文章。";
    for(const result of results){const button=articleRow(result.title,"",result.snippet,false,result.sourceName);button.addEventListener("click",()=>void openSearchResult(result));searchResults.append(button);}
  } catch(error){if(revision===searchRevision && searchDialog.open)searchStatus.textContent=`搜索失败：${error instanceof Error ? error.message : String(error)}`;}
}
$("#show-search").addEventListener("click",()=>{openDialog(searchDialog);searchInput.focus();if(searchInput.value.trim())void performSearch();});
searchInput.addEventListener("input",()=>{searchRevision++;clearTimeout(searchTimer);searchResults.replaceChildren();searchStatus.textContent=searchInput.value.trim() ? "正在搜索…" : "输入关键词开始搜索。";searchTimer=setTimeout(()=>void performSearch(),180);});
$("#close-search").addEventListener("click",()=>searchDialog.close());
searchDialog.addEventListener("close",()=>{searchRevision++;clearTimeout(searchTimer);});
starItem.addEventListener("click",async()=>{
  if(!subscription || !item) return; const address=subscription.url, key=item.key || item.id || item.link; if(!key) { showError("文章缺少可用标识"); return; } const starred=!item.starred;
  const operation=`${address}\u0000${key}`;
  if(pendingStars.has(operation)) return;
  pendingStars.add(operation); clearError(); updateView();
  try {
    await Library.starSubscription(address,key,starred);
    subscriptionsRevision++;
    // A favorite only changes local reading state. Reloading the subscription
    // would reopen its iframe, losing scroll position and media playback.
    const current=subscriptions.find(entry=>entry.url===address)?.feed.items.find(entry=>(entry.key || entry.id || entry.link)===key);
    if(current) current.starred=starred;
    if(subscription?.url===address && item && (item.key || item.id || item.link)===key) item.starred=starred;
  }
  catch(e) { showError(e); } finally {pendingStars.delete(operation);updateView();}
});
shareItem.addEventListener("click",async()=>{
  if(!item?.link) return; try { const url=planetShareURL(selectedGateway(publicGateway.value),item.link,!!item.contentCid); shareURL.value=url; try {await window.navigator.clipboard.writeText(url);shareStatus.textContent="链接已复制";} catch {shareURL.hidden=false;shareURL.focus();shareURL.select();shareStatus.textContent="请按 Ctrl+C 复制";} } catch(e) {showError(e);}
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
      site=imported; subscription=null; item=null; favoritesView=false; smartView=null; article=null; draft=null; drafts=[]; draftsView=false; articles=[]; editing=false; dirty=false; navigationRevision++;
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
const templateGallery=$<HTMLDialogElement>("#template-gallery"), templateList=$("#template-list"), templateFrame=$<HTMLIFrameElement>("#template-preview"), templateApply=$<HTMLButtonElement>("#template-apply"), templateImport=$<HTMLButtonElement>("#template-import"), templateClose=$<HTMLButtonElement>("#template-gallery-close");
let gallerySiteID="", gallerySelected="", galleryRevision=0, galleryItems:Builtin[]=[];
async function showTemplatePreview(item:Builtin) {
  gallerySelected=item.id; const revision=++galleryRevision;
  for(const button of templateList.querySelectorAll<HTMLButtonElement>(".template-choice")) button.setAttribute("aria-pressed",String(button.dataset.templateId===item.id));
  $("#template-preview-name").textContent=item.name; $("#template-preview-description").textContent=`${item.description}，build ${item.build}`;
  $("#template-gallery-result").textContent="正在生成预览…";templateFrame.srcdoc="";templateApply.disabled=true;
  try { const html=await Library.builtinTemplatePreview(item.id); if(revision!==galleryRevision || !templateGallery.open)return; templateFrame.srcdoc=html;$("#template-gallery-result").textContent="";templateApply.disabled=false; }
  catch(e) { if(revision===galleryRevision && templateGallery.open) $("#template-gallery-result").textContent=`预览失败：${String(e)}`; }
}
selectTemplate.addEventListener("click", async () => {
  if (!site || saving) return; gallerySiteID=site.id; gallerySelected="";templateList.replaceChildren();templateFrame.srcdoc="";templateApply.disabled=true;$("#template-gallery-result").textContent="正在读取模板…";openDialog(templateGallery);
  const revision=++galleryRevision;
  try { galleryItems=await Library.builtinTemplates();if(revision!==galleryRevision || !templateGallery.open)return;
    for(const item of galleryItems) { const button=document.createElement("button");button.type="button";button.className="template-choice";button.dataset.templateId=item.id;button.setAttribute("aria-pressed","false");const name=document.createElement("strong"),description=document.createElement("small");name.textContent=item.name;description.textContent=item.description;button.append(name,description);button.addEventListener("click",()=>void showTemplatePreview(item));templateList.append(button); }
    const initial=galleryItems.find(item=>item.name===site?.templateName) ?? galleryItems[0];if(initial) void showTemplatePreview(initial);else $("#template-gallery-result").textContent="没有可用的内置模板。";
  } catch(e) {if(revision===galleryRevision && templateGallery.open)$("#template-gallery-result").textContent=`读取失败：${String(e)}`;}
});
templateClose.addEventListener("click",()=>{if(!saving){galleryRevision++;templateGallery.close();}});
templateGallery.addEventListener("cancel",event=>{if(saving)event.preventDefault();else galleryRevision++;});
async function applyTemplate(manual:boolean) {
  if (!site || site.id!==gallerySiteID || saving || (!manual && !gallerySelected)) return;
  const siteID=gallerySiteID; navigationRevision++; saving=true;templateApply.disabled=true;templateImport.disabled=true;templateClose.disabled=true;updateView();clearError();
  try {
    const generation=publicationState(siteID).generation, updated=manual ? await Library.selectTemplate(siteID) : await Library.applyBuiltinTemplate(siteID,gallerySelected);
    if (updated.id) {
      const selected=mergeSitePublication(sites.find(entry=>entry.id===siteID),updated,generation);
      markLocalChange(siteID); sitesRevision++; sites=sites.map(entry=>entry.id===siteID ? selected : entry);
      if (site?.id===siteID) site=selected;
      galleryRevision++;templateGallery.close();
      window.alert(selected.imported ? `已选择模板：${selected.templateName}\n可尝试导出备份；不支持的原站点设置会提示原因。` : `已选择模板：${selected.templateName}\n建议先导出网页检查效果。`);
      if (selected.publishPending) void queuePublication(siteID);
    }
  } catch(e) { $("#template-gallery-result").textContent=`选择失败：${String(e)}`; } finally { saving=false;templateApply.disabled=!gallerySelected;templateImport.disabled=false;templateClose.disabled=false;updateView(); }
}
templateApply.addEventListener("click",()=>void applyTemplate(false));
templateImport.addEventListener("click",()=>void applyTemplate(true));
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
    editingTemplateSiteID=siteID; openDialog(templateSettingsDialog);
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
function telegramContextCurrent(id: string, revision: number): boolean {
  return telegramDialog.open && telegramSiteID===id && site?.id===id && telegramRevision===revision;
}
function telegramHasUnsavedChanges(): boolean {
  return telegramChat.value.trim()!==(telegramCurrent?.chatId ?? "") || !!telegramToken.value;
}
function telegramRevisionPublished(): boolean {
  if(!site || site.id!==telegramSiteID || !site.ipns || !site.lastPublishedCID || site.publishPending || (site.contentRevision ?? 0)>(site.publishedRevision ?? 0))return false;
  const state=publicationStates.get(site.id);
  return !state || !state.running && !state.pending && state.changes<=state.publishedChanges;
}
function telegramCanAct(recover=false): boolean {
  const available=!!telegramCurrent || recover && telegramLoadFailed && !telegramHasUnsavedChanges();
  return !saving && !telegramBusy && !telegramLoading && available && telegramContextCurrent(telegramSiteID,telegramRevision);
}
function updateTelegramControls() {
  const blocked=saving || telegramBusy || telegramLoading || !telegramContextCurrent(telegramSiteID,telegramRevision);
  const unsaved=telegramHasUnsavedChanges();
  telegramChat.disabled=blocked; telegramToken.disabled=blocked;
  telegramToken.required=!telegramCurrent?.configured || telegramChat.value.trim()!==telegramCurrent.chatId;
  telegramForm.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled=blocked || !telegramCurrent;
  $<HTMLButtonElement>("#telegram-close").disabled=saving || telegramBusy;
  const retry=$<HTMLButtonElement>("#telegram-retry"), disable=$<HTMLButtonElement>("#telegram-disable");
  retry.disabled=blocked || !telegramCurrent?.configured || unsaved || !telegramRevisionPublished();
  disable.disabled=blocked || (!telegramCurrent?.configured && !telegramLoadFailed) || unsaved;
  disable.textContent=telegramLoadFailed ? "清除本机配置" : "停用";
  retry.title=unsaved ? "请先保存 Telegram 配置" : !telegramRevisionPublished() ? "请先发布当前已保存内容" : "";
  disable.title=unsaved ? "请先保存或还原 Telegram 配置" : "";
  telegramForm.setAttribute("aria-busy",String(telegramLoading || telegramBusy));
}
function telegramStatus(): string {
  if(!telegramCurrent?.configured)return "尚未配置 Telegram 同步。";
  const info=telegramCurrent;
  return `已连接 ${info.chatTitle || info.chatId}，已同步 ${info.syncedCount} 篇文章。${info.pending ? "上次发送结果不确定，请检查 Telegram 后再重试，重试可能产生重复消息。" : ""}${telegramRevisionPublished() ? "" : "请先发布当前已保存内容，再同步 Telegram。"}`;
}
async function readTelegram(id: string, revision: number, message="") {
  const info=await Library.telegramInfo(id);
  if(!telegramContextCurrent(id,revision))return;
  telegramCurrent=info; telegramLoadFailed=false; telegramChat.value=info.chatId || ""; telegramToken.value="";
  telegramResult.textContent=message ? `${message}${telegramStatus()}` : telegramStatus();
  rememberDialog(telegramDialog);
}
async function loadTelegram() {
  const id=telegramSiteID, revision=++telegramRevision;
  telegramCurrent=null; telegramLoadFailed=false; telegramLoading=true; telegramResult.textContent="正在读取配置…"; updateTelegramControls();
  try { await readTelegram(id,revision); }
  catch(error) { if(telegramContextCurrent(id,revision)){telegramLoadFailed=true;telegramResult.textContent=`读取失败：${error instanceof Error ? error.message : String(error)}。可关闭后重试，或清除本机配置和同步记录后重新配置；已有 Telegram 消息不会删除。`;} }
  finally { if(revision===telegramRevision){telegramLoading=false;updateTelegramControls();} }
}
async function withTelegramOperation(action: (id:string)=>Promise<string>, clearToken=false, recover=false) {
  if(!telegramCanAct(recover))return;
  const id=telegramSiteID, revision=telegramRevision;
  telegramBusy=true; saving=true; navigationRevision++; updateView();
  try {
    const message=await action(id);
    if(!telegramContextCurrent(id,revision))return;
    // A successful configuration change makes the previous saved target stale.
    telegramCurrent=null;
    await readTelegram(id,revision,message);
  } catch(error) {
    if(telegramContextCurrent(id,revision))telegramResult.textContent=`操作失败：${error instanceof Error ? error.message : String(error)}`;
  } finally {
    if(clearToken)telegramToken.value="";
    telegramBusy=false; saving=false; updateView();
  }
}
telegramButton.addEventListener("click",()=>{
  if(!site || saving || telegramBusy)return;
  telegramSiteID=site.id; telegramForm.reset(); telegramCurrent=null; telegramLoadFailed=false;
  openDialog(telegramDialog); void loadTelegram();
});
for(const input of [telegramChat,telegramToken])input.addEventListener("input",()=>{
  updateTelegramControls();
  if(telegramCurrent)telegramResult.textContent=telegramHasUnsavedChanges() ? "配置有未保存的更改，请先保存后再同步。" : telegramStatus();
});
$("#telegram-close").addEventListener("click",()=>{if(!telegramBusy && !saving)telegramDialog.close();});
telegramDialog.addEventListener("cancel",event=>{if(telegramBusy || saving)event.preventDefault();});
telegramDialog.addEventListener("close",()=>{
  telegramRevision++; telegramSiteID=""; telegramCurrent=null; telegramLoading=false; telegramLoadFailed=false; telegramForm.reset();
});
telegramForm.addEventListener("submit",event=>{
  event.preventDefault(); if(!telegramCanAct())return;
  const chat=telegramChat.value.trim(), token=telegramToken.value.trim();
  if(!/^(?:@[A-Za-z0-9_]+|-[1-9]\d*)$/.test(chat)){telegramResult.textContent="请输入 @频道用户名或群组 / 频道的负数 ID。";telegramChat.focus();return;}
  if(!token && (!telegramCurrent?.configured || chat!==telegramCurrent.chatId)){telegramResult.textContent="首次配置或更换目标时，请输入机器人令牌。";telegramToken.focus();return;}
  if(telegramCurrent?.configured && (chat!==telegramCurrent.chatId || !!token) && !window.confirm("更换机器人或群组 / 频道会清除本机同步记录，下次同步可能重新发送全部文章；已有 Telegram 消息不会删除。继续保存？"))return;
  void withTelegramOperation(async id=>{
    await Library.configureTelegram(id,chat,token);
    return "已保存，未发送消息；下次站点发布成功后自动同步。";
  },true);
});
$("#telegram-retry").addEventListener("click",()=>{
  if(!telegramCanAct() || !telegramCurrent?.configured || telegramHasUnsavedChanges() || !telegramRevisionPublished())return;
  if(!window.confirm("将向已配置的 Telegram 群组 / 频道发送所有尚未同步的文章，并更新已同步文章。若上次发送结果不确定，重试可能产生重复消息；请先检查 Telegram。继续同步？"))return;
  void withTelegramOperation(async id=>{const count=await Library.retryTelegram(id);return `同步完成，本次处理 ${count} 篇文章。`;});
});
$("#telegram-disable").addEventListener("click",()=>{
  if(!telegramCanAct(true) || (!telegramCurrent?.configured && !telegramLoadFailed) || telegramHasUnsavedChanges())return;
  const recover=telegramLoadFailed;
  if(!window.confirm(`${recover ? "清除当前站点的 Telegram 本机配置和同步记录" : "停用 Telegram 同步并删除本机配置和同步记录"}？已有 Telegram 消息不会删除；再次启用后可能重新发送全部文章。`))return;
  void withTelegramOperation(async id=>{await Library.disableTelegram(id);return recover ? "已清除本机配置，可以重新配置 Telegram 同步。" : "已停用 Telegram 同步。";},false,recover);
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
    pinnableSiteID=siteID;pinnableConfigured=configured;pinnableForm.reset();pinnableResult.textContent=configured ? "已配置固定服务地址。" : "尚未配置固定服务。";openDialog(pinnableDialog);
  } catch(e){showError(e);} finally {saving=false;title.disabled=false;content.disabled=false;updatePinnableControls();updateView();}
});
$("#pinnable-close").addEventListener("click",()=>{if(!saving)pinnableDialog.close();});
pinnableDialog.addEventListener("cancel",event=>{if(saving)event.preventDefault();});
pinnableDialog.addEventListener("close",()=>{pinnableForm.reset();pinnableSiteID="";});
pinnableForm.addEventListener("submit",event=>{
  event.preventDefault();const input=pinnableForm.querySelector<HTMLInputElement>('[name="endpoint"]')!,endpoint=input.value.trim();
  if(!endpoint){pinnableResult.textContent="请输入固定服务地址。";return;}
  void withPinnableOperation(async()=>{await Library.configurePinnable(pinnableSiteID,endpoint);input.value="";pinnableConfigured=true;rememberDialog(pinnableDialog);pinnableResult.textContent="已保存地址；下次站点发布成功后会请求固定。";});
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
    pinnableResult.textContent=`状态：${status.status}，${identity}，${version}${status.last_known_cid ? `，CID：${status.last_known_cid}` : ""}`;
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
 finally{saving=false;updateServiceControls();updateView();rememberDialog(serviceDialog);}
}
serviceButton.addEventListener("click",()=>{if(!site || saving)return;serviceSiteID=site.id;serviceForm.reset();openDialog(serviceDialog);void loadService();});
serviceKind.addEventListener("change",()=>void loadService());
for(const control of [serviceName,serviceServer,serviceScript,serviceToken])control.addEventListener("input",()=>{updateServiceControls();if(serviceKind.value==="plausible" && serviceHasUnsavedChanges())serviceResult.textContent="配置有未保存的更改，请先保存后再查询。";});
$("#service-close").addEventListener("click",()=>{if(!saving)serviceDialog.close();});
serviceDialog.addEventListener("cancel",e=>{if(saving)e.preventDefault();});
serviceDialog.addEventListener("close",()=>{serviceGeneration++;serviceSiteID="";serviceForm.reset();});
async function serviceOperation(action:()=>Promise<void>){if(saving || !serviceSiteID || site?.id!==serviceSiteID)return;navigationRevision++;saving=true;title.disabled=true;content.disabled=true;updateServiceControls();updateView();try{await action();}catch(e){serviceResult.textContent=`操作失败：${e instanceof Error?e.message:String(e)}`;}finally{saving=false;title.disabled=false;content.disabled=false;updateServiceControls();updateView();}}
serviceForm.addEventListener("submit",e=>{e.preventDefault();const name=serviceName.value.trim(),token=serviceToken.value.trim();if(!name || (!token && serviceKind.value==="filebase")){serviceResult.textContent="请输入名称；Filebase 需要令牌。";return;}if(serviceKind.value==="filebase" && serviceConfigured && !window.confirm("修改 Filebase 名称或令牌会清除本机请求记录，可能导致重复固定及费用。请确认服务端状态后继续。"))return;void serviceOperation(async()=>{if(serviceKind.value==="plausible")await Library.configurePlausibleAt(serviceSiteID,name,token,serviceScript.value.trim(),serviceServer.value.trim());else await Library.configureService(serviceSiteID,serviceKind.value,name,token);serviceSavedName=name;serviceSavedServer=serviceServer.value.trim();serviceSavedScript=serviceScript.value.trim();serviceToken.value="";serviceConfigured=serviceConfigured || !!token;rememberDialog(serviceDialog);if(serviceKind.value==="plausible")markLocalChange(serviceSiteID);serviceResult.textContent=serviceKind.value==="filebase"?"已保存；下次发布后提交固定请求。":"已保存统计配置；重新发布站点后跟踪脚本生效。";});});
$("#service-disable").addEventListener("click",()=>{if((!serviceConfigured && serviceKind.value!=="plausible") || !window.confirm("删除当前服务的本机配置？远程内容不会删除；停用 Plausible 后需重新发布站点。"))return;void serviceOperation(async()=>{if(serviceKind.value==="plausible")markLocalChange(serviceSiteID);await Library.configureService(serviceSiteID,serviceKind.value,"","");serviceConfigured=false;serviceName.value="";serviceToken.value="";serviceResult.textContent="已停用；如更改了跟踪脚本，请重新发布站点。";});});
$("#service-stats").addEventListener("click",()=>{if(!serviceConfigured || serviceKind.value!=="plausible" || serviceHasUnsavedChanges())return;const period=serviceRange.value;void serviceOperation(async()=>{const stats=await Library.plausibleStatsRange(serviceSiteID,period);const label=period==="day"?"今天":period==="30d"?"最近 30 天":"最近 7 天";serviceResult.textContent=`${label}：访客 ${stats.visitors}，浏览量 ${stats.pageviews}。`;});});
$("#service-pin").addEventListener("click",()=>{if(!serviceConfigured || serviceKind.value!=="filebase" || serviceHasUnsavedChanges() || !window.confirm("将为当前版本提交新的 Filebase 固定请求。若此前请求已被接受，可能产生重复固定和费用；请先检查 Filebase。继续吗？"))return;void serviceOperation(async()=>{await Library.requestFilebasePin(serviceSiteID);serviceResult.textContent="请求已提交；请查询固定状态。";});});
$("#service-pin-status").addEventListener("click",()=>{if(!serviceConfigured || serviceKind.value!=="filebase" || serviceHasUnsavedChanges())return;void serviceOperation(async()=>{const status=await Library.filebaseStatus(serviceSiteID);serviceResult.textContent=`状态：${status.status}，CID：${status.pin.cid}${status.pin.cid===site?.lastPublishedCID ? "（当前发布版本）" : "（不是当前发布版本）"}`;});});
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
  event.preventDefault(); if (saving) return;
  const button = feedForm.querySelector<HTMLButtonElement>('button[type="submit"]')!, cancel = $<HTMLButtonElement>("#cancel-feed"), input = feedForm.querySelector<HTMLInputElement>('[name="url"]')!;
  const address = input.value; button.disabled = true; cancel.disabled = true; input.disabled = true;
  saving=true; busyMessage="正在添加订阅…"; button.textContent="正在添加…"; $("#feed-result").textContent=""; updateView();
  const revision = ++navigationRevision;
  try {
    const entry = await Library.follow(address); await reloadSubscriptions();
    if (revision !== navigationRevision) { updateView(); return; }
    site = null; favoritesView = false; smartView=null; subscription = entry; item = null; article = null; draft=null; draftsView=false; editing = false; dirty = false; feedDialog.close(); clearError(); updateView();
  } catch (e) { $("#feed-result").textContent=`添加失败：${e instanceof Error ? e.message : String(e)}。请检查地址后重试。`; }
  finally { button.disabled = false; cancel.disabled=false; input.disabled=false; saving=false; busyMessage=""; button.textContent="添加订阅"; updateView(); }
});
newArticle.addEventListener("click", startArticle);
$("#new-article-header").addEventListener("click", startArticle);
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
publicGateway.addEventListener("change", () => {
  publicGateway.value = selectedGateway(publicGateway.value);
  try { window.localStorage.setItem(gatewayKey, publicGateway.value); }
  catch { showError("无法保存网关设置，本次选择将在关闭程序后失效。"); }
  copyStatus.textContent = ""; shareStatus.textContent = ""; shareURL.hidden = true;
  updateView();
});
copyPublishedURL.addEventListener("click", async () => {
  if (!publishedURL.value) return;
  try { await window.navigator.clipboard.writeText(publishedURL.value); copyStatus.textContent = "已复制"; }
  catch { publishedURL.focus(); publishedURL.select(); copyStatus.textContent = "请按 Ctrl+C 复制"; }
});
emptyAction.addEventListener("click", () => site ? startArticle() : $("#new-site").click());
saveButton.addEventListener("click", () => void save());
for (const input of [title, content]) input.addEventListener("input", () => { navigationRevision++; dirty = true; updateView(); clearTimeout(previewTimer); previewTimer = setTimeout(() => void renderPreview(), 180); });
window.addEventListener("keydown", event => {
  if (event.isComposing || !(event.ctrlKey || event.metaKey)) return;
  const key = event.key.toLowerCase();
  if (key === "s") {
    event.preventDefault();
    if (!document.querySelector("dialog[open]") && editing) saveDraft.click();
  } else if (key === "f" && !document.querySelector("dialog[open]")) { event.preventDefault(); $("#show-search").click(); }
});
window.addEventListener("beforeunload", (event) => { if (dirty || hasDialogChanges(document)) event.preventDefault(); });
async function init() {
  if (libraryLoading) return;
  libraryLoading=true; libraryFailed=[]; updateView();
  const initialNavigation=navigationRevision, initialSites=++sitesRevision, initialSubscriptions=++subscriptionsRevision;
  await Promise.allSettled([
    (async () => {
      try {
        const loaded = await Library.sites();
        if (sitesRevision!==initialSites) return;
        if (navigationRevision===initialNavigation || (!site && sites.length===0)) sites=loaded;
        if (navigationRevision===initialNavigation && !editing && !dirty) {
          if (site && (!article || articleLoadFailures.has(site.id))) {
            const initialSite=site; updateView();
            try { const entries=await Library.articles(initialSite.id); if(navigationRevision===initialNavigation && site?.id===initialSite.id && !editing && !dirty) { articles=entries; articleLoadFailures.delete(initialSite.id); } }
            catch { if(navigationRevision===initialNavigation) { articleLoadFailures.add(initialSite.id); libraryFailed.push("文章"); } }
          }
        }
      } catch { if(sitesRevision===initialSites) libraryFailed.push("站点"); }
      finally { updateView(); }
    })(),
    (async () => {
      try { const loaded=await Library.subscriptions(); if(subscriptionsRevision===initialSubscriptions) subscriptions=loaded; }
      catch { if(subscriptionsRevision===initialSubscriptions) libraryFailed.push("订阅"); }
      finally { updateView(); }
    })()
  ]);
  libraryLoading=false;
  updateView();
  for (const entry of sites) if (entry.publishPending && !publicationStates.get(entry.id)?.running) void queuePublication(entry.id);
  if (!refreshTimer) { void refreshAutomatically(); refreshTimer=window.setInterval(() => void refreshAutomatically(), 15 * 60 * 1000); }
}
$("#retry-library").addEventListener("click", () => void init());
initUpdates();
document.querySelector(".version")!.textContent=release.version;
void init();
