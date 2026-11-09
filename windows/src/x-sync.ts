import { Library, type XSyncInfo, type XSyncPostInfo } from "./mygo";
import { openDialog, rememberDialog } from "./ui";
import { createFormErrors } from "./form-errors";

type XSyncContext = {
  getSiteID: () => string | null;
  isBusy: () => boolean;
  setBusy: (busy: boolean) => void;
};

export function initXSync(context: XSyncContext) {
  const doc = document;
  const el = <T extends HTMLElement>(selector: string) => doc.querySelector<T>(selector)!;
  const open = el<HTMLButtonElement>("#x-sync-settings"), dialog = el<HTMLDialogElement>("#x-sync-dialog");
  const form = el<HTMLFormElement>("#x-sync-form"), result = el("#x-sync-result"), posts = el("#x-sync-posts");
  const formErrors = createFormErrors(form, result);
  const save = form.querySelector<HTMLButtonElement>('[type="submit"]')!;
  const disable = el<HTMLButtonElement>("#x-sync-disable"), close = el<HTMLButtonElement>("#x-sync-close");
  // This component guards active operations itself; a failed load must remain dismissible.
  dialog.dataset.dismissWhileBusy = "true";
  const secrets = ["apiKey", "apiSecret", "accessToken", "accessTokenSecret"].map(name => form.elements.namedItem(name) as HTMLInputElement);
  const auto = form.elements.namedItem("autoSync") as HTMLInputElement;
  let siteID = "", busy = false, loadFailed = false, revision = 0, baseline: XSyncInfo | null = null;
  const dirty = () => secrets.some(input => !!input.value.trim()) || auto.checked !== !!baseline?.autoSync;
  const current = () => !!siteID && context.getSiteID() === siteID && dialog.open;
  const locked = () => busy || context.isBusy();
  const clearSecrets = () => { for (const input of secrets) input.value = ""; };
  const canSync = (post: XSyncPostInfo) => current() && !locked() && !!baseline?.configured && !dirty() && post.canSync && ["", "pending", "failed"].includes(post.status);

  function update() {
    open.hidden = !context.getSiteID(); open.disabled = !context.getSiteID() || locked();
    if (dialog.open && siteID && context.getSiteID() !== siteID) dialog.close();
    for (const input of [...secrets, auto]) input.disabled = locked() || !baseline;
    save.disabled = locked() || !baseline;
    close.disabled = locked(); disable.disabled = locked() || (!baseline?.configured && !loadFailed);
    for (const button of posts.querySelectorAll<HTMLButtonElement>("button[data-sync-article]")) {
      const post = baseline?.posts?.find(entry => entry.articleId === button.dataset.syncArticle);
      button.disabled = !post || !canSync(post);
      button.title = post?.status === "sent" ? "已同步的文章不会再次发布" : dirty() ? "请先保存配置" : !baseline?.configured ? "请先保存 X 凭据" : !post?.canSync ? "请先发布当前站点内容；独立页面不参与同步" : "";
    }
  }

  function renderPosts() {
    posts.replaceChildren();
    if (!baseline?.posts?.length) { const empty = doc.createElement("p"); empty.textContent = "暂无文章。保存并发布文章后可在这里同步。"; posts.append(empty); return; }
    for (const post of baseline.posts) {
      const row = doc.createElement("div"), info = doc.createElement("div"), title = doc.createElement("span"), status = doc.createElement("span"), actions = doc.createElement("div");
      row.className = "removed-site"; row.dataset.articleId = post.articleId; row.setAttribute("role", "listitem");
      info.className = "removed-site-info"; title.className = "site-name"; title.textContent = post.title || "无标题"; title.title = post.title || "无标题";
      status.className = "removed-site-status";
      status.textContent = post.status === "sent" ? "已同步" : post.status === "pending" ? "结果未确认，重试前请检查 X" : post.status === "failed" ? `同步失败${post.error ? `：${post.error}` : ""}` : post.canSync ? "未同步" : "暂不可同步，请先发布当前内容，独立页面不参与同步";
      info.append(title, status); actions.className = "removed-site-actions";
      if (/^https:\/\/x\.com\/i\/web\/status\/[0-9]+$/.test(post.url || "")) {
        const link = doc.createElement("a"); link.href = post.url; link.target = "_blank"; link.rel = "noopener noreferrer"; link.className = "secondary"; link.textContent = "查看"; link.setAttribute("aria-label", `在 X 查看 ${post.title || "无标题"}`); actions.append(link);
      }
      const button = doc.createElement("button"); button.type = "button"; button.className = "secondary"; button.dataset.syncArticle = post.articleId;
      button.textContent = post.status === "sent" ? "已同步" : post.status === "pending" || post.status === "failed" ? "重试" : "同步";
      button.setAttribute("aria-label", `${button.textContent} ${post.title || "无标题"}`);
      button.addEventListener("click", () => void sync(post)); actions.append(button); row.append(info, actions); posts.append(row);
    }
  }

  async function operation(action: (id: string, valid: () => boolean) => Promise<void>) {
    if (!current() || locked()) return;
    const id = siteID, request = ++revision;
    const valid = () => request === revision && current() && siteID === id;
    busy = true; context.setBusy(true); update();
    try { await action(id, valid); }
    catch (error) { if (valid()) result.textContent = `操作失败：${error instanceof Error ? error.message : String(error)}`; }
    finally { busy = false; context.setBusy(false); update(); formErrors.focusFirst(); }
  }

  async function sync(post: XSyncPostInfo) {
    if (!canSync(post)) return;
    const retry = post.status === "pending" || post.status === "failed";
    if (retry && !doc.defaultView!.confirm("请先在 X 检查这篇文章是否已发出。确认没有发出后再重试，以免重复发布。继续重试吗？")) return;
    await operation(async (id, valid) => {
      result.textContent = "正在同步到 X…";
      try {
        const updated = await Library.syncXArticle(id, post.articleId, retry);
        if (!valid() || !baseline) return;
        baseline.posts = baseline.posts.map(entry => entry.articleId === updated.articleId ? updated : entry);
        renderPosts(); result.textContent = updated.status === "sent" ? "已同步到 X。" : updated.error || "同步结果尚未确认，请先检查 X。";
      } catch (error) {
        // A request can fail after the server accepts it; always show the persisted status.
        try { const info = await Library.xSyncInfo(id); if (valid()) { baseline = info; renderPosts(); } } catch { /* Keep the last known status and the original error. */ }
        throw error;
      }
    });
  }

  async function refreshSaved(id: string, valid: () => boolean, message: string) {
    result.textContent = message;
    try {
      const info = await Library.xSyncInfo(id); if (!valid()) return;
      baseline = info; auto.checked = info.autoSync; renderPosts(); rememberDialog(dialog);
    } catch {
      if (valid()) result.textContent = `${message} 文章状态读取失败，请关闭后重新打开。`;
    }
  }

  open.addEventListener("click", () => {
    const id = context.getSiteID(); if (!id || locked() || dialog.open) return;
    siteID = id; baseline = null; loadFailed = false; form.reset(); clearSecrets(); posts.replaceChildren(); result.textContent = "正在读取 X 同步设置…"; openDialog(dialog);
    void operation(async (id, valid) => {
      let info: XSyncInfo;
      try { info = await Library.xSyncInfo(id); }
      catch (error) { if (valid()) loadFailed = true; throw error; }
      if (!valid()) return;
      baseline = info; auto.checked = info.autoSync; renderPosts(); rememberDialog(dialog);
      result.textContent = info.configured ? "凭据已保存。历史文章可在下方手动同步。" : "尚未配置 X 同步。";
    });
  });
  for (const event of ["input", "change"]) form.addEventListener(event, update);
  form.addEventListener("submit", event => {
    event.preventDefault(); if (!current() || locked() || !baseline) return;
    result.textContent = ""; if (!formErrors.validate()) return;
    const values = secrets.map(input => input.value.trim());
    if (values.some(Boolean) && !values.every(Boolean) || !baseline.configured && !values.every(Boolean)) {
      formErrors.show(secrets.filter((_, index) => !values[index]).map(field => ({field, message: "请填写四项凭据；已有配置时可全部留空以保留原凭据。"}))); return;
    }
    const invalid = secrets.filter((_, index) => !!values[index] && !/^[!-~]{1,4096}$/.test(values[index]!));
    if (invalid.length) { formErrors.show(invalid.map(field => ({field, message: "X 凭据只能包含不带空格的可打印 ASCII 字符，且不能超过 4096 个字符。"}))); return; }
    const automatic = auto.checked;
    void operation(async (id, valid) => {
      result.textContent = "正在保存 X 同步配置…";
      try { await Library.configureXSync(id, values[0]!, values[1]!, values[2]!, values[3]!, automatic); }
      catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        if (valid() && ["请填写 X 的四项凭据", "请完整填写 X 的四项凭据"].includes(message)) {
          const missing = secrets.filter((_, index) => !values[index]);
          if (missing.length) formErrors.show(missing.map(field => ({field, message: `操作失败：${message}`})));
        }
        throw error;
      }
      clearSecrets(); if (!valid() || !baseline) return;
      baseline.configured = true; baseline.autoSync = automatic; rememberDialog(dialog);
      await refreshSaved(id, valid, automatic ? "已保存。仅启用后新建的文章会在发布成功后自动同步；历史文章需手动同步。" : "已保存。可在下方手动同步文章。");
    });
  });
  disable.addEventListener("click", () => {
    if (!current() || locked() || (!baseline?.configured && !loadFailed)) return;
    if (!doc.defaultView!.confirm("停用 X 同步并删除本机保存的凭据？已发送记录会保留，已同步的文章不会重复发布。")) return;
    void operation(async (id, valid) => {
      await Library.disableXSync(id); clearSecrets(); if (!valid()) return;
      formErrors.clear();
      // Stopping does not depend on readable article records; keep this result if refresh fails.
      loadFailed = false;
      if (baseline) { baseline.configured = false; baseline.autoSync = false; }
      auto.checked = false; rememberDialog(dialog);
      await refreshSaved(id, valid, "已停用并删除凭据，已发送记录保留。");
    });
  });
  close.addEventListener("click", () => { if (!locked()) dialog.close(); });
  dialog.addEventListener("cancel", event => { if (locked()) { event.preventDefault(); event.stopImmediatePropagation(); } }, true);
  dialog.addEventListener("close", () => { revision++; siteID = ""; baseline = null; loadFailed = false; clearSecrets(); form.reset(); posts.replaceChildren(); result.textContent = ""; });
  update();
  return {update};
}
