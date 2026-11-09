import { expect, test } from "bun:test";
import { Window } from "happy-dom";

async function styledDocument() {
  const window = new Window({ url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  // Keep the real stylesheet order so a later override cannot hide a focus ring.
  for (const link of window.document.querySelectorAll('link[rel="stylesheet"]')) {
    const style = window.document.createElement("style");
    style.textContent = await Bun.file(link.getAttribute("href")!).text();
    link.replaceWith(style);
  }
  window.document.querySelector<HTMLElement>("#editor")!.hidden = false;
  return window;
}

test("title and body retain the two-pixel keyboard focus ring through the full CSS cascade", async () => {
  const window = await styledDocument();
  for (const selector of ["#title", "#content"]) {
    const field = window.document.querySelector<HTMLElement>(selector)!;
    field.focus();
    const style = window.getComputedStyle(field);
    expect(style.outlineStyle).toBe("solid");
    expect(style.outlineWidth).toBe("2px");
    expect(style.outlineOffset).toBe("2px");
  }
  // These immediate wrappers otherwise cut off all four pixels outside the textarea.
  for (const selector of [".writing-grid", ".writing-pane"]) {
    expect(window.getComputedStyle(window.document.querySelector(selector)!).overflow).toBe("visible");
  }
  await window.happyDOM.close();
});

test("skip link becomes visible on focus and main targets retain a visible indicator", async () => {
  const unfocused = await styledDocument();
  expect(unfocused.getComputedStyle(unfocused.document.querySelector("#skip-to-main")!).clipPath).toBe("inset(50%)");
  await unfocused.happyDOM.close();
  // A fresh document avoids happy-dom's computed-style cache across focus changes.
  const window = await styledDocument();
  const skip = window.document.querySelector<HTMLElement>("#skip-to-main")!;
  skip.focus();
  expect(window.getComputedStyle(skip).clipPath).not.toBe("inset(50%)");
  expect(window.getComputedStyle(skip).outlineWidth).toBe("2px");
  for (const selector of ["#dashboard", "#workspace-main"]) {
    const target = window.document.querySelector<HTMLElement>(selector)!;
    target.hidden = false; target.focus();
    expect(window.getComputedStyle(target).outlineStyle).toBe("solid");
    expect(window.getComputedStyle(target).outlineWidth).toBe("2px");
  }
  await window.happyDOM.close();
});
