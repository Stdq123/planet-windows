import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { enterFirstSite } from "./site-workflow";

type Info = { configured: boolean; chatId: string; chatTitle: string; syncedCount: number; pending: boolean };
const emptyInfo: Info = { configured: false, chatId: "", chatTitle: "", syncedCount: 0, pending: false };
const publishedSite = { id: "s", name: "Site", about: "", created: "2026-01-01T00:00:00Z", ipns: "test-ipns", lastPublishedCID: "test-cid", contentRevision: 1, publishedRevision: 1, publishPending: false };
async function waitFor(predicate: () => boolean) {
  for (let i = 0; i < 150; i++) { if (predicate()) return; await new Promise(resolve => setTimeout(resolve, 10)); }
  throw new Error("Timed out");
}
let scenario = 0;
async function setup(options: { info?: Info; sites?: typeof publishedSite[]; handle?: (method: string, args: any[]) => any } = {}) {
  const window = new Window({ url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, { window, document: window.document });
  const calls: Array<[string, ...any[]]> = [], confirmations: string[] = [], alerts: string[] = [];
  let accept = true, info = options.info ?? emptyInfo;
  window.confirm = message => { confirmations.push(message); return accept; };
  window.alert = message => { alerts.push(message); };
  (globalThis as any).mygo = { call: async (method: string, ...args: any[]) => {
    calls.push([method, ...args]);
    const handled = options.handle?.(method, args); if (handled !== undefined) return await handled;
    switch (method) {
      case "Library.Sites": return options.sites ?? [publishedSite];
      case "Library.Subscriptions": case "Library.Articles": return [];
      case "Library.TelegramInfo": return info;
      case "Library.ConfigureTelegram": info = { configured: true, chatId: "-100123", chatTitle: "News", syncedCount: 0, pending: false }; return;
      case "Library.DisableTelegram": info = emptyInfo; return;
      case "Library.RetryTelegram": info = { ...info, pending: false, syncedCount: 3 }; return 3;
      case "Library.PublishSite": return { cid: "test-cid", ipns: "test-ipns", revision: 1, pending: false };
      default: throw new Error(method);
    }
  }};
  await import(`../src/main.ts?telegram-settings-${++scenario}`);
  enterFirstSite(window);
  await waitFor(() => window.document.querySelector("#site-title")?.textContent === "Site");
  const element = <T extends HTMLElement>(selector: string): T => {
    const value = window.document.querySelector<T>(selector); expect(value).not.toBeNull(); return value!;
  };
  const input = (name: string) => element<HTMLInputElement>(`#telegram-form [name="${name}"]`);
  const click = (selector: string) => element<HTMLButtonElement>(selector).click();
  const open = async () => { click("#telegram-settings"); await waitFor(() => !input("chatId").disabled); };
  const set = (name: string, value: string) => { input(name).value = value; input(name).dispatchEvent(new window.Event("input", { bubbles: true })); };
  const submit = () => element<HTMLFormElement>("#telegram-form").dispatchEvent(new window.Event("submit", { cancelable: true }));
  return { window, calls, confirmations, alerts, element, input, click, open, set, submit, accept: (value: boolean) => { accept = value; } };
}

test("Telegram configuration validates and saves without sending or publishing, then clears the token", async () => {
  const ui = await setup(); await ui.open();
  expect(ui.input("token").type).toBe("password");
  ui.set("chatId", " @news "); ui.set("token", "bot-secret"); ui.submit();
  await waitFor(() => ui.element("#telegram-result").textContent?.includes("已保存") === true);
  expect(ui.calls.filter(call => call[0] === "Library.ConfigureTelegram")).toEqual([["Library.ConfigureTelegram", "s", "@news", "bot-secret"]]);
  expect(ui.calls.some(call => call[0] === "Library.RetryTelegram" || call[0] === "Library.PublishSite")).toBe(false);
  expect(ui.input("token").value).toBe("");
  expect(ui.input("chatId").value).toBe("-100123");
  expect(ui.confirmations).toEqual([]);
});

test("Telegram settings clear secrets on close and load the selected site's configuration", async () => {
  const ui = await setup({ sites: [publishedSite, { ...publishedSite, id: "other", name: "Other" }], handle: (method, args) => method === "Library.TelegramInfo" ? { ...emptyInfo, configured: true, chatId: args[0] === "s" ? "-1001" : "-1002", chatTitle: args[0] } : undefined });
  await ui.open(); ui.set("token", "unsaved-secret"); ui.click("#telegram-close");
  expect(ui.input("token").value).toBe("");
  ui.window.document.querySelectorAll<HTMLButtonElement>("#sites button")[1]!.click();
  await waitFor(() => ui.element("#site-title").textContent === "Other"); await ui.open();
  expect(ui.input("chatId").value).toBe("-1002");
  expect(ui.input("token").value).toBe("");
  ui.submit(); await waitFor(() => ui.calls.some(call => call[0] === "Library.ConfigureTelegram"));
  expect(ui.calls.find(call => call[0] === "Library.ConfigureTelegram")).toEqual(["Library.ConfigureTelegram", "other", "-1002", ""]);
});

test("late Telegram loads cannot overwrite a different site's open settings", async () => {
  let resolveOld!: (info: Info) => void;
  const oldLoad = new Promise<Info>(resolve => { resolveOld = resolve; });
  const ui = await setup({ sites: [publishedSite, { ...publishedSite, id: "other", name: "Other" }], handle: (method, args) => method === "Library.TelegramInfo" ? args[0] === "s" ? oldLoad : { ...emptyInfo, chatId: "-1002" } : undefined });
  ui.click("#telegram-settings"); ui.click("#telegram-close");
  expect(ui.element<HTMLDialogElement>("#telegram-dialog").open).toBe(false);
  ui.window.document.querySelectorAll<HTMLButtonElement>("#sites button")[1]!.click();
  await waitFor(() => ui.element("#site-title").textContent === "Other"); await ui.open();
  resolveOld({ ...emptyInfo, chatId: "-1001" }); await new Promise(resolve => setTimeout(resolve, 20));
  expect(ui.input("chatId").value).toBe("-1002");
});

test("unsaved Telegram chat or token blocks retry and disable, including dispatched clicks", async () => {
  const ui = await setup({ info: { ...emptyInfo, configured: true, chatId: "-1001" } }); await ui.open();
  for (const [name, value] of [["chatId", "-1002"], ["token", "new-secret"]]) {
    ui.set("chatId", "-1001"); ui.set("token", ""); ui.set(name!, value!);
    for (const id of ["#telegram-retry", "#telegram-disable"]) {
      expect(ui.element<HTMLButtonElement>(id).disabled).toBe(true);
      ui.element(id).dispatchEvent(new ui.window.Event("click"));
    }
  }
  expect(ui.calls.some(call => ["Library.RetryTelegram", "Library.DisableTelegram"].includes(call[0]))).toBe(false);
  expect(ui.confirmations).toEqual([]);
});

test("pending Telegram retries require confirmation and show refreshed progress", async () => {
  const ui = await setup({ info: { ...emptyInfo, configured: true, chatId: "-1001", pending: true } }); await ui.open();
  expect(ui.element("#telegram-result").textContent).toContain("不确定");
  ui.accept(false); ui.click("#telegram-retry");
  expect(ui.calls.some(call => call[0] === "Library.RetryTelegram")).toBe(false);
  expect(ui.confirmations[0]).toContain("所有尚未同步"); expect(ui.confirmations[0]).toContain("重复");
  ui.accept(true); ui.click("#telegram-retry");
  await waitFor(() => ui.element("#telegram-result").textContent?.includes("3") === true);
  expect(ui.calls.filter(call => call[0] === "Library.RetryTelegram")).toEqual([["Library.RetryTelegram", "s"]]);
});

test("Telegram retry is blocked until the current saved revision is published", async () => {
  const ui = await setup({ sites: [{ ...publishedSite, contentRevision: 2 }], info: { ...emptyInfo, configured: true, chatId: "-1001" } }); await ui.open();
  expect(ui.element<HTMLButtonElement>("#telegram-retry").disabled).toBe(true);
  ui.element("#telegram-retry").dispatchEvent(new ui.window.Event("click"));
  expect(ui.calls.some(call => call[0] === "Library.RetryTelegram")).toBe(false);
  expect(ui.element("#telegram-result").textContent).toContain("发布");
});

test("changing an existing Telegram target requires a token and confirmation before resetting history", async () => {
  const ui = await setup({ info: { ...emptyInfo, configured: true, chatId: "-1001", syncedCount: 8 } }); await ui.open();
  ui.set("chatId", "-1002"); ui.submit();
  expect(ui.calls.some(call => call[0] === "Library.ConfigureTelegram")).toBe(false);
  expect(ui.element("#telegram-result").textContent).toContain("令牌");
  ui.set("token", "replacement-secret"); ui.accept(false); ui.submit();
  expect(ui.calls.some(call => call[0] === "Library.ConfigureTelegram")).toBe(false);
  expect(ui.confirmations.at(-1)).toContain("记录");
  ui.accept(true); ui.submit();
  await waitFor(() => ui.input("token").value === "");
  expect(ui.calls.filter(call => call[0] === "Library.ConfigureTelegram")).toEqual([["Library.ConfigureTelegram", "s", "-1002", "replacement-secret"]]);
});

test("failed Telegram configuration clears the submitted secret and retains an actionable error", async () => {
  const ui = await setup({ handle: method => method === "Library.ConfigureTelegram" ? Promise.reject(new Error("机器人权限不足")) : undefined }); await ui.open();
  ui.set("chatId", "@news"); ui.set("token", "secret"); ui.submit();
  await waitFor(() => ui.element("#telegram-result").textContent?.includes("权限不足") === true);
  expect(ui.input("token").value).toBe("");
  expect(ui.element<HTMLDialogElement>("#telegram-dialog").open).toBe(true);
  expect(ui.element<HTMLButtonElement>("#telegram-retry").disabled).toBe(true);
});

test("disabling Telegram confirms local history removal and refreshes configuration", async () => {
  const ui = await setup({ info: { ...emptyInfo, configured: true, chatId: "-1001", syncedCount: 4 } }); await ui.open();
  ui.click("#telegram-disable");
  await waitFor(() => ui.element("#telegram-result").textContent?.includes("已停用") === true);
  expect(ui.confirmations[0]).toContain("不会删除");
  expect(ui.calls.filter(call => call[0] === "Library.DisableTelegram")).toEqual([["Library.DisableTelegram", "s"]]);
  expect(ui.input("chatId").value).toBe("");
  expect(ui.element<HTMLButtonElement>("#telegram-retry").disabled).toBe(true);
});

test("unreadable Telegram configuration can be cleared only after loading and confirmation", async () => {
  let rejectLoad!: (error: Error) => void;
  const failedLoad = new Promise<Info>((_, reject) => { rejectLoad = reject; });
  let reads = 0;
  const ui = await setup({ handle: method => method === "Library.TelegramInfo" && ++reads === 1 ? failedLoad : undefined });
  ui.click("#telegram-settings");
  expect(ui.element<HTMLButtonElement>("#telegram-disable").disabled).toBe(true);
  ui.element("#telegram-disable").dispatchEvent(new ui.window.Event("click"));
  expect(ui.confirmations).toEqual([]);
  rejectLoad(new Error("无法解密本机配置"));
  await waitFor(() => ui.element("#telegram-result").textContent?.includes("无法解密") === true);
  expect(ui.element<HTMLButtonElement>("#telegram-disable").disabled).toBe(false);
  expect(ui.element<HTMLButtonElement>('#telegram-form button[type="submit"]').disabled).toBe(true);
  ui.set("token", "unsaved-secret");
  expect(ui.element<HTMLButtonElement>("#telegram-disable").disabled).toBe(true);
  ui.element("#telegram-disable").dispatchEvent(new ui.window.Event("click"));
  expect(ui.confirmations).toEqual([]);
  ui.set("token", ""); ui.accept(false); ui.click("#telegram-disable");
  expect(ui.calls.some(call => call[0] === "Library.DisableTelegram")).toBe(false);
  ui.accept(true); ui.click("#telegram-disable");
  await waitFor(() => !ui.element<HTMLButtonElement>('#telegram-form button[type="submit"]').disabled);
  expect(ui.calls.filter(call => call[0] === "Library.DisableTelegram")).toEqual([["Library.DisableTelegram", "s"]]);
  expect(ui.input("chatId").value).toBe(""); expect(ui.input("token").value).toBe("");
  expect(ui.element<HTMLButtonElement>("#telegram-disable").disabled).toBe(true);
  ui.set("chatId", "@news"); ui.set("token", "replacement-secret"); ui.submit();
  await waitFor(() => ui.calls.some(call => call[0] === "Library.ConfigureTelegram"));
  expect(ui.calls.find(call => call[0] === "Library.ConfigureTelegram")).toEqual(["Library.ConfigureTelegram", "s", "@news", "replacement-secret"]);
});

test("Telegram and X settings remain available for the same selected site", async () => {
  const ui = await setup({ handle: method => method === "Library.XSyncInfo" ? { configured: false, autoSync: false, posts: [] } : undefined });
  const menu = ui.element("#workspace-menu");
  expect(menu.querySelector("#telegram-settings")).not.toBeNull();
  expect(menu.querySelector("#x-sync-settings")).not.toBeNull();
  await ui.open();
  expect(ui.element<HTMLDialogElement>("#telegram-dialog").open).toBe(true);
  ui.click("#telegram-close"); ui.click("#x-sync-settings");
  await waitFor(() => !ui.element<HTMLButtonElement>("#x-sync-close").disabled);
  expect(ui.element<HTMLDialogElement>("#telegram-dialog").open).toBe(false);
  expect(ui.element<HTMLDialogElement>("#x-sync-dialog").open).toBe(true);
  expect(ui.calls.filter(call => ["Library.TelegramInfo", "Library.XSyncInfo"].includes(call[0]))).toEqual([["Library.TelegramInfo", "s"], ["Library.XSyncInfo", "s"]]);
  expect(ui.calls.some(call => ["Library.RetryTelegram", "Library.SyncXArticle", "Library.PublishSite"].includes(call[0]))).toBe(false);
});

test("publication displays X, Telegram and pinning warnings together without marking publication failed", async () => {
  const ui = await setup({ handle: method => method === "Library.PublishSite" ? { cid: "test-cid", ipns: "test-ipns", revision: 1, pending: false, pinnableWarning: "固定服务暂时不可用。站点已发布，但 X 同步未完成", telegramWarning: "Telegram 同步未完成" } : undefined });
  ui.click("#publish-site"); await waitFor(() => ui.alerts.length === 1);
  expect(ui.element("#error").textContent).toContain("固定服务暂时不可用");
  expect(ui.element("#error").textContent).toContain("X 同步未完成");
  expect(ui.element("#error").textContent).toContain("Telegram 同步未完成");
  expect(ui.alerts[0]).toContain("固定服务暂时不可用");
  expect(ui.alerts[0]).toContain("X 同步未完成");
  expect(ui.alerts[0]).toContain("Telegram 同步未完成");
  expect(ui.element("#save-state").textContent).toContain("已发布");
});
