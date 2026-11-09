import { enterFirstSite } from "./site-workflow";
import {expect,test} from "bun:test";
import {Window} from "happy-dom";

async function waitFor(predicate:()=>boolean){for(let i=0;i<100;i++){if(predicate())return;await new Promise(resolve=>setTimeout(resolve,10));}throw new Error("Timed out");}

test("site setting saves search-engine indexing preference with the site",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  let site={id:"s",name:"My site",about:"",created:"2026-01-01T00:00:00Z",doNotIndex:false};let call:any[]=[];
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.UpdateSiteSettings":call=args;site={...site,doNotIndex:args[3]};return site;default:throw new Error(method);}}};
  await import("../src/main.ts?no-index-setting"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#edit-site")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!,toggle=form.querySelector<HTMLInputElement>('input[name="doNotIndex"]')!;
  toggle.checked=true;form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));
  await waitFor(()=>call.length>0);expect(call).toEqual(["s","My site","",true]);
});

test("site avatar can be selected without losing unsaved site text",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"Before",created:"2026-01-01T00:00:00Z"};let selected=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.SelectSiteAvatar":selected++;return {...site,avatarPng:"aGVsbG8="};case "Library.PublishSite":return {cid:"test",ipns:"test"};default:throw new Error(method);}}};
  await import("../src/main.ts?site-avatar"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
  window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!,name=form.querySelector<HTMLInputElement>('[name="name"]')!;name.value="Unsaved name";
  window.document.querySelector<HTMLButtonElement>("#choose-site-avatar")!.click();await waitFor(()=>selected===1);
  expect(name.value).toBe("Unsaved name");
  await waitFor(()=>window.document.querySelector<HTMLImageElement>("#site-avatar-preview")?.src.includes("aGVsbG8=")===true);
});

test("site identity saves slug and author alongside existing settings",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let received:any[]=[];
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.UpdateSiteDetails":received=args;return {...site,slug:args[4],authorName:args[5]};default:throw new Error(method);}}};
  await import("../src/main.ts?site-identity"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('[name="siteSlug"]')!.value="my-site";form.querySelector<HTMLInputElement>('[name="authorName"]')!.value="Ada";
  form.dispatchEvent(new window.Event("submit",{cancelable:true}));await waitFor(()=>received.length>0);expect(received).toEqual(["s","Site","",false,"my-site","Ada"]);
});

test("invalid site identity keeps the dialog and entered text for correction",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.UpdateSiteDetails":throw new Error("站点标识无效");default:throw new Error(method);}}};
  await import("../src/main.ts?site-identity-error"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!,input=form.querySelector<HTMLInputElement>('[name="siteSlug"]')!;input.value="bad/slug";
  form.dispatchEvent(new window.Event("submit",{cancelable:true}));await waitFor(()=>window.document.querySelector("#site-settings-result")?.textContent?.includes("站点标识无效")===true);
  expect(window.document.querySelector<HTMLDialogElement>("#site-dialog")!.open).toBe(true);expect(input.value).toBe("bad/slug");
});

test("site setting errors associate native and known server failures without losing input",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  window.confirm=()=>true;
  const site={id:"s",name:"Site",about:"Before",created:"2026-01-01T00:00:00Z"};let calls=0,error="站点标识只能包含字母、数字和连字符";
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.UpdateSiteDetails":calls++;throw new Error(error);default:throw new Error(method);}}};
  await import("../src/main.ts?site-field-errors");enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
  const open=window.document.querySelector<HTMLButtonElement>("#edit-site")!;open.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!,name=form.querySelector<HTMLInputElement>('[name="name"]')!,slug=form.querySelector<HTMLInputElement>('[name="siteSlug"]')!;
  const submit=()=>form.dispatchEvent(new window.Event("submit",{cancelable:true}));
  const result=window.document.querySelector("#site-settings-result")!;
  const hint=window.document.createElement("span");hint.id="site-slug-hint";hint.textContent="已有说明";slug.after(hint);slug.setAttribute("aria-describedby",hint.id);
  name.value="";slug.value="bad/slug";submit();
  expect(calls).toBe(0);expect(name.getAttribute("aria-invalid")).toBe("true");expect(name.getAttribute("aria-describedby")).toContain(result.id);expect(window.document.activeElement).toBe(name);
  name.value="Unsaved name";name.dispatchEvent(new window.Event("input",{bubbles:true}));expect(name.hasAttribute("aria-invalid")).toBe(false);
  submit();await waitFor(()=>calls===1 && !slug.disabled);
  expect(slug.value).toBe("bad/slug");expect(name.value).toBe("Unsaved name");expect(slug.getAttribute("aria-invalid")).toBe("true");expect(slug.getAttribute("aria-describedby")).toBe("site-slug-hint site-settings-result");expect(window.document.activeElement).toBe(slug);
  slug.value="good-slug";slug.dispatchEvent(new window.Event("input",{bubbles:true}));expect(slug.hasAttribute("aria-invalid")).toBe(false);expect(slug.getAttribute("aria-describedby")).toBe("site-slug-hint");expect(result.textContent).toBe("");
  error="无法连接服务";submit();await waitFor(()=>calls===2 && !slug.disabled);
  expect(result.textContent).toContain(error);expect(form.querySelector('[aria-invalid="true"]')).toBeNull();
  error="站点标识只能包含字母、数字和连字符";submit();await waitFor(()=>calls===3 && !slug.disabled);
  window.document.querySelector<HTMLButtonElement>("#cancel-site")!.click();open.click();
  expect(slug.hasAttribute("aria-invalid")).toBe(false);expect(slug.getAttribute("aria-describedby")).toBe("site-slug-hint");expect(result.textContent).toBe("");
});
