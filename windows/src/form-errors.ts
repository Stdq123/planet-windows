type Field = HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement;
type FieldError = {field: Field; message: string};

// Keep native constraints, custom validation and server field errors on one path.
export function createFormErrors(form: HTMLFormElement, result: HTMLElement) {
  const errors = new Map<Field, {message: string; targets: {element: HTMLElement; invalid: string | null; addedDescription: boolean}[]}>();
  let summary = "";
  // Submission handlers call validate() so native failures also receive ARIA links.
  form.noValidate = true;

  function choiceGroup(field: Field) {
    const group = field.nextElementSibling as HTMLElement | null;
    return field.hidden && group?.dataset.select === field.id ? group : null;
  }

  function render() {
    summary = [...new Set([...errors.values()].map(error => error.message))].join(" ");
    result.textContent = summary;
  }

  function clear(field?: Field) {
    const ownedSummary = result.textContent === summary;
    for (const [input, error] of errors) {
      if (field && field !== input) continue;
      for (const target of error.targets) {
        if (target.invalid === null) target.element.removeAttribute("aria-invalid");
        else target.element.setAttribute("aria-invalid", target.invalid);
        if (target.addedDescription) {
          const descriptions = (target.element.getAttribute("aria-describedby") || "").split(/\s+/).filter(id => id && id !== result.id);
          if (descriptions.length) target.element.setAttribute("aria-describedby", descriptions.join(" "));
          else target.element.removeAttribute("aria-describedby");
        }
      }
      errors.delete(input);
    }
    if (ownedSummary) render();
  }

  function focusFirst() {
    const field = [...form.querySelectorAll<Field>("input,textarea,select")].find(input => errors.has(input) && !input.disabled);
    if (!field) return;
    const group = choiceGroup(field);
    (group?.querySelector<HTMLInputElement>('input:checked:not(:disabled)') ?? group?.querySelector<HTMLInputElement>('input:not(:disabled)') ?? field).focus();
  }

  function show(failures: FieldError[]) {
    for (const {field, message} of failures) {
      let error = errors.get(field);
      if (!error) {
        const group = choiceGroup(field);
        const targets = [field, ...(group ? [group] : [])].map(element => {
          const invalid = element.getAttribute("aria-invalid");
          const descriptions = (element.getAttribute("aria-describedby") || "").split(/\s+/).filter(Boolean);
          const addedDescription = !descriptions.includes(result.id);
          element.setAttribute("aria-invalid", "true");
          if (addedDescription) element.setAttribute("aria-describedby", [...descriptions, result.id].join(" "));
          return {element, invalid, addedDescription};
        });
        error = {message, targets};
        errors.set(field, error);
      } else error.message = message;
    }
    render();
    focusFirst();
  }

  function validate() {
    clear();
    const failures: FieldError[] = [];
    for (const field of form.querySelectorAll<Field>("input,textarea,select")) {
      if (!field.willValidate || field.validity.valid) continue;
      const label = field.getAttribute("aria-label") || field.labels?.[0]?.firstChild?.textContent?.trim() || "此项";
      const message = field.validity.valueMissing ? "请填写此项。" : field.validity.typeMismatch ? "请输入有效的格式。" : field.validity.badInput || field.validity.stepMismatch ? "请输入有效的数值。" : field.validationMessage || "请检查此项输入。";
      failures.push({field, message: `${label}：${message}`});
    }
    if (failures.length) show(failures);
    return failures.length === 0;
  }

  for (const event of ["input", "change"]) form.addEventListener(event, event => {
    const field = event.target as Field;
    if (errors.has(field)) clear(field);
  });
  form.addEventListener("reset", () => clear());
  form.closest("dialog")?.addEventListener("close", () => clear());
  return {clear, show, validate, focusFirst};
}
