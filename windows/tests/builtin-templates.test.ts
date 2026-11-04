import {expect,test} from "bun:test";
import {Window} from "happy-dom";
async function waitFor(fn:()=>boolean){for(let i=0;i<150;i++){if(fn())return;await new Promise(r=>setTimeout(r,10));}throw Error("timed out");}

test("template gallery previews and applies a bundled template, with manual import available",async()=>{
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let selected="",manual=0;
 (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.BuiltinTemplates":return [{id:"plain",name:"Plain",description:"简洁",build:132},{id:"grid",name:"Grid",description:"图片",build:99}];case "Library.BuiltinTemplatePreview":return `<h1>Preview ${args[0]}</h1>`;case "Library.ApplyBuiltinTemplate":selected=args[1];return {...site,templateName:"Grid",templateId:"template-1"};case "Library.SelectTemplate":manual++;return {};default:throw Error(method);}}};
 window.alert=()=>{};await import("../src/main.ts?builtin-templates");await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
 window.document.querySelector<HTMLButtonElement>("#select-template")!.click();await waitFor(()=>window.document.querySelectorAll(".template-choice").length===2);
 expect(window.document.querySelector("#template-gallery")!.hasAttribute("open")).toBe(true);
 expect(window.document.querySelector<HTMLIFrameElement>("#template-preview")!.getAttribute("sandbox")).toBe("");
 window.document.querySelector<HTMLButtonElement>('[data-template-id="grid"]')!.click();await waitFor(()=>window.document.querySelector<HTMLIFrameElement>("#template-preview")!.srcdoc.includes("Preview grid"));
 window.document.querySelector<HTMLButtonElement>("#template-apply")!.click();await waitFor(()=>selected==="grid");
 window.document.querySelector<HTMLButtonElement>("#select-template")!.click();await waitFor(()=>window.document.querySelectorAll(".template-choice").length===2);
 window.document.querySelector<HTMLButtonElement>("#template-import")!.click();await waitFor(()=>manual===1);
});
