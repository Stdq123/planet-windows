import { enterFirstSite } from "./site-workflow";
import {expect,test} from "bun:test";
import {Window} from "happy-dom";
async function waitFor(fn:()=>boolean){for(let i=0;i<150;i++){if(fn())return;await new Promise(r=>setTimeout(r,10));}throw Error("timed out");}

test("site public profile saves domain and social fields",async()=>{
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let saved:any;
 (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.UpdateSiteProfile":saved=JSON.parse(args[1]);return {...site,...saved};case "Library.PublishSite":return {cid:"test",ipns:"test"};default:throw Error(method);}}};
 await import("../src/main.ts?site-profile"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
 window.document.querySelector<HTMLButtonElement>("#site-profile")!.click();const form=window.document.querySelector<HTMLFormElement>("#site-profile-form")!;
 form.querySelector<HTMLInputElement>('[name="domain"]')!.value="example.eth";form.querySelector<HTMLInputElement>('[name="githubUsername"]')!.value="alice-smith";
 form.dispatchEvent(new window.Event("submit",{cancelable:true}));await waitFor(()=>!!saved);expect(saved.domain).toBe("example.eth");expect(saved.githubUsername).toBe("alice-smith");
});
