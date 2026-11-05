import { expect, test } from "bun:test";
import { Window } from "happy-dom";

const site = {id:"s", name:"Site", about:"", created:"2026-01-01T00:00:00Z"};
const draft = {id:"d", siteId:"s", title:"Private draft", content:"Private text", tags:{}, created:site.created, modified:site.created};
let serial = 0;
async function until(condition: () => boolean) {
  for (let i=0; i<100; i++) { if(condition()) return; await Bun.sleep(10); }
  throw new Error("UI did not reach the expected state");
}
async function fixture(override: (method:string, args:any[])=>unknown = ()=>undefined) {
  const window = new Window({url:"http://localhost/"});
  window.happyDOM.setWindowSize({width:1280,height:800});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis,{window,document:window.document});
  const calls:string[]=[];
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{
    calls.push(method); const result=override(method,args); if(result!==undefined) return await result;
    switch(method) {
      case "Library.Sites": return [site];
      case "Library.Subscriptions": case "Library.Articles": return [];
      case "Library.Drafts": return [draft];
      case "Library.SaveDraft": return {...draft,title:args[3],content:args[4]};
      case "Library.Preview": case "Library.PreviewDraft": return "<p>Preview</p>";
      default: throw new Error(`Unexpected backend call: ${method}`);
    }
  }};
  await import(`../src/main.ts?workspace-regression-${++serial}`);
  const $=<T extends Element=HTMLElement>(selector:string)=>window.document.querySelector<T>(selector)!;
  return {window,$,calls};
}

test("Ctrl+S cannot publish a draft behind a modal and saves locally in the editor",async()=>{
  const {window,$,calls}=await fixture();
  await until(()=>!$<HTMLButtonElement>("#show-drafts").disabled);
  $<HTMLButtonElement>("#show-drafts").click();await until(()=>!!$(".article-item"));
  $<HTMLButtonElement>(".article-item").click();$<HTMLButtonElement>("#edit-site").click();
  $("#site-form input").dispatchEvent(new window.KeyboardEvent("keydown",{key:"s",ctrlKey:true,bubbles:true,cancelable:true}));
  await Bun.sleep(20);
  expect(calls.some(name=>/SaveDraft|PublishDraft|SaveArticle|PublishSite/.test(name))).toBe(false);
  $<HTMLButtonElement>("#cancel-site").click();
  window.dispatchEvent(new window.KeyboardEvent("keydown",{key:"s",ctrlKey:true,cancelable:true}));
  await until(()=>calls.includes("Library.SaveDraft"));
  expect(calls.some(name=>/PublishDraft|PublishSite/.test(name))).toBe(false);
});

test("failed creation retains input and supports retry without closing",async()=>{
  let attempts=0;
  const {window,$}=await fixture(method=>method==="Library.CreateSite" ? (++attempts===1?Promise.reject(new Error("storage unavailable")):{...site,id:"new",name:"New"}) : undefined);
  await until(()=>$("#site-title").textContent==="Site");
  $<HTMLButtonElement>("#new-site").click();
  $<HTMLInputElement>('#site-form [name="name"]').value="New";
  $<HTMLTextAreaElement>('#site-form [name="about"]').value="Keep this text";
  $("#site-form").dispatchEvent(new window.Event("submit",{cancelable:true}));
  await until(()=>$("#site-settings-result").textContent!.includes("storage unavailable"));
  expect($<HTMLDialogElement>("#site-dialog").open).toBe(true);
  expect($<HTMLTextAreaElement>('#site-form [name="about"]').value).toBe("Keep this text");
  expect($<HTMLButtonElement>('#site-form [type="submit"]').disabled).toBe(false);
  $("#site-form").dispatchEvent(new window.Event("submit",{cancelable:true}));
  await until(()=>!$<HTMLDialogElement>("#site-dialog").open);
  expect(attempts).toBe(2);
});

test("subscription failures preserve the entered URL for correction",async()=>{
  const {window,$}=await fixture(method=>method==="Library.Follow"?Promise.reject(new Error("invalid feed")):undefined);
  await until(()=>$("#site-title").textContent==="Site");
  $<HTMLButtonElement>("#new-feed").click();$<HTMLInputElement>('#feed-form [name="url"]').value="https://example.com/feed";
  $("#feed-form").dispatchEvent(new window.Event("submit",{cancelable:true}));
  await until(()=>$("#feed-result").textContent!.includes("invalid feed"));
  expect($<HTMLDialogElement>("#feed-dialog").open).toBe(true);
  expect($<HTMLInputElement>('#feed-form [name="url"]').value).toBe("https://example.com/feed");
});

test("initial errors are distinct from empty data and can be retried",async()=>{
  let reads=0;
  const { $ }=await fixture(method=>method==="Library.Sites" ? (++reads===1?Promise.reject(new Error("private path / internal stack")):[site]):undefined);
  await until(()=>!$<HTMLButtonElement>("#retry-library").hidden);
  expect($("#empty-action").hidden).toBe(true);
  expect($("#library-message").textContent).toContain("站点");
  expect($("#library-message").textContent).not.toContain("private path");
  $<HTMLButtonElement>("#retry-library").click();
  await until(()=>$("#site-title").textContent==="Site" && $("#library-notice").hidden===true);
  expect(reads).toBe(2);
});

test("a slow subscription read does not prevent already loaded sites being used",async()=>{
  let resolve:(value:any[])=>void=()=>{};
  const pending=new Promise<any[]>(r=>resolve=r);
  const {$}=await fixture(method=>method==="Library.Subscriptions"?pending:undefined);
  await until(()=>$("#site-title").textContent==="Site");
  expect($("#library-notice").hidden).toBe(false);
  expect($("#library-message").textContent).toContain("正在读取");
  expect($<HTMLButtonElement>("#new-article").disabled).toBe(false);
  resolve([]);await until(()=>$("#library-notice").hidden===true);
});

test("composition Enter does not consume a tag or falsely report a new article as saved",async()=>{
  const {window,$}=await fixture();await until(()=>!$<HTMLButtonElement>("#new-article").disabled);
  $<HTMLButtonElement>("#new-article").click();
  expect($("#save-state").textContent).toBe("新文章");
  const tag=$<HTMLInputElement>("#tag-input");tag.value="ceshi";
  const event=new window.KeyboardEvent("keydown",{key:"Enter",isComposing:true,bubbles:true,cancelable:true});tag.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(false);expect(tag.value).toBe("ceshi");expect($("#tag-list").textContent).toBe("");
  tag.value="测试";tag.dispatchEvent(new window.KeyboardEvent("keydown",{key:"Enter",bubbles:true,cancelable:true}));
  expect($("#tag-list").textContent).toContain("测试");
});

test("editor and preview switching keeps unsaved content and exposes only the chosen pane",async()=>{
  const {window,$}=await fixture();await until(()=>!$<HTMLButtonElement>("#new-article").disabled);
  $<HTMLButtonElement>("#new-article").click();
  const content=$<HTMLTextAreaElement>("#content");content.value="Unsent words";content.dispatchEvent(new window.Event("input"));
  $<HTMLButtonElement>("#view-preview").click();
  expect($("#writing-pane").hidden).toBe(true);expect($("#preview-pane").hidden).toBe(false);
  expect($("#view-preview").getAttribute("aria-pressed")).toBe("true");
  $<HTMLButtonElement>("#view-write").click();
  expect($("#writing-pane").hidden).toBe(false);expect($("#preview-pane").hidden).toBe(true);expect(content.value).toBe("Unsent words");
});

test("dirty settings can reject both Escape and the cancel button",async()=>{
  const {window,$}=await fixture();await until(()=>$("#site-title").textContent==="Site");
  $<HTMLButtonElement>("#edit-site").click();$<HTMLInputElement>('#site-form [name="name"]').value="Unsaved";
  window.confirm=()=>false;
  const escape=new window.Event("cancel",{cancelable:true});$("#site-dialog").dispatchEvent(escape);
  expect(escape.defaultPrevented).toBe(true);
  $<HTMLButtonElement>("#cancel-site").click();expect($<HTMLDialogElement>("#site-dialog").open).toBe(true);
  window.confirm=()=>true;$<HTMLButtonElement>("#cancel-site").click();expect($<HTMLDialogElement>("#site-dialog").open).toBe(false);
});
