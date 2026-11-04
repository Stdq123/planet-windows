import {expect,test} from "bun:test";
import {Window} from "happy-dom";
async function waitFor(fn:()=>boolean){for(let i=0;i<150;i++){if(fn())return;await new Promise(r=>setTimeout(r,10));}throw Error("timed out");}
test("Cloudflare deployment is saved locally and deployed only after user action",async()=>{
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});window.confirm=()=>true;
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let saved:any[]=[],deploys=0;
 (globalThis as any).mygo={call:async(method:string,...a:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.DeploymentInfo":return {configured:false,name:"",server:"",autoDeploy:false,deleteRemote:false,pending:false,lastUrl:""};case "Library.ConfigureDeployment":saved=a;return;case "Library.DeploySite":deploys++;return "https://my-site.pages.dev/";default:throw Error(method);}}};
 await import("../src/main.ts?deployment-settings");await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");window.document.querySelector<HTMLButtonElement>("#deployment-settings")!.click();
 const form=window.document.querySelector<HTMLFormElement>("#deployment-form")!,kind=form.querySelector<HTMLSelectElement>('[name="kind"]')!;kind.value="cloudflare";kind.dispatchEvent(new window.Event("change"));
 await waitFor(()=>form.querySelector<HTMLInputElement>('[name="name"]')?.disabled===false);form.querySelector<HTMLInputElement>('[name="name"]')!.value="my-site";form.querySelector<HTMLInputElement>('[name="server"]')!.value="0123456789abcdef0123456789abcdef";form.querySelector<HTMLInputElement>('[name="token"]')!.value="secret";
 form.dispatchEvent(new window.Event("submit",{cancelable:true}));await waitFor(()=>saved.length>0);expect(saved).toEqual(["s","cloudflare","my-site","0123456789abcdef0123456789abcdef","secret",false,false]);expect(deploys).toBe(0);
});
