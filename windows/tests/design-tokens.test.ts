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

function resolved(window: Window, property: string, token: string) {
  const probe = window.document.createElement("div");
  probe.style.setProperty(property, `var(${token})`);
  window.document.body.append(probe);
  return window.getComputedStyle(probe).getPropertyValue(property);
}

test("legacy aliases resolve to the graphite roles through the full cascade", async () => {
  const window = await styledDocument();
  expect(resolved(window, "color", "--text-secondary")).toBe("#5f5c58");
  expect(resolved(window, "color", "--ink")).toBe("#2b2b29");
  expect(resolved(window, "color", "--muted")).toBe("#6d6a65");
  expect(resolved(window, "background-color", "--selection")).toBe("#e9e7e2");
  expect(resolved(window, "background-color", "--hover")).toBe("#eeece7");
  expect(resolved(window, "background-color", "--brand")).toBe("#333432");
  expect(resolved(window, "color", "--focus")).toBe("#333432");
  await window.happyDOM.close();
});

test("the converged scales expose every promised step", async () => {
  const window = await styledDocument();
  const sizes: [string, string][] = [
    ["--text-xs", "11px"], ["--text-sm", "12px"], ["--text-base", "13px"], ["--text-md", "14px"],
    ["--text-lg", "16px"], ["--text-xl", "20px"], ["--text-2xl", "24px"], ["--text-3xl", "28px"],
  ];
  for (const [token, value] of sizes) expect(resolved(window, "font-size", token)).toBe(value);
  const radii: [string, string][] = [
    ["--radius-sm", "4px"], ["--radius-md", "6px"], ["--radius-lg", "8px"], ["--radius-xl", "10px"], ["--radius-dialog", "16px"],
  ];
  for (const [token, value] of radii) expect(resolved(window, "border-radius", token)).toBe(value);
  const spaces: [string, string][] = [
    ["--space-1", "4px"], ["--space-2", "8px"], ["--space-3", "12px"], ["--space-4", "16px"], ["--space-6", "24px"], ["--space-8", "32px"],
  ];
  for (const [token, value] of spaces) expect(resolved(window, "padding-left", token)).toBe(value);
  expect(resolved(window, "transition-duration", "--duration-fast")).toBe("120ms");
  await window.happyDOM.close();
});