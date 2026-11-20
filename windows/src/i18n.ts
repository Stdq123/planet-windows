export type Locale = "zh-CN" | "zh-TW" | "en";

const translations: Record<Locale, Record<string, string>> = {
  "zh-CN": {
    dashboard: "写作台",
    search: "搜索文章",
    mySites: "我的站点",
    reading: "阅读",
    today: "今天",
    unread: "未读",
    favorites: "收藏",
    subscriptions: "订阅",
    ipfsNode: "IPFS 节点",
    settings: "设置",
    keyManager: "IPNS 密钥管理器",
    appLogs: "运行日志",
    downloads: "下载管理器",
    publishedFolders: "发布文件夹",
    quickPost: "快速发帖",
    localAPI: "本地 API 控制台",
    aiAssistant: "AI 助手与设置",
    languageLabel: "界面语言",
  },
  "zh-TW": {
    dashboard: "寫作台",
    search: "搜尋文章",
    mySites: "我的站點",
    reading: "閱讀",
    today: "今天",
    unread: "未讀",
    favorites: "收藏",
    subscriptions: "訂閱",
    ipfsNode: "IPFS 節點",
    settings: "設定",
    keyManager: "IPNS 金鑰管理器",
    appLogs: "執行日誌",
    downloads: "下載管理器",
    publishedFolders: "發布資料夾",
    quickPost: "快速發帖",
    localAPI: "本機 API 控制台",
    aiAssistant: "AI 助手與設定",
    languageLabel: "介面語言",
  },
  en: {
    dashboard: "Dashboard",
    search: "Search",
    mySites: "My Planets",
    reading: "Reading",
    today: "Today",
    unread: "Unread",
    favorites: "Starred",
    subscriptions: "Following",
    ipfsNode: "IPFS Node",
    settings: "Settings",
    keyManager: "Key Manager",
    appLogs: "App Logs",
    downloads: "Downloads",
    publishedFolders: "Published Folders",
    quickPost: "Quick Post",
    localAPI: "Local API Console",
    aiAssistant: "AI Assistant",
    languageLabel: "Language",
  },
};

export function normalizeLocale(raw: string | null | undefined): Locale {
  if (raw === "zh-TW" || raw === "en") return raw;
  return "zh-CN";
}

export function applyLocale(doc: Document, locale: Locale) {
  const dict = translations[locale] ?? translations["zh-CN"];
  doc.documentElement.lang = locale;
  const setLabel = (selector: string, key: string) => {
    const el = doc.querySelector<HTMLElement>(selector);
    if (el && dict[key]) el.textContent = dict[key];
  };
  setLabel("#show-dashboard .nav-label", "dashboard");
  setLabel("#show-search .nav-label", "search");
  setLabel("#sites-heading", "mySites");
  setLabel("#reading-heading", "reading");
  setLabel("#show-today .nav-label", "today");
  setLabel("#show-unread .nav-label", "unread");
  setLabel("#show-favorites .nav-label", "favorites");
  setLabel("#subscriptions-heading", "subscriptions");
  setLabel("#show-ipfs-status .nav-label", "ipfsNode");
  setLabel(".sidebar-tools > summary .nav-label", "settings");
}
