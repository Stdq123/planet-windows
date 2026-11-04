import {expect,test} from "bun:test";
import {Window} from "happy-dom";
const waitFor=async(fn:()=>boolean)=>{for(let n=0;n<100;n++){if(fn())return;await new Promise(r=>setTimeout(r,10));}throw new Error("update UI timed out");};
test("updates require saved trust settings and an explicit verified download",async()=>{
 const w=new Window({url:"http://localhost/"});w.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window:w,document:w.document});
 let settings={feedURL:"",publicKey:"",autoCheck:false},checks=0,downloads=0,reveals=0;
 (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){
 case "Library.UpdateSettings":return {settings,currentVersion:"0.3.0",configured:!!settings.feedURL};
 case "Library.SaveUpdateSettings":settings={feedURL:args[0],publicKey:args[1],autoCheck:args[2]};return {settings,currentVersion:"0.3.0",configured:true};
 case "Library.CheckUpdates":checks++;return {version:"0.4.0",sha256:"a".repeat(64),size:123,available:true};
 case "Library.DownloadUpdate":downloads++;expect(args).toEqual(["0.4.0","a".repeat(64)]);return "C:\\private\\setup.exe";
 case "Library.RevealDownloadedUpdate":reveals++;return;
 default:throw new Error(method);
 }}};
 const {initUpdates}=await import("../src/updates");initUpdates();
 w.document.querySelector<HTMLButtonElement>("#show-updates")!.click();await waitFor(()=>w.document.querySelector<HTMLDialogElement>("#updates-dialog")!.open);
 const form=w.document.querySelector<HTMLFormElement>("#updates-form")!,check=w.document.querySelector<HTMLButtonElement>("#check-updates")!,download=w.document.querySelector<HTMLButtonElement>("#download-update")!;
 expect(check.disabled).toBe(true);expect(checks).toBe(0);
 form.querySelector<HTMLInputElement>('[name="feedURL"]')!.value="https://updates.example/feed.json";form.querySelector<HTMLInputElement>('[name="publicKey"]')!.value="publisher-key";
 form.dispatchEvent(new w.Event("submit",{cancelable:true}));await waitFor(()=>!check.disabled);
 check.click();await waitFor(()=>!download.disabled);expect(downloads).toBe(0);
 download.click();await waitFor(()=>w.document.querySelector<HTMLInputElement>("#update-path")!.value!=="");expect(downloads).toBe(1);
 w.document.querySelector<HTMLButtonElement>("#reveal-update")!.click();await waitFor(()=>reveals===1);
 const key=form.querySelector<HTMLInputElement>('[name="publicKey"]')!;key.value="replacement";key.dispatchEvent(new w.Event("input"));expect(w.document.querySelector("#show-updates")!.textContent).toBe("检查更新");expect(w.document.querySelector<HTMLInputElement>("#update-path")!.value).toBe("");expect(download.disabled).toBe(true);
});

test("editing trust settings invalidates an outstanding check and its download",async()=>{
 const w=new Window({url:"http://localhost/"});w.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window:w,document:w.document});
 let resolveCheck:((r:any)=>void)|undefined,downloads=0;
 (globalThis as any).mygo={call:async(method:string)=>{switch(method){
 case "Library.UpdateSettings":return {settings:{feedURL:"https://updates.example/feed.json",publicKey:"key",autoCheck:false},currentVersion:"0.3.0",configured:true};
 case "Library.CheckUpdates":return new Promise(r=>resolveCheck=r);
 case "Library.DownloadUpdate":downloads++;return "unexpected";
 default:throw new Error(method);
 }}};
 const {initUpdates}=await import("../src/updates");initUpdates();w.document.querySelector<HTMLButtonElement>("#show-updates")!.click();
 const check=w.document.querySelector<HTMLButtonElement>("#check-updates")!;await waitFor(()=>!check.disabled);check.click();await waitFor(()=>!!resolveCheck);
 const key=w.document.querySelector<HTMLInputElement>('[name="publicKey"]')!;key.value="new-key";key.dispatchEvent(new w.Event("input"));
 resolveCheck!({version:"0.4.0",sha256:"b".repeat(64),size:1,available:true});await new Promise(r=>setTimeout(r,25));
 const download=w.document.querySelector<HTMLButtonElement>("#download-update")!;expect(download.disabled).toBe(true);expect(w.document.querySelector("#update-result")!.textContent).toContain("保存");download.click();expect(downloads).toBe(0);
});
