import { enterFirstSite } from "./site-workflow";
import { expect, test } from "bun:test";
import { Window } from "happy-dom";

async function waitFor(predicate: () => boolean) {
  for (let i = 0; i < 150; i++) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("Timed out waiting for subscription icons");
}

test("subscribed sites display site icon or default website icon without letter initials", async () => {
  const window = new Window({ url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, { window, document: window.document });

  const iconBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
  const now = new Date().toISOString();
  const subs = [
    {
      url: "https://with-icon.example/feed",
      lastUpdated: now,
      feed: {
        title: "Alpha Blog",
        description: "",
        link: "https://with-icon.example/",
        iconPng: iconBase64,
        items: [{ id: "1", key: "1", title: "Post A", link: "https://with-icon.example/1", description: "Body", content: "Body", published: now, read: false, starred: true }],
      },
    },
    {
      url: "https://no-icon.example/feed",
      lastUpdated: now,
      feed: {
        title: "Beta News",
        description: "",
        link: "https://no-icon.example/",
        items: [{ id: "2", key: "2", title: "Post B", link: "https://no-icon.example/2", description: "Body", content: "Body", published: now, read: false, starred: true }],
      },
    },
  ];
  (globalThis as any).mygo = {
    call: async (method: string) => {
      switch (method) {
        case "Library.Sites":
          return [];
        case "Library.Subscriptions":
          return subs;
        case "Library.SetSubscriptionRead":
          return;
        default:
          throw new Error(`Unexpected call: ${method}`);
      }
    },
  };

  await import("../src/main.ts?subscription-icons");
  enterFirstSite(window);
  await waitFor(() => window.document.querySelectorAll("#subscriptions .site-item").length === 2);

  const items = window.document.querySelectorAll<HTMLButtonElement>("#subscriptions .site-item");
  const withIconImg = items[0]!.querySelector<HTMLImageElement>("img.site-icon.subscription-icon");
  expect(withIconImg).not.toBeNull();
  expect(withIconImg!.src).toBe(`data:image/png;base64,${iconBase64}`);

  const fallbackSlot = items[1]!.querySelector<HTMLElement>(".site-icon.subscription-icon-fallback");
  expect(fallbackSlot).not.toBeNull();
  expect(fallbackSlot!.textContent?.trim()).toBe("");
  const fallbackGlyph = fallbackSlot!.querySelector<HTMLImageElement>("img.subscription-fallback-glyph");
  expect(fallbackGlyph).not.toBeNull();
  expect(fallbackGlyph!.src).toContain("globe.svg");

  // Open the subscription without an icon and verify article list and reader never show letter initials.
  items[1]!.click();
  await waitFor(() => !!window.document.querySelector("#articles .article-item"));
  const rowIcon = window.document.querySelector<HTMLElement>("#articles .article-source-icon.subscription-icon-fallback");
  expect(rowIcon).not.toBeNull();
  expect(rowIcon!.textContent?.trim()).toBe("");
  expect(rowIcon!.querySelector("img.subscription-fallback-glyph")).not.toBeNull();

  window.document.querySelector<HTMLButtonElement>("#articles .article-item")!.click();
  await waitFor(() => !window.document.querySelector<HTMLElement>("#reader")!.hidden);
  for (const selector of ["#reader .reader-source-icon", "#reader .reader-author-avatar"]) {
    const slot = window.document.querySelector<HTMLElement>(selector)!;
    expect(slot.textContent?.trim()).toBe("");
    expect(slot.querySelector<HTMLImageElement>("img")?.src).toContain("globe.svg");
  }

  // Open the subscription with an icon and verify article list and reader render the fetched site icon.
  items[0]!.click();
  await waitFor(() => window.document.querySelector("#site-title")?.textContent === "Alpha Blog");
  const rowSiteIcon = window.document.querySelector<HTMLImageElement>("#articles img.article-source-icon.subscription-icon");
  expect(rowSiteIcon).not.toBeNull();
  expect(rowSiteIcon!.src).toBe(`data:image/png;base64,${iconBase64}`);

  window.document.querySelector<HTMLButtonElement>("#articles .article-item")!.click();
  await waitFor(() => window.document.querySelector("#reader-source")?.textContent === "Alpha Blog");
  for (const selector of ["#reader .reader-source-icon", "#reader .reader-author-avatar"]) {
    const slot = window.document.querySelector<HTMLElement>(selector)!;
    expect(slot.textContent?.trim()).toBe("");
    expect(slot.querySelector<HTMLImageElement>("img")?.src).toBe(`data:image/png;base64,${iconBase64}`);
  }
});
