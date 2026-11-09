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

test("legacy shell surfaces resolve to graphite roles through the full cascade", async () => {
  const window = await styledDocument();
  const document = window.document;
  const color = (selector: string) => window.getComputedStyle(document.querySelector(selector)!).color;
  const background = (selector: string) => window.getComputedStyle(document.querySelector(selector)!).backgroundColor;
  expect(color("#ipfs-open-site")).toBe("#5f5c58");
  expect(color(".ipfs-stats dt")).toBe("#6d6a65");
  expect(color(".reader h2")).toBe("#2b2b29");
  expect(color(".site-address")).toBe("#6d6a65");
  expect(color(".empty p")).toBe("#6d6a65");
  expect(color(".workspace-menu-content button")).toBe("#2b2b29");
  expect(color(".avatar-preview")).toBe("#5f5c58");
  expect(background(".avatar-preview")).toBe("#e9e7e2");
  const tagInput = document.querySelector(".tag-bar input")!;
  expect(window.getComputedStyle(tagInput).borderTopColor).toBe("#857a6e");
  await window.happyDOM.close();
});