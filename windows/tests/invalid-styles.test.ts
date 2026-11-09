import { expect, test } from "bun:test";
import { Window } from "happy-dom";
import { createFormErrors } from "../src/form-errors";
import { prepareChoices } from "../src/ui";

async function styledDocument() {
  const window = new Window({ url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  // Keep the real stylesheet order so a later override cannot hide the invalid boundary.
  for (const link of window.document.querySelectorAll('link[rel="stylesheet"]')) {
    const style = window.document.createElement("style");
    style.textContent = await Bun.file(link.getAttribute("href")!).text();
    link.replaceWith(style);
  }
  prepareChoices(window.document as unknown as Document);
  return window;
}

function markInvalid(window: Window, formSelector: string, resultSelector: string, fieldName: string) {
  const form = window.document.querySelector<HTMLFormElement>(formSelector)!;
  const result = window.document.querySelector<HTMLElement>(resultSelector)!;
  const field = form.querySelector<HTMLInputElement | HTMLSelectElement>(`[name="${fieldName}"]`)!;
  const errors = createFormErrors(form as unknown as HTMLFormElement, result as unknown as HTMLElement);
  errors.show([{ field: field as unknown as HTMLInputElement, message: "请更正此项" }]);
  return { form, field };
}

function expectDangerBorder(window: Window, element: Element) {
  const color = window.getComputedStyle(element).borderTopColor;
  expect(["#a13f45", "rgb(161, 63, 69)"]).toContain(color);
}

test("invalid site, article and X sync fields keep a danger boundary through the full CSS cascade", async () => {
  const window = await styledDocument();
  const site = markInvalid(window, "#site-form", "#site-settings-result", "name");
  expectDangerBorder(window, site.field);
  const article = markInvalid(window, "#article-settings-form", "#article-settings-result", "weight");
  expectDangerBorder(window, article.field);
  const xSync = markInvalid(window, "#x-sync-form", "#x-sync-result", "apiKey");
  expectDangerBorder(window, xSync.field);
  await window.happyDOM.close();
});

test("a converted select exposes the invalid boundary on its visible radio group", async () => {
  const window = await styledDocument();
  const { form } = markInvalid(window, "#article-settings-form", "#article-settings-result", "articleType");
  const group = form.querySelector<HTMLElement>('.choice-group[aria-invalid="true"]')!;
  expect(group).not.toBeNull();
  for (const label of group.querySelectorAll("label")) expectDangerBorder(window, label);
  await window.happyDOM.close();
});

test("clearing the error restores the normal control boundary", async () => {
  const window = await styledDocument();
  const form = window.document.querySelector<HTMLFormElement>("#site-form")!;
  const result = window.document.querySelector<HTMLElement>("#site-settings-result")!;
  const field = form.querySelector<HTMLInputElement>('[name="name"]')!;
  const errors = createFormErrors(form as unknown as HTMLFormElement, result as unknown as HTMLElement);
  errors.show([{ field: field as unknown as HTMLInputElement, message: "请更正此项" }]);
  expectDangerBorder(window, field);
  errors.clear();
  const color = window.getComputedStyle(field).borderTopColor;
  expect(["#857a6e", "rgb(133, 122, 110)"]).toContain(color);
  await window.happyDOM.close();
});