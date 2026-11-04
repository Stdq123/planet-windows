import {expect,test} from "bun:test";
import {Window} from "happy-dom";
async function waitFor(fn:()=>boolean){for(let i=0;i<150;i++){if(fn())return;await new Promise(r=>setTimeout(r,10));}throw Error("timed out");}
test("podcast settings save language, explicit flag and categories",async()=>{
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let args:any[]=[];
 (globalThis as any).mygo={call:async(method:string,...a:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.UpdatePodcastSettings":args=a;return {...site,podcastLanguage:a[1],podcastExplicit:a[2],podcastCategories:JSON.parse(a[3])};case "Library.PublishSite":return {cid:"test",ipns:"test"};default:throw Error(method);}}};
 await import("../src/main.ts?podcast-settings");await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");window.document.querySelector<HTMLButtonElement>("#podcast-settings")!.click();
 const form=window.document.querySelector<HTMLFormElement>("#podcast-settings-form")!;form.querySelector<HTMLInputElement>('[name="language"]')!.value="zh-CN";form.querySelector<HTMLInputElement>('[name="explicit"]')!.checked=true;form.querySelector<HTMLInputElement>('[value="Technology"]')!.checked=true;
 form.dispatchEvent(new window.Event("submit",{cancelable:true}));await waitFor(()=>args.length>0);expect(args).toEqual(["s","zh-CN",true,`["Technology"]`]);
});
