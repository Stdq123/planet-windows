// Native controls keep keyboard focus, modality and form semantics in WebView2.
const baselines = new WeakMap<HTMLDialogElement, string>();

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

export function openDialog(dialog: HTMLDialogElement) {
  syncChoices(dialog);
  rememberDialog(dialog);
  dialog.showModal();
  positionDialog(dialog);
}

function positionDialog(dialog: HTMLDialogElement) {
  if (!dialog.open) return;
  const view = dialog.ownerDocument.defaultView!, height = dialog.getBoundingClientRect().height;
  if (!height) return;
  const top = Math.max(24, Math.min(view.innerHeight - height - 24, view.innerHeight * .382 - height / 2));
  dialog.style.marginTop = `${top}px`;
}

function mayDismiss(dialog: HTMLDialogElement): boolean {
  if (dialog.dataset.dismissWhileBusy !== "true" && dialog.querySelector('button[type="submit"]:disabled')) return false;
  return !dialog.querySelector("form") || baselines.get(dialog) === formSignature(dialog)
    || dialog.ownerDocument.defaultView!.confirm("有尚未保存的设置，确定放弃更改吗？");
}

export function prepareDialogs(document: Document) {
  const observer = new document.defaultView!.ResizeObserver(entries => {
    for (const entry of entries) positionDialog(entry.target as HTMLDialogElement);
  });
  for (const dialog of document.querySelectorAll<HTMLDialogElement>("dialog")) {
    const host = dialog.querySelector<HTMLFormElement>(":scope > form") ?? dialog;
    const footer = host.querySelector<HTMLElement>(":scope > .dialog-actions");
    const heading = host.querySelector<HTMLElement>(":scope > h2, :scope > .template-gallery-header, :scope > .ipfs-dialog-heading");
    if (!footer || !heading) continue;
    const header = document.createElement("div"); header.className = "dialog-header";
    const body = document.createElement("div"); body.className = "dialog-body";
    header.append(heading);
    for (const child of Array.from(host.childNodes)) if (child !== footer) body.append(child);
    host.classList.add("dialog-layout"); host.append(header, body, footer);
    observer.observe(dialog);
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
  document.defaultView!.addEventListener("resize", () => {
    for (const dialog of document.querySelectorAll<HTMLDialogElement>("dialog[open]")) positionDialog(dialog);
  });
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

function syncChoices(root: ParentNode) {
  for (const group of root.querySelectorAll<HTMLElement>(".choice-group")) {
    const select = group.ownerDocument.getElementById(group.dataset.select!) as HTMLSelectElement;
    for (const radio of group.querySelectorAll<HTMLInputElement>("input")) {
      radio.checked = radio.value === select.value; radio.disabled = select.disabled;
    }
  }
}

export const readingStyles = `
  *{box-sizing:border-box}
  body{max-width:800px;margin:0 auto;padding:8px 32px 40px;font:16px/1.85 'Segoe UI Variable','Segoe UI','Microsoft YaHei UI',sans-serif;color:#253344;overflow-wrap:anywhere}
  p{margin:0 0 1.2em}h1,h2,h3{line-height:1.4;color:#172435}a{color:#176bc0}
  img,video{max-width:100%;height:auto}pre{padding:16px;overflow:auto;border-radius:8px;background:#f3f5f8}
  code{font-family:Consolas,monospace}blockquote{margin-left:0;padding-left:20px;border-left:3px solid #d7e2ee;color:#5b6b7e}
  table{display:block;max-width:100%;overflow:auto}iframe{max-width:100%}
  @media(max-width:520px){body{padding-inline:24px}}
`;
