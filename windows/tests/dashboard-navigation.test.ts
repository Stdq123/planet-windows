import { expect, test } from "bun:test";
import { Window } from "happy-dom";

let serial = 0;
const site = { id: "a", name: "First", about: "", created: "2026-01-01T00:00:00Z" };
const other = { ...site, id: "b", name: "Second" };
const draft = { id: "d", siteId: "b", title: "Private draft", content: "Keep my work", created: site.created, modified: site.created, tags: {} };
async function until(check: () => boolean) { for (let i = 0; i < 100; i++) { if (check()) return; await Bun.sleep(10); } throw new Error("UI state timeout"); }
async function fixture(override: (name: string, args: any[]) => unknown = () => undefined) {
  const window = new Window({ url: "http://localhost/" }); window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, { window, document: window.document });
  const calls: string[] = [];
  (globalThis as any).mygo = { call: async (name: string, ...args: any[]) => {
    calls.push(name); const value = override(name, args); if (value !== undefined) return await value;
    switch (name) {
      case "Library.Sites": return [site, other];
      case "Library.Subscriptions": case "Library.Articles": return [];
      case "Library.Drafts": return [draft];
      case "Library.PreviewDraft": case "Library.Preview": return "<p>Preview</p>";
      case "Library.Dashboard": return { siteCount: 2, articleCount: 0, draftCount: 1, unreadCount: 0, sites: [site, other].map(s => ({ ...s, articleCount: 0, draftCount: s.id === "b" ? 1 : 0, hasPublished: false, publishPending: false })), recentArticles: [], recentDrafts: [{ ...draft, siteName: "Second" }] };
      default: throw new Error(`Unexpected call: ${name}`);
    }
  } };
  await import(`../src/main.ts?dashboard-navigation-${++serial}`);
  const $ = <T extends Element = HTMLElement>(selector: string) => window.document.querySelector<T>(selector)!;
  return { window, $, calls };
}

test("startup shows all-sites overview without reading article bodies", async () => {
  const { $, calls } = await fixture(); await until(() => !!$("#dashboard-drafts button"));
  expect($("#dashboard").hidden).toBe(false); expect($(".workspace").hidden).toBe(true);
  expect(calls).not.toContain("Library.Articles"); expect($("#show-dashboard").getAttribute("aria-pressed")).toBe("true");
});

test("skip link precedes navigation and focuses the active main region across modes", async () => {
  const { $, window } = await fixture(); await until(() => !!$("#dashboard-drafts button"));
  const skip = $<HTMLAnchorElement>("#skip-to-main");
  expect(skip).not.toBeNull();
  expect(window.document.body.firstElementChild).toBe(skip);
  expect(skip.textContent).toBe("跳到主要内容");
  const jumpTo = (selector: string) => {
    expect(skip.getAttribute("href")).toBe(selector);
    expect($(selector).hidden).toBe(false);
    expect($(selector).getAttribute("tabindex")).toBe("-1");
    skip.focus(); skip.click();
    expect(window.document.activeElement).toBe($(selector));
  };
  jumpTo("#dashboard");
  $<HTMLButtonElement>("#dashboard-drafts button").click(); await until(() => !$("#editor").hidden);
  jumpTo("#workspace-main");
  $<HTMLButtonElement>("#show-favorites").click();
  jumpTo("#workspace-main");
  $<HTMLButtonElement>("#show-dashboard").click();
  jumpTo("#dashboard");
});

test("cross-site draft entry opens correct editor and home preserves canceled edits", async () => {
  const { window, $ } = await fixture(); await until(() => !!$("#dashboard-drafts button"));
  $<HTMLButtonElement>("#dashboard-drafts button").click(); await until(() => !$("#editor").hidden);
  expect($("#site-title").textContent).toBe("Second");
  const content = $<HTMLTextAreaElement>("#content"); expect(content.value).toBe("Keep my work");
  content.value = "Unsaved changes"; content.dispatchEvent(new window.Event("input"));
  window.confirm = () => false; $<HTMLButtonElement>("#show-dashboard").click();
  expect($("#dashboard").hidden).toBe(true); expect(content.value).toBe("Unsaved changes");
  window.confirm = () => true; $<HTMLButtonElement>("#show-dashboard").click();
  expect($("#dashboard").hidden).toBe(false);
});

test("returning home supersedes a slow cross-site draft navigation", async () => {
  let resolve!: (value: unknown[]) => void;
  const { $ } = await fixture(name => name === "Library.Drafts" ? new Promise(r => resolve = r) : undefined);
  await until(() => !!$("#dashboard-drafts button")); $<HTMLButtonElement>("#dashboard-drafts button").click();
  await until(() => !!resolve); $<HTMLButtonElement>("#show-dashboard").click(); resolve([draft]); await Bun.sleep(20);
  expect($("#dashboard").hidden).toBe(false); expect($(".workspace").hidden).toBe(true);
});

test("completed read status refreshes overview after returning home", async () => {
  let resolve!: () => void, read = false;
  const post = { key: "p", id: "p", title: "Unread story", content: "Text", description: "", link: "https://example.test/p", published: site.created, read: false };
  const { $ } = await fixture(name => {
    if (name === "Library.Subscriptions") return [{ url: "https://example.test/feed", lastUpdated: new Date().toISOString(), feed: { title: "Feed", items: [{ ...post, read }] } }];
    if (name === "Library.Dashboard") return { siteCount: 0, articleCount: 0, draftCount: 0, unreadCount: read ? 0 : 1, sites: [], recentArticles: [], recentDrafts: [] };
    if (name === "Library.SetSubscriptionRead") return new Promise<void>(r => { resolve = () => { read = true; r(); }; });
  });
  await until(() => !!$("#subscriptions button"));
  $<HTMLButtonElement>("#show-unread").click(); $<HTMLButtonElement>("#articles button").click();
  await until(() => !!resolve); $<HTMLButtonElement>("#show-dashboard").click();
  await until(() => $("[data-count='unread']").textContent === "1");
  resolve(); await until(() => $("#unread-count").textContent === "0");
  await until(() => $("[data-count='unread']").textContent === "0");
});

test("dashboard navigation during slow startup fills navigation with the newest site list", async () => {
  let resolve!: (value: unknown[]) => void, reads = 0;
  const { $ } = await fixture(name => name === "Library.Sites" ? (++reads === 1 ? new Promise(r => resolve = r) : [site, other]) : undefined);
  await until(() => !!$("#dashboard-sites button"));
  $<HTMLButtonElement>("#dashboard-sites button").click();
  await until(() => $("#site-title").textContent === "First");
  resolve([]); await until(() => $("#library-notice").hidden === true);
  expect($("#sites").children.length).toBe(2);
});

test("overview search opens the existing dialog and focuses its query field", async () => {
  const { $, window } = await fixture(); await until(() => !!$("#dashboard-drafts button"));
  $<HTMLButtonElement>("#dashboard-search").click();
  expect($<HTMLDialogElement>("#search-dialog").open).toBe(true);
  expect(window.document.activeElement).toBe($("#search-input"));
  $<HTMLButtonElement>("#close-search").click();
  expect($<HTMLDialogElement>("#search-dialog").open).toBe(false);
  expect($("#dashboard").hidden).toBe(false);
});
