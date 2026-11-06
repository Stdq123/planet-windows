import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { hasDialogChanges, openDialog, prepareDialogs } from "../src/ui";
import { enterFirstSite } from "./site-workflow";

function fixture() {
  const window = new Window({url: "http://localhost/"});
  window.document.body.innerHTML = `
    <details id="menu" open><summary>更多操作</summary><button id="entry">站点设置</button></details>
    <button id="other">其他入口</button>
    <main id="dashboard" tabindex="-1" hidden><h1>写作台</h1></main>
    <main id="workspace-main" tabindex="-1"><h1>文章</h1></main>
    <dialog id="settings"><form><h2>设置</h2><input name="name" value="原值">
      <button id="nested-entry" type="button">更多设置</button>
      <div class="dialog-actions"><button id="cancel" type="button">取消</button><button type="submit">保存</button></div>
    </form></dialog>
    <dialog id="nested"><h2>更多设置</h2><input id="nested-input">
      <div class="dialog-actions"><button type="button">关闭</button></div>
    </dialog>`;
  const $ = <T extends Element = HTMLElement>(selector: string) => window.document.querySelector<T>(selector)!;
  prepareDialogs(window.document as unknown as Document);
  const dialog = $<HTMLDialogElement>("#settings"), nested = $<HTMLDialogElement>("#nested");
  const entry = $<HTMLButtonElement>("#entry"), menu = $<HTMLDetailsElement>("#menu");
  $("#cancel").addEventListener("click", () => dialog.close());
  // happy-dom does not implement native showModal focus movement or Escape's default close action.
  const focusInside = () => $<HTMLInputElement>("#settings input").focus();
  const escape = () => {
    const event = new window.Event("cancel", {cancelable: true});
    dialog.dispatchEvent(event);
    if (!event.defaultPrevented) dialog.close();
    return event;
  };
  return {window, $, dialog, nested, entry, menu, focusInside, escape};
}

test("closing a menu dialog returns focus to the summary after its entry is folded away", () => {
  const {window, $, dialog, entry, menu, focusInside} = fixture();
  entry.focus(); openDialog(dialog); menu.open = false; focusInside();
  $("#cancel").click();
  expect(dialog.open).toBe(false);
  expect(window.document.activeElement === menu.querySelector("summary")).toBe(true);
});

test("closing a dialog preserves its visible trigger as the first return target", () => {
  const {window, dialog, entry, focusInside} = fixture();
  entry.focus(); openDialog(dialog); focusInside(); dialog.close();
  expect(window.document.activeElement === entry).toBe(true);
});

test("an asynchronous opener survives a folded menu and focus already lost to body", () => {
  const {window, dialog, entry, menu, focusInside} = fixture();
  entry.focus(); menu.open = false; entry.blur();
  expect(window.document.activeElement === window.document.body).toBe(true);
  openDialog(dialog, entry); focusInside(); dialog.close();
  expect(window.document.activeElement === menu.querySelector("summary")).toBe(true);
});

test("a folded outer details returns to its visible summary", () => {
  const {window, dialog, entry, menu, focusInside} = fixture();
  const outer = window.document.createElement("details");
  outer.innerHTML = "<summary>工具</summary>"; menu.before(outer); outer.append(menu); outer.open = true;
  entry.focus(); openDialog(dialog); menu.open = false; outer.open = false; focusInside(); dialog.close();
  expect(window.document.activeElement === outer.querySelector(":scope > summary")).toBe(true);
});

test.each(["removed", "hidden", "disabled", "inert", "css-hidden", "aria-disabled"])(
  "an unavailable trigger (%s) returns focus to the visible main content",
  reason => {
    const {window, $, dialog, focusInside} = fixture();
    const trigger = $<HTMLButtonElement>("#other");
    trigger.focus(); openDialog(dialog); focusInside();
    if (reason === "removed") trigger.remove();
    if (reason === "hidden") trigger.hidden = true;
    if (reason === "disabled") trigger.disabled = true;
    if (reason === "inert") trigger.setAttribute("inert", "");
    if (reason === "css-hidden") trigger.style.display = "none";
    if (reason === "aria-disabled") trigger.setAttribute("aria-disabled", "true");
    dialog.close();
    expect(window.document.activeElement === $("#workspace-main")).toBe(true);
  }
);

test("an unavailable menu returns focus to the current dashboard instead of hidden workspace", () => {
  const {window, $, dialog, entry, menu, focusInside} = fixture();
  entry.focus(); openDialog(dialog); focusInside();
  menu.hidden = true; $("#workspace-main").hidden = true; $("#dashboard").hidden = false;
  dialog.close();
  expect(window.document.activeElement === $("#dashboard")).toBe(true);
});

test("reopening updates the return target and direct native opens do not reuse an old target", () => {
  const {window, $, dialog, entry, focusInside} = fixture();
  entry.focus(); openDialog(dialog); focusInside(); dialog.close();
  const other = $("#other"); other.focus(); openDialog(dialog); focusInside(); dialog.close();
  expect(window.document.activeElement === other).toBe(true);
  entry.focus(); dialog.showModal(); dialog.close();
  expect(window.document.activeElement === entry).toBe(true);
});

test("opening an already open dialog preserves both its return target and unsaved baseline", () => {
  const {window, $, dialog, entry, focusInside} = fixture();
  entry.focus(); openDialog(dialog); focusInside();
  $<HTMLInputElement>("#settings input").value = "未保存";
  openDialog(dialog);
  expect(hasDialogChanges(window.document as unknown as Document)).toBe(true);
  dialog.close();
  expect(window.document.activeElement === entry).toBe(true);
});

test("closing a nested dialog restores its parent trigger before the original outside trigger", () => {
  const {window, $, dialog, nested, entry, focusInside} = fixture();
  entry.focus(); openDialog(dialog); focusInside();
  const parentTrigger = $("#nested-entry"); parentTrigger.focus(); openDialog(nested);
  $("#nested-input").focus(); nested.close();
  expect(window.document.activeElement === parentTrigger).toBe(true);
  dialog.close();
  expect(window.document.activeElement === entry).toBe(true);
});

test("closing a lower dialog does not take focus from another open modal", () => {
  const {window, $, dialog, nested, entry, focusInside} = fixture();
  entry.focus(); openDialog(dialog); focusInside();
  $("#nested-entry").focus(); openDialog(nested); $("#nested-input").focus();
  dialog.close();
  expect(window.document.activeElement === $("#nested-input")).toBe(true);
  nested.close();
  expect(window.document.activeElement === $("#workspace-main")).toBe(true);
});

test("unsaved cancellation keeps focus inside until the user accepts discarding changes", () => {
  const {window, $, dialog, entry, focusInside, escape} = fixture();
  entry.focus(); openDialog(dialog); focusInside();
  const input = $<HTMLInputElement>("#settings input"); input.value = "未保存";
  window.confirm = () => false;
  expect(escape().defaultPrevented).toBe(true);
  $("#cancel").click();
  expect(dialog.open).toBe(true);
  expect(window.document.activeElement === input).toBe(true);
  window.confirm = () => true;
  expect(escape().defaultPrevented).toBe(false);
  expect(window.document.activeElement === entry).toBe(true);
});

test("busy cancellation retains focus and a successful save can still close and restore it", () => {
  const {window, $, dialog, entry, focusInside, escape} = fixture();
  entry.focus(); openDialog(dialog); focusInside();
  const input = $<HTMLInputElement>("#settings input"); input.value = "保存中";
  $<HTMLButtonElement>('#settings button[type="submit"]').disabled = true;
  expect(escape().defaultPrevented).toBe(true);
  $("#cancel").click();
  expect(dialog.open).toBe(true);
  expect(window.document.activeElement === input).toBe(true);
  dialog.close();
  expect(window.document.activeElement === entry).toBe(true);
});

test("dismissWhileBusy retains its existing Escape exception", () => {
  const {window, $, dialog, entry, focusInside, escape} = fixture();
  entry.focus(); openDialog(dialog); focusInside();
  dialog.dataset.dismissWhileBusy = "true";
  $<HTMLButtonElement>('#settings button[type="submit"]').disabled = true;
  expect(escape().defaultPrevented).toBe(false);
  expect(dialog.open).toBe(false);
  expect(window.document.activeElement === entry).toBe(true);
});

test.each([
  ["template-settings", "template-settings-dialog", "cancel-template-settings", "Library.TemplateSettings"],
  ["pinnable-settings", "pinnable-dialog", "pinnable-close", "Library.PinnableConfigured"]
])("the real asynchronous %s entry returns to its menu after loading", async (entryID, dialogID, closeID, method) => {
  const window = new Window({url: "http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document: window.document});
  const site = {id: "s", name: "Site", about: "", templateId: "template-sepia", templateName: "Sepia", created: "2026-01-01T00:00:00Z"};
  let release: (() => void) | undefined;
  (globalThis as any).mygo = {call: async (name: string) => {
    if (name === method) return await new Promise(resolve => { release = () => resolve(name === "Library.TemplateSettings" ? {} : false); });
    switch (name) {
      case "Library.Sites": return [site];
      case "Library.Subscriptions": case "Library.Articles": return [];
      default: throw new Error(name);
    }
  }};
  const until = async (condition: () => boolean) => {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (condition()) return;
      await Bun.sleep(10);
    }
    throw new Error("The asynchronous settings dialog did not settle");
  };
  // Keep the specifiers literal: Bun 1.3.5 on Windows can treat a computed query import as a file after earlier suites.
  if (entryID === "template-settings") await import("../src/main.ts?dialog-focus-template-settings");
  else await import("../src/main.ts?dialog-focus-pinnable-settings");
  enterFirstSite(window);
  const $ = <T extends Element = HTMLElement>(selector: string) => window.document.querySelector<T>(selector)!;
  const entry = $<HTMLButtonElement>(`#${entryID}`), menu = $<HTMLDetailsElement>("#workspace-menu");
  const dialog = $<HTMLDialogElement>(`#${dialogID}`), close = $<HTMLButtonElement>(`#${closeID}`);
  await until(() => !entry.disabled);
  menu.open = true; entry.focus(); entry.click();
  // Loading disables the entry before its click bubbles; the user can still fold its menu while waiting.
  menu.open = false;
  expect(menu.open).toBe(false);
  expect(dialog.open).toBe(false);
  // happy-dom refuses blur() on the now-disabled entry; model focus leaving through the summary.
  const summary = menu.querySelector<HTMLElement>("summary")!; summary.focus(); summary.blur();
  expect(window.document.activeElement === window.document.body).toBe(true);
  await until(() => !!release); release!();
  await until(() => dialog.open && !close.disabled);
  close.focus(); close.click();
  expect(dialog.open).toBe(false);
  expect(window.document.activeElement === menu.querySelector("summary")).toBe(true);
});
