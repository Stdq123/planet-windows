import { expect, test } from "bun:test";
import { Window } from "happy-dom";

async function styledDocument() {
  const window = new Window({ url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  for (const link of window.document.querySelectorAll('link[rel="stylesheet"]')) {
    const style = window.document.createElement("style");
    style.textContent = await Bun.file(link.getAttribute("href")!).text();
    link.replaceWith(style);
  }
  return window;
}

test("legacy shell surfaces resolve to original Mac Planet roles through the full cascade", async () => {
  const window = await styledDocument();
  const document = window.document;
  const color = (selector: string) => window.getComputedStyle(document.querySelector(selector)!).color;
  const background = (selector: string) => window.getComputedStyle(document.querySelector(selector)!).backgroundColor;
  expect(color("#ipfs-open-site")).toBe("#515154");
  expect(color(".ipfs-stats dt")).toBe("#86868b");
  expect(color(".reader h2")).toBe("#1d1d1f");
  expect(color(".site-address")).toBe("#86868b");
  expect(color(".empty p")).toBe("#86868b");
  expect(color(".workspace-menu-content button:not(:disabled)")).toBe("#1d1d1f");
  expect(color(".avatar-preview")).toBe("#515154");
  expect(background(".avatar-preview")).toBe("#e8f2ff");
  const tagInput = document.querySelector(".tag-bar input")!;
  expect(window.getComputedStyle(tagInput).borderTopColor).toBe("#c7c7cc");
  await window.happyDOM.close();
});