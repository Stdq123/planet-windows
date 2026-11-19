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

test("legacy aliases resolve to the original Mac Planet roles through the full cascade", async () => {
  const window = await styledDocument();
  expect(resolved(window, "color", "--text-secondary")).toBe("#515154");
  expect(resolved(window, "color", "--ink")).toBe("#1d1d1f");
  expect(resolved(window, "color", "--muted")).toBe("#86868b");
  expect(resolved(window, "background-color", "--selection")).toBe("#e8f2ff");
  expect(resolved(window, "background-color", "--hover")).toBe("rgba(0, 0, 0, 0.05)");
  expect(resolved(window, "background-color", "--brand")).toBe("#007aff");
  expect(resolved(window, "color", "--focus")).toBe("#007aff");
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

test("components read the converged scale instead of off-ladder literals", async () => {
  const window = await styledDocument();
  const document = window.document;
  // Former 9px eyebrow sits on the xs floor.
  expect(window.getComputedStyle(document.querySelector(".eyebrow")!).fontSize).toBe("11px");
  // Former 9px menu corner converges to the lg step.
  expect(window.getComputedStyle(document.querySelector(".workspace-menu-content")!).borderRadius).toBe("8px");
  // Module gaps read the spacing scale; micro control gaps stay literal.
  expect(window.getComputedStyle(document.querySelector(".dashboard-columns")!).gap).toBe("16px");
  expect(window.getComputedStyle(document.querySelector(".view-switch")!).gap).toBe("2px");
  expect(window.getComputedStyle(document.querySelector(".posts-header h1")!).fontWeight).toBe("600");
  await window.happyDOM.close();
});