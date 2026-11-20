// Native controls keep keyboard focus, modality and form semantics in WebView2.
const baselines = new WeakMap<HTMLDialogElement, string>();
const dialogReturnTargets = new WeakMap<HTMLDialogElement, HTMLElement[]>();
const dialogCloseListeners = new WeakSet<HTMLDialogElement>();
const dialogStacks = new WeakMap<Document, HTMLDialogElement[]>();

function canReceiveFocus(element: HTMLElement): boolean {
  if (!element.isConnected || element.matches(':disabled, [aria-disabled="true"]')
    || element.closest('[hidden], [inert], [aria-hidden="true"]')) return false;
  const view = element.ownerDocument.defaultView!;
  for (let ancestor: HTMLElement | null = element; ancestor; ancestor = ancestor.parentElement) {
    if (ancestor.matches("dialog:not([open])")) return false;
    if (ancestor.matches("details:not([open])") && !ancestor.querySelector(":scope > summary")?.contains(element)) return false;
    const style = view.getComputedStyle(ancestor);
    if (style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse") return false;
  }
  return element.matches('button, input:not([type="hidden"]), select, textarea, a[href], summary, iframe, [tabindex], [contenteditable="true"]');
}

function restoreDialogFocus(dialog: HTMLDialogElement) {
  // A queued close event can arrive after the same dialog has been reopened.
  if (dialog.open) return;
  const targets = dialogReturnTargets.get(dialog);
  if (!targets) return;
  dialogReturnTargets.delete(dialog);
  const document = dialog.ownerDocument;
  const stack = (dialogStacks.get(document) ?? []).filter(entry => entry.isConnected && entry.open);
  dialogStacks.set(document, stack);
  const activeModal = document.activeElement?.closest<HTMLDialogElement>("dialog[open]")
    ?? stack.at(-1) ?? Array.from(document.querySelectorAll<HTMLDialogElement>("dialog[open]")).at(-1);
  // Return into a parent modal, but never take focus out of another open modal.
  const candidates = activeModal ? targets.filter(target => activeModal.contains(target))
    : [...targets, ...document.querySelectorAll<HTMLElement>("main[tabindex]")];
  for (const target of candidates) {
    if (!canReceiveFocus(target)) continue;
    target.focus({preventScroll: true});
    if (document.activeElement === target) return;
  }
}

function formSignature(dialog: HTMLDialogElement): string {
  return JSON.stringify(Array.from(dialog.querySelectorAll<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>("input,textarea,select"))
    .filter(field => field.name && !field.name.startsWith("ui-choice-"))
    .map(field => [field.name, field.type === "checkbox" || field.type === "radio" ? (field as HTMLInputElement).checked : field.value]));
}

export function rememberDialog(dialog: HTMLDialogElement) {
  baselines.set(dialog, formSignature(dialog));
}

export function hasDialogChanges(document: Document): boolean {
  return Array.from(document.querySelectorAll<HTMLDialogElement>("dialog[open]")).some(dialog =>
    !!dialog.querySelector("form") && baselines.get(dialog) !== formSignature(dialog));
}

export function openDialog(dialog: HTMLDialogElement, opener?: HTMLElement) {
  if (dialog.open) return;
  const document = dialog.ownerDocument;
  const trigger = opener ?? document.activeElement;
  const targets: HTMLElement[] = [];
  if (trigger instanceof document.defaultView!.HTMLElement) {
    targets.push(trigger);
    // The entry can disappear when its enclosing menu closes after this click.
    for (let menu = trigger.closest("details"); menu; menu = menu.parentElement?.closest("details") ?? null) {
      const summary = menu.querySelector<HTMLElement>(":scope > summary");
      if (summary) targets.push(summary);
    }
  }
  syncChoices(dialog);
  rememberDialog(dialog);
  dialog.showModal();
  dialogReturnTargets.set(dialog, targets);
  const stack = (dialogStacks.get(document) ?? []).filter(entry => entry !== dialog && entry.isConnected && entry.open);
  dialogStacks.set(document, [...stack, dialog]);
  if (!dialogCloseListeners.has(dialog)) {
    dialogCloseListeners.add(dialog);
    dialog.addEventListener("close", () => restoreDialogFocus(dialog));
  }
}

function mayDismiss(dialog: HTMLDialogElement): boolean {
  if (dialog.dataset.dismissWhileBusy !== "true" && dialog.querySelector('button[type="submit"]:disabled')) return false;
  return !dialog.querySelector("form") || baselines.get(dialog) === formSignature(dialog)
    || dialog.ownerDocument.defaultView!.confirm("有尚未保存的设置，确定放弃更改吗？");
}

export function prepareDialogs(document: Document) {
  for (const dialog of document.querySelectorAll<HTMLDialogElement>("dialog")) {
    const host = dialog.querySelector<HTMLFormElement>(":scope > form") ?? dialog;
    const footer = host.querySelector<HTMLElement>(":scope > .dialog-actions");
    const heading = host.querySelector<HTMLElement>(":scope > h2, :scope > .template-gallery-header, :scope > .ipfs-dialog-heading, :scope > .site-dialog-heading");
    if (!footer || !heading) continue;
    const header = document.createElement("div"); header.className = "dialog-header";
    const body = document.createElement("div"); body.className = "dialog-body";
    header.append(heading);
    for (const child of Array.from(host.childNodes)) if (child !== footer) body.append(child);
    host.classList.add("dialog-layout"); host.append(header, body, footer);
    const h2 = header.querySelector("h2");
    if (h2) { h2.id ||= `${dialog.id}-heading`; dialog.setAttribute("aria-labelledby", h2.id); }
    dialog.addEventListener("cancel", event => { if (!mayDismiss(dialog)) event.preventDefault(); });
    dialog.addEventListener("click", event => {
      const target = event.target as Element;
      const button = target.closest<HTMLButtonElement>("button");
      if (button?.type === "button" && /^(取消|关闭)$/.test(button.textContent?.trim() ?? "") && !mayDismiss(dialog)) {
        event.preventDefault(); event.stopImmediatePropagation();
      }
    }, true);
  }
}

// Preserve the existing select values/API while exposing small option sets inline.
export function prepareChoices(document: Document) {
  for (const select of document.querySelectorAll<HTMLSelectElement>("select")) {
    if (select.options.length < 2 || select.options.length > 4) continue;
    const original = select.closest("label");
    const name = original?.firstChild?.textContent?.trim() || "选择选项";
    if (original) {
      const fieldset = document.createElement("fieldset"), legend = document.createElement("legend");
      fieldset.id = original.id; fieldset.hidden = original.hidden;
      fieldset.className = `${original.className} choice-field`; legend.textContent = name;
      original.firstChild?.remove(); fieldset.append(legend, ...Array.from(original.childNodes));
      original.replaceWith(fieldset);
    }
    const group = document.createElement("span"); group.className = "choice-group";
    group.setAttribute("role", "radiogroup");
    group.setAttribute("aria-label", name);
    select.id ||= `choice-${select.form?.id}-${select.name}`;
    group.dataset.select = select.id;
    for (const option of Array.from(select.options)) {
      const label = document.createElement("label"), radio = document.createElement("input");
      radio.type = "radio"; radio.name = `ui-choice-${select.id}`; radio.value = option.value;
      radio.addEventListener("change", () => {
        if (!radio.checked || select.disabled) return;
        select.value = radio.value;
        select.dispatchEvent(new document.defaultView!.Event("change", {bubbles: true}));
      });
      label.append(radio, document.createTextNode(option.text)); group.append(label);
    }
    select.hidden = true; select.after(group);
    select.addEventListener("change", () => syncChoices(select.parentElement!));
    new document.defaultView!.MutationObserver(() => syncChoices(select.parentElement!)).observe(select, {attributes: true, attributeFilter: ["disabled"]});
  }
  syncChoices(document);
}

export function syncChoices(root: ParentNode) {
  for (const group of root.querySelectorAll<HTMLElement>(".choice-group")) {
    const select = group.ownerDocument.getElementById(group.dataset.select!) as HTMLSelectElement;
    for (const radio of group.querySelectorAll<HTMLInputElement>("input")) {
      radio.checked = radio.value === select.value; radio.disabled = select.disabled;
    }
  }
}

export const readingStyles = `
  :root{color-scheme:light dark}
  html::-webkit-scrollbar{width:6px;height:6px}
  html::-webkit-scrollbar-button{display:none}
  html::-webkit-scrollbar-track{background:transparent}
  html::-webkit-scrollbar-thumb{background:rgba(0,0,0,.16);border-radius:99px}
  html::-webkit-scrollbar-thumb:hover{background:rgba(0,0,0,.28)}
  *{box-sizing:border-box}
  body{max-width:740px;margin:0 auto;padding:8px 36px 48px;font:16px/1.85 -apple-system,BlinkMacSystemFont,'Segoe UI Variable Text','Segoe UI','Microsoft YaHei UI',sans-serif;color:#1d1d1f;background:#ffffff;overflow-wrap:anywhere;-webkit-font-smoothing:antialiased}
  p{margin:0 0 1.2em}h1,h2,h3{line-height:1.35;color:#1d1d1f}a{color:#007aff;text-decoration:none}a:hover{text-decoration:underline}
  img,video{max-width:100%;height:auto;border-radius:6px}
  pre{padding:16px;overflow:auto;border-radius:8px;background:#f5f5f7}
  code{font-family:Consolas,monospace;padding:.12em .35em;border-radius:4px;background:#f5f5f7;font-size:.9em}
  pre code{padding:0;border-radius:0;background:none;font-size:inherit}
  blockquote{margin-left:0;padding-left:18px;border-left:3px solid #d1d1d6;color:#515154}
  hr{border:0;border-top:1px solid #e2e2e5;margin:1.75em 0}
  table{display:block;max-width:100%;overflow:auto;border-collapse:collapse}
  th,td{padding:8px 12px;border:1px solid #e2e2e5;text-align:left}th{background:#f5f5f7;font-weight:600}
  iframe{max-width:100%}
  @media(prefers-color-scheme:dark){
    html::-webkit-scrollbar-thumb{background:rgba(255,255,255,.18)}
    html::-webkit-scrollbar-thumb:hover{background:rgba(255,255,255,.32)}
    body{color:#f5f5f7;background:#1c1c1e}
    h1,h2,h3{color:#f5f5f7}a{color:#0a84ff}
    pre,code,th{background:#2c2c2e}
    blockquote{border-left-color:#48484a;color:#a1a1a6}
    hr,th,td{border-color:#38383a}
  }
  @media(max-width:520px){body{padding-inline:24px}}
`;
