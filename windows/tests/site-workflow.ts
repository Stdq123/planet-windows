import type { Window } from "happy-dom";

// Site workflow tests explicitly enter a site now that startup opens Overview.
export function enterFirstSite(window: Window) {
  const choose = () => {
    const button = window.document.querySelector<HTMLButtonElement>("#sites button");
    if (!button) {
      if (window.document.querySelector<HTMLElement>("#library-notice")!.dataset.state !== "loading") observer.disconnect();
      return;
    }
    observer.disconnect(); button.click();
  };
  const observer = new window.MutationObserver(choose);
  observer.observe(window.document.querySelector("#sites")!, { childList: true });
  observer.observe(window.document.querySelector("#library-notice")!, { attributes: true });
  choose();
}
