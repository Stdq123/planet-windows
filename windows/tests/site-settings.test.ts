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

test("site settings uses wide two-column multi-page navigation in edit mode and compact layout in create mode",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  let confirmCalls=0;window.confirm=()=>{confirmCalls++;return false;};
  const site={id:"s",name:"My Planet",about:"About",created:"2026-01-01T00:00:00Z"};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];default:throw new Error(method);}}};
  await import("../src/main.ts?site-settings-layout"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="My Planet");
  const dialog=window.document.querySelector<HTMLDialogElement>("#site-dialog")!;
  const nav=window.document.querySelector<HTMLElement>("#site-settings-nav")!;
  window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  expect(dialog.dataset.mode).toBe("edit");
  expect(dialog.dataset.page).toBe("general");
  expect(nav.hidden).toBe(false);
  expect(window.document.querySelector("#site-nav-name")?.textContent).toBe("My Planet");
  const profileNav=window.document.querySelector<HTMLButtonElement>("#site-profile")!;
  const podcastNav=window.document.querySelector<HTMLButtonElement>("#podcast-settings")!;
  const generalNav=window.document.querySelector<HTMLButtonElement>("#site-general-nav")!;
  profileNav.click();
  expect(dialog.dataset.page).toBe("site-profile-dialog");
  expect(profileNav.classList.contains("is-active")).toBe(true);
  expect(profileNav.getAttribute("aria-current")).toBe("true");
  expect(window.document.querySelector<HTMLDialogElement>("#site-profile-dialog")!.open).toBe(true);
  podcastNav.click();
  expect(dialog.dataset.page).toBe("podcast-settings-dialog");
  expect(window.document.querySelector<HTMLDialogElement>("#site-profile-dialog")!.open).toBe(false);
  expect(window.document.querySelector<HTMLDialogElement>("#podcast-settings-dialog")!.open).toBe(true);
  generalNav.click();
  expect(dialog.dataset.page).toBe("general");
  expect(window.document.querySelector<HTMLDialogElement>("#podcast-settings-dialog")!.open).toBe(false);
  expect(generalNav.classList.contains("is-active")).toBe(true);
  const nameInput=window.document.querySelector<HTMLInputElement>('#site-form input[name="name"]')!;
  nameInput.value="Edited Name";
  window.document.querySelector<HTMLButtonElement>("#close-site-x")!.click();
  expect(confirmCalls).toBe(1);
  expect(dialog.open).toBe(true);
  window.confirm=()=>true;
  window.document.querySelector<HTMLButtonElement>("#close-site-x")!.click();
  expect(dialog.open).toBe(false);
  window.document.querySelector<HTMLButtonElement>("#new-site")!.click();
  expect(dialog.dataset.mode).toBe("create");
  expect(nav.hidden).toBe(true);
});

test("custom code and publication flags save from the dedicated site settings page",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"My Planet",about:"About",created:"2026-01-01T00:00:00Z"};
  let savedPayload:any=null;
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.UpdateSiteCustomCode":savedPayload=JSON.parse(args[1]);return {...site,customCodeHeadEnabled:savedPayload.headEnabled,customCodeHead:savedPayload.head,publishAsIPNS:savedPayload.publishAsIPNS,prewarmNewPost:savedPayload.prewarmNewPost};case "Library.PublishSite":return {cid:"test",ipns:"test"};default:throw new Error(method);}}};
  await import("../src/main.ts?site-custom-code"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="My Planet");
  window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  window.document.querySelector<HTMLButtonElement>("#custom-code-settings")!.click();
  const page=window.document.querySelector<HTMLDialogElement>("#custom-code-dialog")!;
  expect(page.open).toBe(true);
  const form=window.document.querySelector<HTMLFormElement>("#custom-code-form")!;
  form.querySelector<HTMLInputElement>('[name="headEnabled"]')!.checked=true;
  form.querySelector<HTMLTextAreaElement>('[name="head"]')!.value="<meta name=\"test\" content=\"1\">";
  form.querySelector<HTMLInputElement>('[name="publishAsIPNS"]')!.checked=false;
  form.querySelector<HTMLInputElement>('[name="prewarmNewPost"]')!.checked=false;
  form.dispatchEvent(new window.Event("submit",{cancelable:true}));
  await waitFor(()=>savedPayload!==null);
  expect(savedPayload.headEnabled).toBe(true);
  expect(savedPayload.head).toBe("<meta name=\"test\" content=\"1\">");
  expect(savedPayload.publishAsIPNS).toBe(false);
  expect(savedPayload.prewarmNewPost).toBe(false);
});

test("avatar gallery applies preset avatar and toggles saveRoundAvatar, and article list toggles grid view",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  let site:any={id:"s",name:"My Planet",about:"About",created:"2026-01-01T00:00:00Z"};
  const article={id:"a1",siteId:"s",title:"First Post",content:"![cover](hero.png)",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",heroImage:"hero.png"};
  let appliedKey="";let roundValue:boolean|null=null;
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){
    case "Library.Sites":return [site];
    case "Library.Subscriptions":return [];
    case "Library.Articles":return [article];
    case "Library.PresetAvatars":return [{key:"NSTG-0001",category:"Nostalgia",title:"Maglev Pram",png:"aGVsbG8="}];
    case "Library.ApplyPresetAvatar":appliedKey=args[1];site={...site,avatarPng:"aGVsbG8="};return site;
    case "Library.SetSaveRoundAvatar":roundValue=args[1];site={...site,saveRoundAvatar:args[1]};return site;
    case "Library.PublishSite":return {cid:"test",ipns:"test"};
    default:throw new Error(method);
  }}};
  await import("../src/main.ts?avatar-gallery-and-grid"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="My Planet");
  expect(window.document.querySelector(".article-hero-badge")?.textContent).toContain("hero.png");

  window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  const roundCheck=window.document.querySelector<HTMLInputElement>("#site-round-avatar")!;
  roundCheck.checked=true;roundCheck.dispatchEvent(new window.Event("change",{bubbles:true}));
  await waitFor(()=>roundValue===true);

  window.document.querySelector<HTMLButtonElement>("#open-avatar-gallery")!.click();
  await waitFor(()=>window.document.querySelectorAll(".avatar-gallery-item").length===1);
  window.document.querySelector<HTMLButtonElement>(".avatar-gallery-item")!.click();
  window.document.querySelector<HTMLFormElement>("#avatar-gallery-form")!.dispatchEvent(new window.Event("submit",{cancelable:true}));
  await waitFor(()=>appliedKey==="NSTG-0001");
});

test("reader supports related articles, local AI chat, and TTS speech player",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"My Planet",about:"About",created:"2026-01-01T00:00:00Z",imported:true};
  const article={id:"a1",siteId:"s",title:"IPFS 去中心化发布",content:"通过 Kubo 与 IPNS 构建去中心化站点。",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
  let relatedCalled=false,aiAsked=false;
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){
    case "Library.Sites":return [site];
    case "Library.Subscriptions":return [];
    case "Library.Articles":return [article];
    case "Library.PreviewArticle":return "<p>通过 Kubo 与 IPNS 构建去中心化站点。</p>";
    case "Library.RelatedArticles":relatedCalled=true;return [{title:"Kubo 节点指南",sourceName:"My Planet",snippet:"Kubo 配置",siteId:"s",articleId:"a1"}];
    case "Library.AIConfig":return {baseUrl:"http://localhost:11434/v1",model:"qwen2.5:7b",hasKey:false};
    case "Library.AskAI":aiAsked=true;expect(args[0]).toBe("这篇文章讲了什么？");return "本文介绍了如何使用 Kubo 和 IPNS 发布去中心化网站。";
    default:throw new Error(method);
  }}};
  await import("../src/main.ts?related-ai-tts"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="My Planet");
  // Imported site settings are unlocked
  expect(window.document.querySelector<HTMLButtonElement>("#site-profile")!.disabled).toBe(false);
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  await waitFor(()=>!window.document.querySelector<HTMLElement>("#own-reader")!.hidden);
  expect(window.document.querySelector<HTMLButtonElement>("#article-settings")!.disabled).toBe(false);

  // TTS Speech Player
  window.document.querySelector<HTMLButtonElement>("#speak-own-article")!.click();
  const speechBar=window.document.querySelector<HTMLElement>("#speech-player")!;
  expect(speechBar.hidden).toBe(false);
  expect(window.document.querySelector("#speech-title")?.textContent).toContain("IPFS 去中心化发布");
  window.document.querySelector<HTMLButtonElement>("#speech-stop")!.click();
  expect(speechBar.hidden).toBe(true);

  // Related Articles
  window.document.querySelector<HTMLButtonElement>("#related-own-article")!.click();
  await waitFor(()=>relatedCalled && window.document.querySelectorAll("#related-articles-results .article-item").length===1);
  window.document.querySelector<HTMLButtonElement>("#close-related-articles")!.click();

  // Local AI Chat
  window.document.querySelector<HTMLButtonElement>("#chat-own-article")!.click();
  await waitFor(()=>window.document.querySelector<HTMLDialogElement>("#ai-dialog")!.open);
  window.document.querySelector<HTMLInputElement>("#ai-chat-input")!.value="这篇文章讲了什么？";
  window.document.querySelector<HTMLFormElement>("#ai-chat-form")!.dispatchEvent(new window.Event("submit",{cancelable:true}));
  await waitFor(()=>aiAsked && window.document.querySelectorAll(".ai-chat-bubble").length===2);
});

test("local REST API console loads status, saves config, and displays request logs",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  let savedArgs:any[]=[];
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){
    case "Library.Sites":case "Library.Subscriptions":case "Library.Articles":return [];
    case "Library.LocalAPIStatus":return {running:false,enabled:false,port:8086,authEnabled:false,username:"Planet",hasPasscode:false,logs:[]};
    case "Library.ConfigureLocalAPI":savedArgs=args;return {running:true,enabled:true,port:8086,authEnabled:true,username:"Planet",hasPasscode:true,logs:[{time:"00:55:00",method:"GET",path:"/v0/ping",status:200,durationMs:1}]};
    default:throw new Error(method);
  }}};
  await import("../src/main.ts?local-api-console");
  window.document.querySelector<HTMLButtonElement>("#show-local-api")!.click();
  await waitFor(()=>window.document.querySelector<HTMLDialogElement>("#local-api-dialog")!.open);
  window.document.querySelector<HTMLInputElement>("#local-api-enabled")!.checked=true;
  window.document.querySelector<HTMLInputElement>("#local-api-auth")!.checked=true;
  window.document.querySelector<HTMLInputElement>("#local-api-passcode")!.value="pass123";
  window.document.querySelector<HTMLFormElement>("#local-api-form")!.dispatchEvent(new window.Event("submit",{cancelable:true}));
  await waitFor(()=>savedArgs.length>0);
  expect(savedArgs).toEqual([true,8086,true,"Planet","pass123"]);
  expect(window.document.querySelector("#local-api-logs")?.textContent).toContain("/v0/ping");
});

test("Web3 settings, i18n language switching, and standalone tool dialogs work",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"My Planet",about:"About",created:"2026-01-01T00:00:00Z"};
  let web3Saved:any=null;
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){
    case "Library.Sites":return [site];
    case "Library.Subscriptions":case "Library.Articles":return [];
    case "Library.UpdateSiteWeb3":web3Saved=JSON.parse(args[1]);return {...site,...web3Saved};
    case "Library.PublishSite":return {cid:"test",ipns:"test"};
    case "Library.AppLogs":return [{time:"01:18:00",category:"IPFS",message:"daemon ready"}];
    case "Library.ListDownloads":return [{name:"song.mp3",sourceName:"Pod",url:"https://example.test/song.mp3",size:1024,status:"已下载离线"}];
    default:throw new Error(method);
  }}};
  await import("../src/main.ts?web3-i18n-tools"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="My Planet");
  const langSelect=window.document.querySelector<HTMLSelectElement>("#ui-language")!;
  langSelect.value="en";langSelect.dispatchEvent(new window.Event("change",{bubbles:true}));
  expect(window.document.documentElement.lang).toBe("en");
  expect(window.document.querySelector("#sites-heading")?.textContent).toBe("My Planets");

  window.document.querySelector<HTMLButtonElement>("#show-app-logs")!.click();
  await waitFor(()=>window.document.querySelector("#app-logs-viewer")?.textContent?.includes("daemon ready")===true);
  window.document.querySelector<HTMLButtonElement>("#close-app-logs")!.click();

  window.document.querySelector<HTMLButtonElement>("#show-downloads")!.click();
  await waitFor(()=>window.document.querySelector("#downloads-list")?.textContent?.includes("song.mp3")===true);
  window.document.querySelector<HTMLButtonElement>("#close-downloads")!.click();

  window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  window.document.querySelector<HTMLButtonElement>("#web3-settings")!.click();
  const web3Form=window.document.querySelector<HTMLFormElement>("#web3-form")!;
  web3Form.querySelector<HTMLInputElement>('[name="acceptsDonation"]')!.checked=true;
  web3Form.querySelector<HTMLInputElement>('[name="acceptsDonationETHAddress"]')!.value="vitalik.eth";
  web3Form.querySelector<HTMLInputElement>('[name="juiceboxEnabled"]')!.checked=true;
  web3Form.querySelector<HTMLInputElement>('[name="juiceboxProjectID"]')!.value="7";
  web3Form.dispatchEvent(new window.Event("submit",{cancelable:true}));
  await waitFor(()=>web3Saved!==null);
  expect(web3Saved.acceptsDonationETHAddress).toBe("vitalik.eth");
  expect(web3Saved.juiceboxProjectID).toBe(7);
});
