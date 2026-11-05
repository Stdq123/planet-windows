import { openDialog, rememberDialog } from "./ui";
import {Library, type UpdateCheckInfo, type UpdateSettingsInfo} from "./mygo";

export function initUpdates(){
 const el=<T extends Element>(s:string)=>document.querySelector<T>(s)!;
 const open=el<HTMLButtonElement>("#show-updates"),dialog=el<HTMLDialogElement>("#updates-dialog"),form=el<HTMLFormElement>("#updates-form");
 dialog.dataset.dismissWhileBusy="true";
 const feed=form.elements.namedItem("feedURL") as HTMLInputElement,key=form.elements.namedItem("publicKey") as HTMLInputElement,auto=form.elements.namedItem("autoCheck") as HTMLInputElement;
 const check=el<HTMLButtonElement>("#check-updates"),download=el<HTMLButtonElement>("#download-update"),reveal=el<HTMLButtonElement>("#reveal-update"),path=el<HTMLInputElement>("#update-path"),result=el<HTMLParagraphElement>("#update-result"),save=form.querySelector<HTMLButtonElement>('[type="submit"]')!;
 let baseline:UpdateSettingsInfo|null=null,release:UpdateCheckInfo|null=null,busy=false,revision=0;
 const dirty=()=>!baseline || feed.value.trim()!==baseline.settings.feedURL || key.value.trim()!==baseline.settings.publicKey || auto.checked!==baseline.settings.autoCheck;
 const controls=()=>{save.disabled=busy;check.disabled=busy||dirty()||!baseline?.configured;download.disabled=busy||dirty()||!release?.available||!!path.value;reveal.hidden=!path.value||dirty();};
 const resetResult=()=>{release=null;path.value="";path.hidden=true;reveal.hidden=true;open.textContent="检查更新";open.title="";};
 const apply=(info:UpdateSettingsInfo)=>{baseline=info;feed.value=info.settings.feedURL;key.value=info.settings.publicKey;auto.checked=info.settings.autoCheck;el("#update-current").textContent=`当前版本 ${info.currentVersion}`;rememberDialog(dialog);};
 async function checkNow(visible=true){
  if(visible&&(busy||dirty()||!baseline?.configured))return;
  const request=++revision;busy=true;resetResult();if(visible)result.textContent="正在验证更新清单…";controls();
  try{const found=await Library.checkUpdates();if(request!==revision)return;release=found;result.textContent=found.available?`发现版本 ${found.version}（${(found.size/1024/1024).toFixed(1)} MiB），签名已验证。`:"当前已是最新版本。";if(found.available){open.textContent=`有更新 ${found.version}`;open.title="已验证更新清单，点击查看";}}
  catch(e){if(request===revision&&visible)result.textContent=String(e);}
  finally{if(request===revision){busy=false;controls();}}
 }
 open.addEventListener("click",async()=>{
  const request=++revision;busy=true;baseline=null;resetResult();result.textContent="正在读取更新设置…";openDialog(dialog);controls();
  try{const info=await Library.updateSettings();if(request!==revision)return;apply(info);result.textContent=info.configured?"点击检查更新验证发布者清单。":"尚未配置更新源，不会自动联网检查。";}
  catch(e){if(request===revision)result.textContent=String(e);}
  finally{if(request===revision){busy=false;controls();}}
 });
 for(const input of [feed,key,auto])input.addEventListener("input",()=>{revision++;busy=false;resetResult();result.textContent="设置有未保存的更改，请先保存后再检查。";controls();void Library.cancelUpdateOperation().catch(()=>{});});
 form.addEventListener("submit",async(event)=>{
  event.preventDefault();if(busy)return;const request=++revision;busy=true;resetResult();controls();
  try{const info=await Library.saveUpdateSettings(feed.value.trim(),key.value.trim(),auto.checked);if(request!==revision)return;apply(info);result.textContent=info.configured?"已保存可信更新设置。":"已关闭更新检查。";}
  catch(e){if(request===revision)result.textContent=String(e);}
  finally{if(request===revision){busy=false;controls();}}
 });
 check.addEventListener("click",()=>void checkNow());
 download.addEventListener("click",async()=>{
  if(busy||dirty()||!release?.available)return;const request=++revision,selected=release;busy=true;result.textContent="正在下载并校验安装包…";controls();
  try{const saved=await Library.downloadUpdate(selected.version,selected.sha256);if(request!==revision)return;path.value=saved;path.hidden=false;result.textContent="已验证签名与安装包校验值。关闭 Planet 后运行此安装包。";}
  catch(e){if(request===revision)result.textContent=String(e);}
  finally{if(request===revision){busy=false;controls();}}
 });
 reveal.addEventListener("click",async()=>{if(busy||dirty()||!path.value)return;try{await Library.revealDownloadedUpdate();}catch(e){result.textContent=String(e);}});
 const cancel=()=>{revision++;busy=false;resetResult();void Library.cancelUpdateOperation().catch(()=>{});};
 el("#close-updates").addEventListener("click",()=>{cancel();dialog.close();});dialog.addEventListener("cancel",event=>{if(!event.defaultPrevented)cancel();});
 controls();
 // Only an explicit saved preference enables startup network access.
 const startupRevision=revision;
 void Library.updateSettings().then(info=>{if(startupRevision!==revision)return;if(info.configured&&info.settings.autoCheck){apply(info);void checkNow(false);}}).catch(()=>{});
}
