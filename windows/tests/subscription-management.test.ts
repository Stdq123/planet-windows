import { enterFirstSite } from "./site-workflow";
import { expect, test } from "bun:test";
import { Window } from "happy-dom";

async function waitFor(predicate: () => boolean) {
  for (let i = 0; i < 150; i++) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("Timed out waiting for subscription management UI");
}

test("subscriptions can be edited (title and URL) and deleted from the subscription dialog", async () => {
  const window = new Window({ url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, { window, document: window.document });
  window.confirm = () => true;

  const now = new Date().toISOString();
  let subs = [
    {
      url: "https://old.example/rss.xml",
      lastUpdated: now,
      feed: {
        title: "Old Feed",
        description: "",
        link: "https://old.example/",
        items: [{ id: "1", key: "1", title: "Hello", link: "https://old.example/1", description: "Body", content: "Body", published: now, read: false }],
      },
    },
  ];
  const calls: Array<{ method: string; args: unknown[] }> = [];
  (globalThis as any).mygo = {
    call: async (method: string, ...args: unknown[]) => {
      calls.push({ method, args });
      switch (method) {
        case "Library.Sites":
          return [];
        case "Library.Subscriptions":
          return structuredClone(subs);
        case "Library.UpdateSubscription": {
          const [oldURL, newURL, customTitle] = args as [string, string, string];
          subs = subs.map((s) =>
            s.url === oldURL ? { ...s, url: newURL, customTitle: customTitle.trim() } : s,
          );
          return structuredClone(subs[0]);
        }
        case "Library.Unfollow": {
          const [address] = args as [string];
          subs = subs.filter((s) => s.url !== address);
          return;
        }
        default:
          throw new Error(`Unexpected call: ${method}`);
      }
    },
  };

  await import("../src/main.ts?subscription-management");
  enterFirstSite(window);
  await waitFor(() => window.document.querySelectorAll("#subscriptions .site-item").length === 1);

  // Select subscription and verify edit button becomes visible.
  window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();
  const editButton = window.document.querySelector<HTMLButtonElement>("#edit-subscription")!;
  await waitFor(() => !editButton.hidden && !editButton.disabled);

  // Open edit dialog and verify pre-filled values and delete button.
  editButton.click();
  const dialog = window.document.querySelector<HTMLDialogElement>("#feed-dialog")!;
  expect(dialog.open).toBe(true);
  expect(window.document.querySelector("#feed-dialog-title")!.textContent).toBe("编辑订阅");
  expect(window.document.querySelector<HTMLElement>("#feed-title-field")!.hidden).toBe(false);
  expect(window.document.querySelector<HTMLButtonElement>("#delete-feed")!.hidden).toBe(false);

  const titleInput = window.document.querySelector<HTMLInputElement>('#feed-form [name="title"]')!;
  const urlInput = window.document.querySelector<HTMLInputElement>('#feed-form [name="url"]')!;
  expect(urlInput.value).toBe("https://old.example/rss.xml");

  titleInput.value = "自定义博客名";
  urlInput.value = "https://new.example/feed.xml";
  window.document.querySelector<HTMLFormElement>("#feed-form")!.dispatchEvent(new window.Event("submit", { cancelable: true }));
  await waitFor(() => !dialog.open);

  expect(calls.some((c) => c.method === "Library.UpdateSubscription" && c.args[0] === "https://old.example/rss.xml" && c.args[1] === "https://new.example/feed.xml" && c.args[2] === "自定义博客名")).toBe(true);
  expect(window.document.querySelector("#subscriptions .site-name")!.textContent).toBe("自定义博客名");
  expect(window.document.querySelector("#site-title")!.textContent).toBe("自定义博客名");

  // Reopen edit dialog and delete the subscription.
  editButton.click();
  await waitFor(() => dialog.open);
  window.document.querySelector<HTMLButtonElement>("#delete-feed")!.click();
  await waitFor(() => !dialog.open && window.document.querySelectorAll("#subscriptions .site-item").length === 0);
  expect(calls.some((c) => c.method === "Library.Unfollow" && c.args[0] === "https://new.example/feed.xml")).toBe(true);

  // Opening add dialog hides title field and delete button.
  window.document.querySelector<HTMLButtonElement>("#new-feed")!.click();
  expect(dialog.open).toBe(true);
  expect(window.document.querySelector("#feed-dialog-title")!.textContent).toBe("添加订阅");
  expect(window.document.querySelector<HTMLElement>("#feed-title-field")!.hidden).toBe(true);
  expect(window.document.querySelector<HTMLButtonElement>("#delete-feed")!.hidden).toBe(true);
});
