import { expect, test } from "bun:test";
import { Window } from "happy-dom";

async function styledDashboard(width: number) {
  const window = new Window({ width, height: 900, url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  // Test the active media rules through the same cascade as the application.
  for (const link of window.document.querySelectorAll('link[rel="stylesheet"]')) {
    const style = window.document.createElement("style");
    style.textContent = await Bun.file(link.getAttribute("href")!).text();
    link.replaceWith(style);
  }
  window.document.querySelector(".shell")!.classList.add("dashboard-mode");
  window.document.querySelector<HTMLElement>("#dashboard")!.hidden = false;
  window.document.querySelector<HTMLElement>("#dashboard-content")!.hidden = false;
  return window;
}

for (const width of [860, 1024, 1100, 1280]) {
  test(`dashboard keeps content and keyboard source order at ${width}px`, async () => {
    const window = await styledDashboard(width);
    try {
      const document = window.document;
      expect(window.matchMedia("(max-width: 1100px)").matches).toBe(width <= 1100);
      const blocks = ["#dashboard-drafts", "#dashboard-articles", "#dashboard-sites", ".dashboard-analytics"]
        .map(selector => document.querySelector(selector)!);
      for (let index = 1; index < blocks.length; index++) {
        expect(blocks[index - 1].compareDocumentPosition(blocks[index]) & window.Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
      }
      // happy-dom does not lay out pixels or send Tab. Check that CSS leaves
      // these source-ordered groups in normal grid placement at each width.
      for (const selector of [".dashboard-primary", ".dashboard-secondary", ".dashboard-sites-section", ".dashboard-analytics"]) {
        const style = window.getComputedStyle(document.querySelector(selector)!);
        expect(["", "auto"]).toContain(style.gridRow);
        expect(["", "auto"]).toContain(style.gridColumn);
        expect(["", "0"]).toContain(style.order);
      }
      expect(window.getComputedStyle(document.querySelector(".dashboard-secondary")!).display).toBe("grid");
      expect(window.getComputedStyle(document.querySelector(".dashboard-sites-section .dashboard-section-heading")!).display).toBe("flex");
      const create = document.querySelector<HTMLButtonElement>("#dashboard-create")!;
      expect(create.hidden).toBe(false);
      expect(create.disabled).toBe(false);
      expect(window.getComputedStyle(create).display).not.toBe("none");
      expect(create.tabIndex).toBe(0);
      expect([...document.querySelectorAll("#dashboard [tabindex]")].some(element => Number(element.getAttribute("tabindex")) > 0)).toBe(false);
    } finally {
      await window.happyDOM.close();
    }
  });
}
