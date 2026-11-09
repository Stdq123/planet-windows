import {expect, test} from "bun:test";
import {Window} from "happy-dom";
import {createFormErrors} from "../src/form-errors";
import {prepareChoices} from "../src/ui";

function setup() {
  const window=new Window({url:"http://localhost/"});
  window.document.body.innerHTML='<dialog open><form id="settings"><label>名称<input name="name" required aria-describedby="name-help" /></label><span id="name-help">原有说明</span><label>地址<input name="url" type="url" /></label><p id="errors" role="status"></p><button type="submit">保存</button></form></dialog>';
  const form=window.document.querySelector<HTMLFormElement>("form")!,result=window.document.querySelector<HTMLElement>("#errors")!;
  const name=form.querySelector<HTMLInputElement>('[name="name"]')!,url=form.querySelector<HTMLInputElement>('[name="url"]')!;
  const errors=createFormErrors(form as unknown as HTMLFormElement,result as unknown as HTMLElement);
  return {window,form,result,name,url,errors};
}

test("native submit reports all invalid fields in form order and retains entered values",()=>{
  const ui=setup();let saved=0;
  ui.form.addEventListener("submit",event=>{event.preventDefault();if(ui.errors.validate())saved++;});
  ui.url.value="not-a-url";ui.form.querySelector<HTMLButtonElement>("button")!.click();
  expect(saved).toBe(0);expect(ui.name.getAttribute("aria-invalid")).toBe("true");expect(ui.url.getAttribute("aria-invalid")).toBe("true");expect(ui.window.document.activeElement).toBe(ui.name);
  expect(ui.name.getAttribute("aria-describedby")).toBe("name-help errors");expect(ui.url.getAttribute("aria-describedby")).toBe("errors");expect(ui.result.textContent).toContain("名称");expect(ui.result.textContent).toContain("地址");expect(ui.url.value).toBe("not-a-url");
  ui.name.value="Updated";ui.name.dispatchEvent(new ui.window.Event("input",{bubbles:true}));
  expect(ui.name.getAttribute("aria-describedby")).toBe("name-help");expect(ui.result.textContent).not.toContain("名称");expect(ui.url.getAttribute("aria-invalid")).toBe("true");
  ui.url.value="https://example.com";ui.form.querySelector<HTMLButtonElement>("button")!.click();
  expect(saved).toBe(1);expect(ui.form.querySelector('[aria-invalid="true"]')).toBeNull();expect(ui.result.textContent).toBe("");
});

test("clearing a field error preserves original descriptions and later added descriptions",()=>{
  const ui=setup();ui.name.setAttribute("aria-invalid","false");
  ui.errors.show([{field:ui.name as unknown as HTMLInputElement,message:"请更正名称"}]);
  ui.name.setAttribute("aria-describedby",`${ui.name.getAttribute("aria-describedby")} extra-help`);
  ui.errors.show([{field:ui.name as unknown as HTMLInputElement,message:"请再次更正名称"}]);
  expect(ui.name.getAttribute("aria-describedby")).toBe("name-help errors extra-help");
  ui.errors.clear();expect(ui.name.getAttribute("aria-invalid")).toBe("false");expect(ui.name.getAttribute("aria-describedby")).toBe("name-help extra-help");expect(ui.result.textContent).toBe("");
});

test("clearing field errors does not erase a later service status",()=>{
  const ui=setup();ui.errors.show([{field:ui.name as unknown as HTMLInputElement,message:"请更正名称"}]);
  ui.result.textContent="服务暂不可用，请重试";ui.errors.clear();
  expect(ui.result.textContent).toBe("服务暂不可用，请重试");expect(ui.name.getAttribute("aria-describedby")).toBe("name-help");
});

test("converted select errors are exposed on the visible radio group and focus its choice",()=>{
  const ui=setup(),label=ui.window.document.createElement("label");label.innerHTML='类型<select name="type"><option value="0">文章</option><option value="1">页面</option></select>';ui.form.append(label);
  prepareChoices(ui.window.document as unknown as Document);
  const select=ui.form.querySelector<HTMLSelectElement>("select")!,group=ui.form.querySelector<HTMLElement>('[role="radiogroup"]')!,checked=group.querySelector<HTMLInputElement>("input:checked")!;
  ui.errors.show([{field:select as unknown as HTMLSelectElement,message:"请选择有效类型"}]);
  expect(group.getAttribute("aria-invalid")).toBe("true");expect(group.getAttribute("aria-describedby")).toBe("errors");expect(ui.window.document.activeElement).toBe(checked);
  const next=group.querySelectorAll<HTMLInputElement>("input")[1]!;next.checked=true;next.dispatchEvent(new ui.window.Event("change",{bubbles:true}));
  expect(group.hasAttribute("aria-invalid")).toBe(false);expect(select.hasAttribute("aria-invalid")).toBe(false);expect(ui.result.textContent).toBe("");
});
