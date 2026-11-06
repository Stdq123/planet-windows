import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { enterFirstSite } from "./site-workflow";

type Post = {articleId: string; title: string; status: string; url: string; error: string; canSync: boolean};
const post = (articleId: string, status = "", canSync = true): Post => ({articleId, title: `文章 ${articleId}`, status, url: "", error: "", canSync});
async function waitFor(fn: () => boolean) {
  for (let i = 0; i < 150; i++) { if (fn()) return; await new Promise(resolve => setTimeout(resolve, 10)); }
  throw Error("timed out waiting for X sync UI");
}
let fixtureID = 0;
async function setup(configured = false, posts: Post[] = [], failInfo = false) {
  const window = new Window({url: "http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document: window.document});
  window.confirm = () => true;
  const sites = ["s", "other"].map(id => ({id, name: id === "s" ? "Site" : "Other", about: "", created: "2026-01-01T00:00:00Z"}));
  const state = {configured, autoSync: false, posts};
  const saves: unknown[][] = [], syncs: unknown[][] = [], calls: string[] = [];
  let saveDelay: Promise<void> | undefined;
  (globalThis as any).mygo = {call: async (method: string, ...args: any[]) => {
    calls.push(method);
    switch (method) {
      case "Library.Sites": return sites;
      case "Library.Subscriptions": case "Library.Articles": return [];
      case "Library.XSyncInfo":
        if (failInfo) throw Error("无法读取同步记录");
        return structuredClone({...state, posts: state.posts.map(post => ({...post, canSync: state.configured && post.canSync}))});
      case "Library.ConfigureXSync":
        saves.push(args); await saveDelay; state.configured = true; state.autoSync = args[5]; return;
      case "Library.DisableXSync": state.configured = false; state.autoSync = false; return;
      case "Library.SyncXArticle": {
        syncs.push(args); const entry = state.posts.find(post => post.articleId === args[1])!;
        Object.assign(entry, {status: "sent", url: "https://x.com/i/web/status/12345", error: "", canSync: false});
        return structuredClone(entry);
      }
      default: throw Error(method);
    }
  }};
  await import(`../src/main.ts?x-sync-${fixtureID++}`);
  enterFirstSite(window);
  await waitFor(() => window.document.querySelector("#site-title")?.textContent === "Site");
  const button = window.document.querySelector<HTMLButtonElement>("#x-sync-settings");
  expect(button).not.toBeNull();
  expect(button!.disabled).toBe(false);
  expect(calls).not.toContain("Library.XSyncInfo");
  button!.click();
  const form = window.document.querySelector<HTMLFormElement>("#x-sync-form")!;
  const dialog = window.document.querySelector<HTMLDialogElement>("#x-sync-dialog")!;
  const input = (name: string) => form.querySelector<HTMLInputElement>(`[name="${name}"]`)!;
  const submit = () => form.dispatchEvent(new window.Event("submit", {cancelable: true}));
  const syncButton = (id: string) => form.querySelector<HTMLButtonElement>(`[data-article-id="${id}"] button`)!;
  await waitFor(() => window.document.querySelector<HTMLButtonElement>("#x-sync-close")?.disabled === false);
  return {window, form, dialog, state, saves, syncs, calls, input, submit, syncButton,
    delaySave: (promise: Promise<void>) => { saveDelay = promise; }};
}

test("X credentials save locally, clear after success and close, and never send posts on save", async () => {
  const ui = await setup(false, [post("old")]);
  for (const name of ["apiKey", "apiSecret", "accessToken", "accessTokenSecret"]) {
    expect(ui.input(name).type).toBe("password"); ui.input(name).value = `${name}-test`;
  }
  ui.input("autoSync").checked = true;
  ui.submit();
  await waitFor(() => ui.saves.length === 1 && !ui.form.querySelector<HTMLButtonElement>('[type="submit"]')!.disabled);
  expect(ui.saves[0]).toEqual(["s", "apiKey-test", "apiSecret-test", "accessToken-test", "accessTokenSecret-test", true]);
  expect(ui.syncs).toEqual([]);
  expect(ui.syncButton("old").disabled).toBe(false);
  for (const field of ui.form.querySelectorAll<HTMLInputElement>('[type="password"]')) expect(field.value).toBe("");
  ui.input("apiKey").value = "discard-test";
  ui.window.document.querySelector<HTMLButtonElement>("#x-sync-close")!.click();
  expect(ui.dialog.open).toBe(false);
  for (const field of ui.form.querySelectorAll<HTMLInputElement>('[type="password"]')) expect(field.value).toBe("");
});

test("saved X credentials can stay blank but partial replacement is rejected", async () => {
  const ui = await setup(true);
  expect(ui.form.checkValidity()).toBe(true);
  ui.input("apiKey").value = "partial-test"; ui.submit();
  expect(ui.saves).toHaveLength(0);
  expect(ui.window.document.querySelector("#x-sync-result")!.textContent).toContain("四项");
  ui.input("apiKey").value = ""; ui.input("autoSync").checked = true; ui.submit();
  await waitFor(() => ui.saves.length === 1 && !ui.form.querySelector<HTMLButtonElement>('[type="submit"]')!.disabled);
  expect(ui.saves[0]).toEqual(["s", "", "", "", "", true]);
  expect(ui.syncs).toHaveLength(0);
});

test("X history needs explicit sync and uncertain or failed retries require confirmation", async () => {
  const failed = {...post("failed", "failed"), error: "测试错误：写入权限不足"};
  const ui = await setup(true, [post("history"), post("pending", "pending"), failed, post("sent", "sent"), post("page", "", false)]);
  expect(ui.syncs).toHaveLength(0);
  expect(ui.syncButton("sent").disabled).toBe(true);
  expect(ui.syncButton("page").disabled).toBe(true);
  expect(ui.form.textContent).toContain(failed.error);
  ui.syncButton("history").click();
  await waitFor(() => ui.syncButton("history").disabled && ui.syncs.length === 1);
  expect(ui.syncs[0]).toEqual(["s", "history", false]);
  await waitFor(() => !ui.syncButton("pending").disabled);
  let confirmation = "";
  ui.window.confirm = text => { confirmation = text; return false; };
  ui.syncButton("pending").click();
  expect(confirmation).toContain("X"); expect(confirmation).toContain("检查"); expect(ui.syncs).toHaveLength(1);
  ui.window.confirm = () => true; ui.syncButton("pending").click();
  await waitFor(() => ui.syncs.length === 2 && !ui.syncButton("failed").disabled);
  expect(ui.syncs[1]).toEqual(["s", "pending", true]);
  ui.window.confirm = () => false; ui.syncButton("failed").click(); expect(ui.syncs).toHaveLength(2);
  ui.window.confirm = () => true; ui.syncButton("failed").click();
  await waitFor(() => ui.syncs.length === 3);
  expect(ui.syncs[2]).toEqual(["s", "failed", true]);
});

test("unsaved X credentials or automatic preference block posting", async () => {
  const ui = await setup(true, [post("history")]);
  expect(ui.syncButton("history").disabled).toBe(false);
  ui.input("apiKey").value = "unsaved-test"; ui.input("apiKey").dispatchEvent(new ui.window.Event("input", {bubbles: true}));
  expect(ui.syncButton("history").disabled).toBe(true);
  ui.input("apiKey").value = ""; ui.input("apiKey").dispatchEvent(new ui.window.Event("input", {bubbles: true}));
  expect(ui.syncButton("history").disabled).toBe(false);
  ui.input("autoSync").checked = true; ui.input("autoSync").dispatchEvent(new ui.window.Event("change", {bubbles: true}));
  expect(ui.syncButton("history").disabled).toBe(true);
  ui.syncButton("history").click(); expect(ui.syncs).toHaveLength(0);
});

test("X operations prevent duplicate submits, closing and switching sites until finished", async () => {
  const ui = await setup(true);
  let release!: () => void;
  ui.delaySave(new Promise<void>(resolve => { release = resolve; }));
  ui.submit(); ui.submit();
  expect(ui.saves).toHaveLength(1);
  expect(ui.window.document.querySelector<HTMLButtonElement>("#x-sync-close")!.disabled).toBe(true);
  const cancel = new ui.window.Event("cancel", {cancelable: true}); ui.dialog.dispatchEvent(cancel);
  expect(cancel.defaultPrevented).toBe(true);
  ui.window.document.querySelectorAll<HTMLButtonElement>("#sites button")[1]!.click();
  await new Promise(resolve => setTimeout(resolve, 20));
  expect(ui.window.document.querySelector("#site-title")!.textContent).toBe("Site");
  release(); await waitFor(() => !ui.form.querySelector<HTMLButtonElement>('[type="submit"]')!.disabled);
});

test("disabling X keeps sent records and only verified X post links are clickable", async () => {
  const sent = {...post("sent", "sent"), url: "https://x.com/i/web/status/12345"};
  const unsafe = {...post("unsafe", "sent"), url: "javascript:alert(1)"};
  const ui = await setup(true, [sent, unsafe]);
  const link = ui.form.querySelector<HTMLAnchorElement>('[data-article-id="sent"] a')!;
  expect(link.href).toBe(sent.url); expect(link.rel).toContain("noopener");
  expect(ui.form.querySelector('[data-article-id="unsafe"] a')).toBeNull();
  ui.window.document.querySelector<HTMLButtonElement>("#x-sync-disable")!.click();
  await waitFor(() => !ui.state.configured && !ui.form.querySelector<HTMLButtonElement>('[type="submit"]')!.disabled);
  expect(ui.syncButton("sent").disabled).toBe(true);
  expect(ui.form.querySelector('[data-article-id="sent"]')!.textContent).toContain("已同步");
  expect(ui.state.posts).toHaveLength(2);
});

test("an X settings read failure leaves the dialog closable and credentials empty", async () => {
  const ui = await setup(false, [], true);
  expect(ui.window.document.querySelector("#x-sync-result")!.textContent).toContain("无法读取同步记录");
  ui.window.document.querySelector<HTMLButtonElement>("#x-sync-close")!.click();
  expect(ui.dialog.open).toBe(false);
  expect(ui.saves).toHaveLength(0);
});

test("X can be stopped when settings and records remain unreadable", async () => {
  const ui = await setup(true, [], true);
  const disable = ui.window.document.querySelector<HTMLButtonElement>("#x-sync-disable")!;
  expect(disable.disabled).toBe(false);
  expect(ui.form.querySelector<HTMLButtonElement>('[type="submit"]')!.disabled).toBe(true);
  disable.click();
  await waitFor(() => ui.calls.includes("Library.DisableXSync") && !ui.window.document.querySelector<HTMLButtonElement>("#x-sync-close")!.disabled);
  expect(ui.state.configured).toBe(false);
  expect(ui.window.document.querySelector("#x-sync-result")!.textContent).toContain("已停用并删除凭据");
  expect(ui.window.document.querySelector("#x-sync-result")!.textContent).toContain("文章状态读取失败");
  expect(disable.disabled).toBe(true);
  expect(ui.input("autoSync").checked).toBe(false);
  expect(ui.saves).toHaveLength(0);
  expect(ui.syncs).toHaveLength(0);
});
