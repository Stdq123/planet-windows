import { enterFirstSite } from "./site-workflow";
import {expect,test} from "bun:test";
import {Window} from "happy-dom";

async function waitFor(fn:()=>boolean) {for(let i=0;i<150;i++){if(fn())return;await new Promise(r=>setTimeout(r,10));}throw new Error("timed out");}

test("article settings save page, path, external link and navigation as one change",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"site",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  const article={id:"article",siteId:"site",title:"About",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{}};
  let received:any[]=[];
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [article];case "Library.PreviewArticle":return "Body";case "Library.UpdateArticleSettings":received=args;return {...article,slug:args[2],articleType:args[3],externalLink:args[4],isIncludedInNavigation:args[5],navigationWeight:args[6]};case "Library.PublishSite":return {cid:"test",ipns:"test"};default:throw new Error(method);}}};
  await import("../src/main.ts?article-settings"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  window.document.querySelector<HTMLButtonElement>("#article-settings")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#article-settings-form")!;
  form.querySelector<HTMLInputElement>('[name="slug"]')!.value="about";
  form.querySelector<HTMLSelectElement>('[name="articleType"]')!.value="1";
  form.querySelector<HTMLInputElement>('[name="externalLink"]')!.value="https://example.com/";
  form.querySelector<HTMLInputElement>('[name="inNavigation"]')!.checked=true;
  form.querySelector<HTMLInputElement>('[name="weight"]')!.value="-2";
  form.dispatchEvent(new window.Event("submit",{cancelable:true}));await waitFor(()=>received.length>0);
  expect(received).toEqual(["site","article","about",1,"https://example.com/",true,-2]);
});

test("sharing a newly changed path warns until publication completes",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",ipns:"k51example",lastPublishedCID:"bafyold",created:"2026-01-01T00:00:00Z"};
  const article={id:"a",siteId:"s",title:"Post",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{}};
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [article];case "Library.PreviewArticle":return "Body";case "Library.UpdateArticleSettings":return {...article,slug:args[2]};case "Library.PublishSite":return new Promise(()=>{});default:throw new Error(method);}}};
  await import("../src/main.ts?article-settings-share-pending"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();window.document.querySelector<HTMLButtonElement>("#article-settings")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#article-settings-form")!;form.querySelector<HTMLInputElement>('[name="slug"]')!.value="new-path";form.dispatchEvent(new window.Event("submit",{cancelable:true}));
  await waitFor(()=>!window.document.querySelector<HTMLDialogElement>("#article-settings-dialog")!.open);window.document.querySelector<HTMLButtonElement>("#share-own-article")!.click();
  await waitFor(()=>!!window.document.querySelector("#own-share-status")?.textContent);expect(window.document.querySelector("#own-share-status")!.textContent).toContain("未发布");
});

test("article settings focus native, custom and server field errors and clear them on retry",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});window.confirm=()=>true;
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  const article={id:"a",siteId:"s",title:"Post",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{}};
  let calls=0,error="自定义路径与其他文章冲突";
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [article];case "Library.PreviewArticle":return "Body";case "Library.UpdateArticleSettings":calls++;throw new Error(error);default:throw new Error(method);}}};
  await import("../src/main.ts?article-field-errors");enterFirstSite(window);await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  const open=window.document.querySelector<HTMLButtonElement>("#article-settings")!;open.click();
  const form=window.document.querySelector<HTMLFormElement>("#article-settings-form")!,weight=form.querySelector<HTMLInputElement>('[name="weight"]')!,external=form.querySelector<HTMLInputElement>('[name="externalLink"]')!,slug=form.querySelector<HTMLInputElement>('[name="slug"]')!;
  const submit=()=>form.dispatchEvent(new window.Event("submit",{cancelable:true}));const result=window.document.querySelector("#article-settings-result")!;
  weight.value="";submit();expect(calls).toBe(0);expect(weight.getAttribute("aria-invalid")).toBe("true");expect(weight.getAttribute("aria-describedby")).toContain(result.id);expect(window.document.activeElement).toBe(weight);
  weight.value="2147483648";submit();expect(calls).toBe(0);expect(weight.getAttribute("aria-invalid")).toBe("true");expect(result.textContent).toContain("整数权重");expect(window.document.activeElement).toBe(weight);
  weight.value="2";external.value="bad-url";submit();expect(calls).toBe(0);expect(weight.hasAttribute("aria-invalid")).toBe(false);expect(external.getAttribute("aria-invalid")).toBe("true");expect(window.document.activeElement).toBe(external);
  external.value="https://example.com/";slug.value="duplicate";submit();await waitFor(()=>calls===1 && !slug.disabled);
  expect(slug.value).toBe("duplicate");expect(external.value).toBe("https://example.com/");expect(external.hasAttribute("aria-invalid")).toBe(false);expect(slug.getAttribute("aria-invalid")).toBe("true");expect(window.document.activeElement).toBe(slug);
  error="写入失败";submit();await waitFor(()=>calls===2 && !slug.disabled);expect(result.textContent).toContain(error);expect(form.querySelector('[aria-invalid="true"]')).toBeNull();
  error="自定义路径与其他文章冲突";submit();await waitFor(()=>calls===3 && !slug.disabled);
  window.document.querySelector<HTMLButtonElement>("#close-article-settings")!.click();open.click();expect(form.querySelector('[aria-invalid="true"]')).toBeNull();expect(result.textContent).toBe("");
});
