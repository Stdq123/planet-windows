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
