import { enterFirstSite } from "./site-workflow";
import { expect, test } from "bun:test";
import { Window } from "happy-dom";

const waitFor = async (condition: () => boolean) => {
  for (let i = 0; i < 100; i++) {
    if (condition()) return;
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  throw new Error("Timed out waiting for the interface");
};

test("live article preview includes the current title as text and renders the body", async () => {
  const window = new Window({url:"http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document:window.document});
  const site = {id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  (globalThis as any).mygo = {call:async(method:string)=>{
    switch (method) {
      case "Library.Sites": return [site];
      case "Library.Subscriptions": case "Library.Articles": return [];
      case "Library.Preview": return "<p>正文预览</p>";
      default: throw new Error(method);
    }
  }};
  await import("../src/main.ts?live-preview-title"); enterFirstSite(window);
  await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#new-article")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#new-article")!.click();
  const title = window.document.querySelector<HTMLInputElement>("#title")!;
  title.value = '<img src=x onerror=alert(1)> & 正文';
  title.dispatchEvent(new window.Event("input"));
  const frame = window.document.querySelector<HTMLIFrameElement>("#preview")!;
  await waitFor(()=>frame.srcdoc.includes("&lt;img src=x onerror=alert(1)&gt; &amp; 正文"));
  expect(frame.srcdoc).toContain("<p>正文预览</p>");
  expect(frame.srcdoc).not.toContain("<img src=x onerror=alert(1)>");
});

test("writing mode collapses the article list and allows manual expansion before restoring browsing", async () => {
  const window = new Window({url:"http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document:window.document});
  const site = {id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  (globalThis as any).mygo = {call:async(method:string)=>{
    switch (method) {
      case "Library.Sites": return [site];
      case "Library.Subscriptions": case "Library.Articles": return [];
      case "Library.Preview": return "";
      default: throw new Error(method);
    }
  }};
  await import("../src/main.ts?writing-list-toggle"); enterFirstSite(window);
  await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#new-article")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#new-article")!.click();
  const toggle = window.document.querySelector<HTMLButtonElement>("#toggle-posts")!;
  const panel = window.document.querySelector<HTMLElement>("#posts-panel")!;
  expect(panel.hidden).toBe(true);
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  toggle.click();
  expect(panel.hidden).toBe(false);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  toggle.click();
  window.document.querySelector<HTMLButtonElement>("#show-favorites")!.click();
  expect(panel.hidden).toBe(false);
});

test("article list presents a safe content summary and guides selection", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  const article={id:"a",siteId:"s",title:"文章标题",content:"# 第一段\n这是一段正文内容",created:"2026-01-01T00:00:00Z",modified:"2026-01-02T00:00:00Z",tags:{}};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [article];default:throw new Error(method);}}};
  await import("../src/main.ts?article-list-summary"); enterFirstSite(window);
  await waitFor(()=>!!window.document.querySelector(".article-item"));
  expect(window.document.querySelector(".article-item .article-excerpt")?.textContent).toContain("这是一段正文内容");
  expect(window.document.querySelector("#empty h2")?.textContent).toContain("选择");
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  expect(window.document.querySelector<HTMLInputElement>("#title")!.value).toBe("文章标题");
});

test("reading layout identifies the real source and header compose opens the existing editor", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());
  window.happyDOM.setWindowSize({width:1280,height:800});
  Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"我的真实站点",about:"",created:"2026-01-01T00:00:00Z"};
  const article={id:"a",siteId:"s",title:"真实标题",content:"正文",created:"2026-01-01T00:00:00Z",modified:"2026-01-02T00:00:00Z",tags:{}};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [article];case "Library.Preview":return "";default:throw new Error(method);}}};
  await import("../src/main.ts?reading-layout-source"); enterFirstSite(window);
  await waitFor(()=>!!window.document.querySelector(".article-item"));
  expect(window.document.querySelector(".article-item .article-source")?.textContent).toBe("我的真实站点");
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  expect(window.document.querySelector("#own-reader-source")?.textContent).toBe("我的真实站点");
  const toggle=window.document.querySelector<HTMLButtonElement>("#toggle-posts")!;
  toggle.click();expect(window.document.querySelector<HTMLElement>("#posts-panel")!.hidden).toBe(true);
  toggle.click();expect(window.document.querySelector<HTMLElement>("#posts-panel")!.hidden).toBe(false);
  const compose=window.document.querySelector<HTMLButtonElement>("#new-article-header")!;
  expect(compose.hidden).toBe(false);compose.click();
  expect(window.document.querySelector<HTMLElement>("#editor")!.hidden).toBe(false);
  expect(window.document.querySelector<HTMLElement>("#preview")).not.toBeNull();
});

test("favorites view identifies itself and has a persistent selection indicator", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];default:throw new Error(method);}}};
  await import("../src/main.ts?favorites-layout-state"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
  const favorite=window.document.querySelector<HTMLButtonElement>("#show-favorites")!;favorite.click();
  expect(window.document.querySelector("#site-title")!.textContent).toBe("收藏的文章");
  expect(window.document.querySelector("#workspace-title")!.textContent).toBe("收藏的文章");
  expect(favorite.getAttribute("aria-pressed")).toBe("true");
  expect(window.document.querySelector<HTMLDetailsElement>("#workspace-menu")!.hidden).toBe(true);
  window.document.querySelector<HTMLButtonElement>("#sites .site-item")!.click();
  await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
  expect(favorite.getAttribute("aria-pressed")).toBe("false");
});

test("IPFS status panel displays owned node diagnostics and refreshes on demand", async () => {
  const window = new Window({url:"http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document:window.document});
  let reads = 0;
  (globalThis as any).mygo = {call:async(method:string)=>{
    switch (method) {
      case "Library.Sites": case "Library.Subscriptions": return [];
      case "Library.IPFSStatus": reads++; return {running:true,online:true,version:"0.30.0",peers:12,totalIn:2048,totalOut:4096,rateIn:512,rateOut:1024};
      default: throw new Error(method);
    }
  }};
  await import("../src/main.ts?ipfs-status-panel"); enterFirstSite(window);
  expect(reads).toBe(0);
  window.document.querySelector<HTMLButtonElement>("#show-ipfs-status")!.click();
  await waitFor(()=>reads===1 && window.document.querySelector("#ipfs-peer-count")?.textContent==="12");
  expect(window.document.querySelector("#ipfs-version")!.textContent).toContain("0.30.0");
  expect(window.document.querySelector("#ipfs-status-message")!.textContent).toContain("在线");
  window.document.querySelector<HTMLButtonElement>("#refresh-ipfs-status")!.click();
  await waitFor(()=>reads===2);
  window.document.querySelector<HTMLButtonElement>("#close-ipfs-status")!.click();
  expect(window.document.querySelector<HTMLDialogElement>("#ipfs-status-dialog")!.open).toBe(false);
});

test("IPFS status panel reports a stopped node and clears stale values", async () => {
  const window = new Window({url:"http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document:window.document});
  let reads = 0;
  (globalThis as any).mygo = {call:async(method:string)=>{
    switch(method) {
      case "Library.Sites": case "Library.Subscriptions": return [];
      case "Library.IPFSStatus": return ++reads === 1 ? {running:true,online:true,version:"0.30.0",peers:12,totalIn:0,totalOut:0,rateIn:0,rateOut:0} : {running:false,online:false,version:"",peers:0,totalIn:0,totalOut:0,rateIn:0,rateOut:0};
      default: throw new Error(method);
    }
  }};
  await import("../src/main.ts?ipfs-status-stopped"); enterFirstSite(window);
  window.document.querySelector<HTMLButtonElement>("#show-ipfs-status")!.click();
  await waitFor(()=>window.document.querySelector("#ipfs-peer-count")?.textContent==="12");
  window.document.querySelector<HTMLButtonElement>("#refresh-ipfs-status")!.click();
  await waitFor(()=>window.document.querySelector("#ipfs-status-message")!.textContent!.includes("尚未启动"));
  expect(window.document.querySelector("#ipfs-peer-count")!.textContent).toBe("—");
  expect(window.document.querySelector("#ipfs-version")!.textContent).toBe("—");
  window.document.querySelector<HTMLButtonElement>("#close-ipfs-status")!.click();
});

test("IPFS panel collects traffic samples and lets the user stop and restart the node", async () => {
  const window = new Window({url:"http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis,{window,document:window.document});
  window.confirm=()=>true;
  let enabled=true, reads=0;
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z",ipns:"k51example",lastPublishedCID:"bafyexample"};
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{
    switch(method){
      case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];
      case "Library.IPFSStatus":reads++;return {disabled:!enabled,running:enabled,online:enabled,version:"0.38.2",peers:3,rateIn:1024*reads,rateOut:256,totalIn:1024,totalOut:256,gateway:enabled?"http://127.0.0.1:12345":""};
      case "Library.SetIPFSEnabled":enabled=args[0];return null;
      default:throw new Error(method);
    }
  }};
  await import("../src/main.ts?ipfs-control-chart"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector<HTMLInputElement>("#published-url")?.value!=="");
  window.document.querySelector<HTMLButtonElement>("#show-ipfs-status")!.click();
  await waitFor(()=>reads===1 && window.document.querySelector("#ipfs-gateway")?.textContent?.includes("12345")===true);
  const link=window.document.querySelector<HTMLAnchorElement>("#ipfs-open-site")!;
  expect(link.href).toBe("http://127.0.0.1:12345/ipns/k51example/");
  const input=window.document.querySelector<HTMLInputElement>("#ipfs-resource")!;
  input.value="ipfs://bafy123/page";input.dispatchEvent(new window.Event("input"));
  expect(window.document.querySelector<HTMLAnchorElement>("#ipfs-open-resource")!.href).toBe("http://127.0.0.1:12345/ipfs/bafy123/page");
  window.document.querySelector<HTMLButtonElement>("#refresh-ipfs-status")!.click();
  await waitFor(()=>reads===2 && window.document.querySelectorAll("#ipfs-chart-in circle").length===2);
  window.document.querySelector<HTMLButtonElement>("#toggle-ipfs")!.click();
  await waitFor(()=>!enabled && window.document.querySelector("#ipfs-status-message")!.textContent!.includes("已关闭"));
  expect(link.hidden).toBe(true);
  window.document.querySelector<HTMLButtonElement>("#toggle-ipfs")!.click();
  await waitFor(()=>enabled && window.document.querySelector("#ipfs-status-message")!.textContent!.includes("在线"));
  window.document.querySelector<HTMLButtonElement>("#close-ipfs-status")!.click();
});

test("restarting IPFS retries saved pending publications", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis,{window,document:window.document});
  let enabled=false, published=0;
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z",publishPending:true};
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{
    switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];
      case "Library.IPFSStatus":return {disabled:!enabled,running:enabled,online:enabled,version:"0.38",peers:0,rateIn:0,rateOut:0,totalIn:0,totalOut:0,gateway:"http://127.0.0.1:1234"};
      case "Library.SetIPFSEnabled":enabled=args[0];return null;
      case "Library.PublishSite":if(!enabled)throw new Error("节点已关闭");published++;return {ipns:"k51test",cid:"bafytest",revision:1,pending:false};
      default:throw new Error(method);}
  }};
  await import("../src/main.ts?ipfs-resume-publish"); enterFirstSite(window);
  window.document.querySelector<HTMLButtonElement>("#show-ipfs-status")!.click();
  await waitFor(()=>window.document.querySelector("#ipfs-status-message")!.textContent!.includes("已关闭"));
  window.document.querySelector<HTMLButtonElement>("#toggle-ipfs")!.click();
  await waitFor(()=>published===1);
  window.document.querySelector<HTMLButtonElement>("#close-ipfs-status")!.click();
});

test("open IPFS panel automatically samples traffic", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis,{window,document:window.document});let reads=0;
  (globalThis as any).mygo={call:async(method:string)=>{
    switch(method){case "Library.Sites":case "Library.Subscriptions":return [];
      case "Library.IPFSStatus":reads++;return {disabled:false,running:true,online:true,version:"0.38",peers:1,rateIn:10,rateOut:20,totalIn:0,totalOut:0,gateway:"http://127.0.0.1:1234"};
      default:throw new Error(method);}
  }};
  await import("../src/main.ts?ipfs-automatic-sampling"); enterFirstSite(window);
  window.document.querySelector<HTMLButtonElement>("#show-ipfs-status")!.click();
  await waitFor(()=>reads===1);
  await new Promise(resolve=>setTimeout(resolve,5200));
  expect(reads).toBeGreaterThanOrEqual(2);
  expect(window.document.querySelectorAll("#ipfs-chart-in circle").length).toBeGreaterThanOrEqual(2);
  window.document.querySelector<HTMLButtonElement>("#close-ipfs-status")!.click();
}, 12000);

test("gateway selection updates and persists the published site address", async () => {
  const window = new Window({url:"http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis,{window,document:window.document});
  let copied = "";
  Object.defineProperty(window.navigator,"clipboard",{value:{writeText:async (value:string)=>{copied=value;}},configurable:true});
  const site={id:"site",name:"Site",about:"",created:"2026-01-01T00:00:00Z",ipns:"k51example",lastPublishedCID:"bafyexample"};
  (globalThis as any).mygo={call:async(method:string)=>{
    switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];default:throw new Error(method);}
  }};
  await import("../src/main.ts?gateway-selection"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector<HTMLInputElement>("#published-url")?.value!=="");
  const select=window.document.querySelector<HTMLSelectElement>("#public-gateway")!;
  expect(select).not.toBeNull();
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toBe("https://ipfs.io/ipns/k51example/");
  select.value="eth.sucks";select.dispatchEvent(new window.Event("change"));
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toBe("https://k51example.eth.sucks/");
  window.document.querySelector<HTMLButtonElement>("#copy-published-url")!.click();
  await waitFor(()=>copied!=="");expect(copied).toBe("https://k51example.eth.sucks/");
  expect(window.localStorage.getItem("planet.publicGateway")).toBe("eth.sucks");
  select.value="aleph";select.dispatchEvent(new window.Event("change"));
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toBe("https://ipfs.aleph.cloud/ipns/k51example/");
});

test("saved gateway is restored when the app starts", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());
  window.localStorage.setItem("planet.publicGateway","dweb.link");Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z",ipns:"k51example",lastPublishedCID:"bafyexample"};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];default:throw new Error(method);}}};
  await import("../src/main.ts?gateway-restored"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector<HTMLInputElement>("#published-url")?.value!=="");
  const select=window.document.querySelector<HTMLSelectElement>("#public-gateway")!;
  const choices=window.document.querySelector<HTMLElement>('.choice-group[data-select="public-gateway"]')!;
  window.document.querySelector<HTMLDetailsElement>(".sidebar-tools")!.open=true;
  expect(select.value).toBe("dweb.link");
  expect(select.hidden).toBe(true);
  expect(choices.querySelectorAll("input:checked")).toHaveLength(1);
  expect(choices.querySelector<HTMLInputElement>("input:checked")!.value).toBe(select.value);
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toBe("https://dweb.link/ipns/k51example/");
  expect(window.localStorage.getItem("planet.publicGateway")).toBe("dweb.link");
  choices.querySelector<HTMLInputElement>('input[value="aleph"]')!.click();
  expect(select.value).toBe("aleph");
  expect(choices.querySelectorAll("input:checked")).toHaveLength(1);
  expect(choices.querySelector<HTMLInputElement>("input:checked")!.value).toBe("aleph");
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toBe("https://ipfs.aleph.cloud/ipns/k51example/");
  expect(window.localStorage.getItem("planet.publicGateway")).toBe("aleph");
});

test("unsupported eth.sucks identity is not copied as a misleading address", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());
  window.localStorage.setItem("planet.publicGateway","eth.sucks");Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z",ipns:"QmLegacyIdentity",lastPublishedCID:"bafyexample"};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];default:throw new Error(method);}}};
  await import("../src/main.ts?gateway-unsupported"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector("#copy-status")?.textContent?.includes("暂不支持")===true);
  expect(window.document.querySelector<HTMLButtonElement>("#copy-published-url")!.disabled).toBe(true);
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toBe("");
});

test("copying a Planet subscription article uses the selected gateway", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const post={id:"post",key:"post",title:"Article",link:"https://ipfs.io/ipns/k51example/post/",description:"",content:"Text",published:"",contentCid:"bafy"};
  const sub={url:"k51example",lastUpdated:new Date().toISOString(),feed:{title:"Planet",description:"",link:"",items:[post]}};let copied="";
  Object.defineProperty(window.navigator,"clipboard",{value:{writeText:async(value:string)=>{copied=value;}},configurable:true});
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return [sub];case "Library.SubscriptionPreview":return "Text";default:throw new Error(method);}}};
  await import("../src/main.ts?gateway-planet-share"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector("#subscriptions .site-item"));
  const select=window.document.querySelector<HTMLSelectElement>("#public-gateway")!;select.value="eth.sucks";select.dispatchEvent(new window.Event("change"));
  window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  window.document.querySelector<HTMLButtonElement>("#share-item")!.click();await waitFor(()=>copied!=="");
  expect(copied).toBe("https://k51example.eth.sucks/post/");
});

test("Filebase and Plausible credentials are configured separately and stats can be queried", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let saved:any,statsArgs:any;
 (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.ServiceInfo":return {configured:false,name:""};case "Library.ConfigurePlausibleAt":saved=args;return;case "Library.PlausibleStatsRange":statsArgs=args;return {visitors:12,pageviews:34};default:throw new Error(method)}}};
 await import("../src/main.ts?service-settings"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-settings")!.disabled);
 window.document.querySelector<HTMLButtonElement>("#service-settings")!.click();await waitFor(()=>window.document.querySelector<HTMLDialogElement>("#service-dialog")!.open);
 const form=window.document.querySelector<HTMLFormElement>("#service-form")!;form.querySelector<HTMLSelectElement>('[name="kind"]')!.value="plausible";form.querySelector<HTMLSelectElement>('[name="kind"]')!.dispatchEvent(new window.Event("change"));
 await new Promise(resolve=>setTimeout(resolve,20));form.querySelector<HTMLInputElement>('[name="name"]')!.value="example.com";form.querySelector<HTMLInputElement>('[name="token"]')!.value="secret";form.querySelector<HTMLInputElement>('[name="scriptId"]')!.value="pa-Test123";form.querySelector<HTMLInputElement>('[name="server"]')!.value="https://stats.example.com";
 form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!saved);expect(saved).toEqual(["s","example.com","secret","pa-Test123","https://stats.example.com"]);
 await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-stats")!.disabled);const range=form.querySelector<HTMLSelectElement>('[name="statsRange"]')!;range.value="day";window.document.querySelector<HTMLButtonElement>("#service-stats")!.click();await waitFor(()=>window.document.querySelector("#service-result")?.textContent?.includes("34")===true);expect(statsArgs).toEqual(["s","day"]);expect(window.document.querySelector("#service-result")?.textContent).toContain("今天");
 expect(form.querySelector<HTMLInputElement>('[name="token"]')!.value).toBe("");
});

test("Filebase can retry a pin and query remote status", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",lastPublishedCID:"bafy-current",created:"2026-01-01T00:00:00Z"};let requests=0;
 (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.ServiceInfo":return {configured:true,name:"site"};case "Library.RequestFilebasePin":requests++;return;case "Library.FilebaseStatus":return {status:"queued",pin:{cid:"bafy-current"}};default:throw new Error(method)}}};
 window.confirm=()=>true;await import("../src/main.ts?filebase-status"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-settings")!.disabled);window.document.querySelector<HTMLButtonElement>("#service-settings")!.click();
 await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-pin")!.disabled);window.document.querySelector<HTMLButtonElement>("#service-pin")!.click();await waitFor(()=>requests===1);
 await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-pin-status")!.disabled);window.document.querySelector<HTMLButtonElement>("#service-pin-status")!.click();await waitFor(()=>window.document.querySelector("#service-result")?.textContent?.includes("queued")===true);
});

test("partial Plausible disable reports the site needs republication", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};window.confirm=()=>true;
 (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.ServiceInfo":return {configured:false,name:""};case "Library.ConfigureService":throw new Error("站点跟踪已停用，但本机服务凭据未能清理，请重试");default:throw new Error(method)}}};
 await import("../src/main.ts?plausible-partial"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-settings")!.disabled);window.document.querySelector<HTMLButtonElement>("#service-settings")!.click();
 const kind=window.document.querySelector<HTMLSelectElement>('#service-form [name="kind"]')!;await waitFor(()=>!kind.disabled);kind.value="plausible";kind.dispatchEvent(new window.Event("change"));
 await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-disable")!.disabled);window.document.querySelector<HTMLButtonElement>("#service-disable")!.click();await waitFor(()=>window.document.querySelector("#service-result")?.textContent?.includes("凭据未能清理")===true);
 expect(window.document.querySelector("#save-state")?.textContent).toContain("站点更改尚未发布");
});

test("Plausible tracking can be configured without a stats API key", async()=>{
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let saved:any;
 (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.ServiceInfo":return {configured:false,name:""};case "Library.ConfigurePlausibleAt":saved=args;return;default:throw new Error(method)}}};
 await import("../src/main.ts?plausible-no-key"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-settings")!.disabled);window.document.querySelector<HTMLButtonElement>("#service-settings")!.click();const form=window.document.querySelector<HTMLFormElement>("#service-form")!,kind=form.querySelector<HTMLSelectElement>('[name="kind"]')!;await waitFor(()=>!kind.disabled);kind.value="plausible";kind.dispatchEvent(new window.Event("change"));await waitFor(()=>!kind.disabled);
 form.querySelector<HTMLInputElement>('[name="name"]')!.value="example.com";form.querySelector<HTMLInputElement>('[name="scriptId"]')!.value="pa-Test123";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!saved);expect(saved).toEqual(["s","example.com","","pa-Test123","https://plausible.io"]);expect(window.document.querySelector<HTMLButtonElement>("#service-stats")!.disabled).toBe(true);
});

test("unsaved Plausible destination or key cannot query saved credentials", async()=>{
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let calls=0;
 (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.ServiceInfo":return {configured:true,name:"example.com",server:"https://stats.example.com"};case "Library.PlausibleStatsRange":calls++;return {visitors:1,pageviews:1};default:throw new Error(method)}}};
 await import("../src/main.ts?plausible-unsaved-destination"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#service-settings")!.disabled);window.document.querySelector<HTMLButtonElement>("#service-settings")!.click();
 const form=window.document.querySelector<HTMLFormElement>("#service-form")!,kind=form.querySelector<HTMLSelectElement>('[name="kind"]')!;await waitFor(()=>!kind.disabled);kind.value="plausible";kind.dispatchEvent(new window.Event("change"));const stats=window.document.querySelector<HTMLButtonElement>("#service-stats")!;await waitFor(()=>!stats.disabled);
 stats.click();await waitFor(()=>calls===1 && !stats.disabled);expect(window.document.querySelector("#service-result")?.textContent).toContain("访客 1");
 const server=form.querySelector<HTMLInputElement>('[name="server"]')!;server.value="https://other.example";server.dispatchEvent(new window.Event("input"));expect(stats.disabled).toBe(true);expect(window.document.querySelector("#service-result")?.textContent).not.toContain("访客 1");stats.click();expect(calls).toBe(1);
 server.value="https://stats.example.com";server.dispatchEvent(new window.Event("input"));expect(stats.disabled).toBe(false);const token=form.querySelector<HTMLInputElement>('[name="token"]')!;token.value="new-key";token.dispatchEvent(new window.Event("input"));expect(stats.disabled).toBe(true);
});

test("Sepia template settings load and save without publishing", async () => {
	const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
	const site={id:"s",name:"Site",about:"",templateId:"template-sepia",templateName:"Sepia",created:"2026-01-01T00:00:00Z"};let sent:any,publications=0;
	(globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
		case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];
		case "Library.TemplateSettings":return {backgroundColor:"#123456",showRef:"true"};
		case "Library.SaveTemplateSettings":sent=JSON.parse(args[1]!);return {...site,templateSettings:sent,contentRevision:1};
		case "Library.PublishSite":publications++;return {};default:throw new Error(method);
	}}};
	await import("../src/main.ts?sepia-settings"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#template-settings")!.disabled);
	window.document.querySelector<HTMLButtonElement>("#template-settings")!.click();await waitFor(()=>window.document.querySelector<HTMLDialogElement>("#template-settings-dialog")!.open);
	const form=window.document.querySelector<HTMLFormElement>("#template-settings-form")!,background=form.querySelector<HTMLInputElement>('[name="backgroundColor"]')!;
	expect(background.value).toBe("#123456");background.value="#654321";
	form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!sent);
	expect(sent.backgroundColor).toBe("#654321");expect(sent.showRef).toBe("true");expect(publications).toBe(0);
});

test("pending template settings save prevents conflicting edits and cancellation", async () => {
	const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
	const site={id:"s",name:"Site",about:"",templateId:"template-sepia",templateName:"Sepia",created:"2026-01-01T00:00:00Z"};let release:(()=>void)|undefined;
	(globalThis as any).mygo={call:async(method:string)=>{switch(method){
		case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];
		case "Library.TemplateSettings":return {backgroundColor:"#123456",showRef:"false"};
		case "Library.SaveTemplateSettings":await new Promise<void>(resolve=>{release=resolve});return {...site,contentRevision:1};
		default:throw new Error(method);
	}}};
	await import("../src/main.ts?sepia-settings-pending"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#template-settings")!.disabled);
	window.document.querySelector<HTMLButtonElement>("#template-settings")!.click();await waitFor(()=>window.document.querySelector<HTMLDialogElement>("#template-settings-dialog")!.open);
	const form=window.document.querySelector<HTMLFormElement>("#template-settings-form")!,dialog=window.document.querySelector<HTMLDialogElement>("#template-settings-dialog")!;
	form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!release);
	expect(form.querySelector<HTMLInputElement>('[name="backgroundColor"]')!.disabled).toBe(true);
	expect(window.document.querySelector<HTMLButtonElement>("#cancel-template-settings")!.disabled).toBe(true);
	const cancel=new window.Event("cancel",{cancelable:true});dialog.dispatchEvent(cancel);expect(cancel.defaultPrevented).toBe(true);
	release!();await waitFor(()=>!dialog.open);
});

test("Pinnable configuration requires explicit input and shows pin status", async () => {
	const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
	const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let configured=false,saved="",requests=0;
	(globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
		case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];
		case "Library.PinnableConfigured":return configured;
		case "Library.ConfigurePinnable":saved=args[1]!;configured=!!saved;return;
		case "Library.RequestPinnablePin":requests++;return;
		case "Library.PinnableStatus":return {status:"pinned",last_known_cid:"bafytest"};
		default:throw new Error(method);
	}}};
	await import("../src/main.ts?pinnable-settings"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#pinnable-settings")!.disabled);
	window.document.querySelector<HTMLButtonElement>("#pinnable-settings")!.click();await waitFor(()=>window.document.querySelector<HTMLDialogElement>("#pinnable-dialog")!.open);
	expect(requests).toBe(0);const form=window.document.querySelector<HTMLFormElement>("#pinnable-form")!;
	form.querySelector<HTMLInputElement>('[name="endpoint"]')!.value="https://pinnable.xyz/pin/test-123";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));
	await waitFor(()=>configured);expect(saved).toBe("https://pinnable.xyz/pin/test-123");expect(requests).toBe(0);
	await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#pinnable-status")!.disabled);
	window.document.querySelector<HTMLButtonElement>("#pinnable-status")!.click();await waitFor(()=>window.document.querySelector("#pinnable-result")?.textContent?.includes("bafytest")===true);
	expect(requests).toBe(0);
});

test("Pinnable failure is shown without claiming IPNS publication failed", async () => {
	const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
	const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
	(globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
		case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.Preview":case "Library.PreviewArticle":return "";
		case "Library.SaveArticle":return {id:"a",siteId:"s",title:args[2],content:args[3],created:site.created,modified:site.created};
		case "Library.PublishSite":return {ipns:"k51test",cid:"bafytest",revision:1,pending:false,pinnableWarning:"站点已发布，但 Pinnable 固定请求失败"};
		default:throw new Error(method);
	}}};
	await import("../src/main.ts?pinnable-warning"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#new-article")!.disabled);
	window.document.querySelector<HTMLButtonElement>("#new-article")!.click();window.document.querySelector<HTMLInputElement>("#title")!.value="Test";window.document.querySelector<HTMLButtonElement>("#save")!.click();
	await waitFor(()=>window.document.querySelector("#error")?.textContent?.includes("Pinnable 固定请求失败")===true);
	expect(window.document.querySelector("#site-address")?.textContent).toContain("k51test");expect(window.document.querySelector("#error")?.textContent).not.toContain("站点发布未确认");
});

test("Pinnable status warns when service identity or version differs", async () => {
	const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
	const site={id:"s",name:"Site",about:"",ipns:"k51site",lastPublishedCID:"bafycurrent",created:"2026-01-01T00:00:00Z"};
	(globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.PinnableConfigured":return true;case "Library.PinnableStatus":return {status:"pinned",last_known_ipns:"k51other",last_known_cid:"bafyold"};default:throw new Error(method);}}};
	await import("../src/main.ts?pinnable-mismatch"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#pinnable-settings")!.disabled);
	window.document.querySelector<HTMLButtonElement>("#pinnable-settings")!.click();await waitFor(()=>window.document.querySelector<HTMLDialogElement>("#pinnable-dialog")!.open);
	window.document.querySelector<HTMLButtonElement>("#pinnable-status")!.click();await waitFor(()=>window.document.querySelector("#pinnable-result")?.textContent?.includes("IPNS 与当前站点不一致")===true);
	expect(window.document.querySelector("#pinnable-result")?.textContent).toContain("不是当前发布版本");expect(window.document.querySelector("#pinnable-result")?.textContent).not.toContain("当前版本已固定");
});

test("permanent site deletion requires the site ID and preserves other sites", async () => {
	const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
	const removed={id:"delete",name:"Old site",about:"",removed:true,created:"2026-01-01T00:00:00Z"},keep={id:"keep",name:"Keep site",about:"",created:removed.created};
	let items=[removed],requested="",promptText="",answer="wrong";
	(globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
		case "Library.Sites":return [keep];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.RemovedSites":return items;
		case "Library.PermanentlyDeleteSite":requested=args[0]!;items=[];return;default:throw new Error(method);
	}}};
	window.prompt=(message?:string)=>{promptText=message ?? "";return answer;};
	await import("../src/main.ts?permanent-delete"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#sites")?.textContent?.includes("Keep site")===true);
	window.document.querySelector<HTMLButtonElement>("#show-removed-sites")!.click();await waitFor(()=>window.document.querySelector("#removed-site-list")?.textContent?.includes("Old site")===true);
	const button=()=>Array.from(window.document.querySelectorAll<HTMLButtonElement>("#removed-site-list button")).find(b=>b.textContent==="永久删除")!;
	button().click();await new Promise(r=>setTimeout(r,20));expect(requested).toBe("");expect(promptText).toContain("私钥");
	answer="delete";button().click();await waitFor(()=>requested==="delete");expect(window.document.querySelector("#sites")?.textContent).toContain("Keep site");
});

test("interrupted permanent deletion offers retry instead of restore", async () => {
	const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
	const entry={id:"delete",name:"Old site",about:"",removed:true,created:"2026-01-01T00:00:00Z"};let pending=false,calls=0;
	(globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":case "Library.Subscriptions":return [];case "Library.RemovedSites":return [{...entry,deletionPending:pending}];
		case "Library.PermanentlyDeleteSite":calls++;if(calls===1){pending=true;throw new Error("file locked")};return;default:throw new Error(method);}}};
	window.prompt=()=>"delete";await import("../src/main.ts?permanent-delete-retry"); enterFirstSite(window);window.document.querySelector<HTMLButtonElement>("#show-removed-sites")!.click();
	await waitFor(()=>window.document.querySelector("#removed-site-list")?.textContent?.includes("Old site")===true);
	Array.from(window.document.querySelectorAll<HTMLButtonElement>("#removed-site-list button")).find(b=>b.textContent==="永久删除")!.click();
	await waitFor(()=>window.document.querySelector("#removed-site-list")?.textContent?.includes("继续清理")===true);
	const restore=Array.from(window.document.querySelectorAll<HTMLButtonElement>("#removed-site-list button")).find(b=>b.textContent==="恢复")!;expect(restore.hidden).toBe(true);
	Array.from(window.document.querySelectorAll<HTMLButtonElement>("#removed-site-list button")).find(b=>b.textContent==="继续清理")!.click();await waitFor(()=>calls===2);
});

test("saved drafts accept private attachments without publishing", async () => {
  const window = new Window({url:"http://localhost/"}); window.document.write(await Bun.file("index.html").text()); Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  let draft:any={id:"d",siteId:"s",title:"Draft",content:"",created:site.created,modified:site.created}, publications=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){
    case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];
    case "Library.Drafts":return [draft];case "Library.Preview":case "Library.PreviewDraft":return "";
    case "Library.AddDraftAttachment":draft={...draft,attachments:["private image.png"]};return draft;
    case "Library.RemoveDraftAttachment":draft={...draft,attachments:[]};return draft;
    case "Library.PublishSite":publications++;return {};
    default:throw new Error(method);
  }}};
  window.confirm=()=>true;
  await import("../src/main.ts?draft-attachments"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#show-drafts")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#show-drafts")!.click();await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("Draft")===true);
  window.document.querySelector<HTMLButtonElement>("#articles button")!.click();
  const add=window.document.querySelector<HTMLButtonElement>("#add-attachment")!;expect(add.disabled).toBe(false);add.click();
  await waitFor(()=>window.document.querySelector("#attachment-list")?.textContent?.includes("private image.png")===true);
  expect(window.document.querySelector<HTMLTextAreaElement>("#content")!.value).toContain("private%20image.png");
  expect(publications).toBe(0);window.document.querySelector<HTMLButtonElement>("#attachment-list .remove-attachment")!.click();
  await waitFor(()=>!window.document.querySelector("#attachment-list")?.textContent?.includes("private image.png"));expect(publications).toBe(0);
});

test("draft can stage and undo removal of an existing article attachment", async () => {
  const window = new Window({url:"http://localhost/"}); window.document.write(await Bun.file("index.html").text()); Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  const post={id:"a",siteId:"s",title:"Post",content:"",attachments:["old.png"],created:site.created,modified:site.created};
  let draft:any={id:"d",siteId:"s",articleId:"a",title:"Draft",content:"",created:site.created,modified:site.created}, publications=0;
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){
    case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [post];case "Library.Drafts":return [draft];
    case "Library.Preview":case "Library.PreviewArticle":case "Library.PreviewDraft":return "";
    case "Library.SetDraftExistingAttachmentRemoved":draft={...draft,removedAttachments:args[3]?[args[2]]:[]};return draft;
    case "Library.PublishSite":publications++;return {};default:throw new Error(method);
  }}};window.confirm=()=>true;
  await import("../src/main.ts?draft-existing-attachment"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#show-drafts")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#show-drafts")!.click();await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("Draft")===true);
  window.document.querySelector<HTMLButtonElement>("#articles button")!.click();
  const button=()=>window.document.querySelector<HTMLButtonElement>("#attachment-list .remove-attachment")!;
  expect(button().disabled).toBe(false);button().click();await waitFor(()=>button().textContent==="撤销");
  expect(publications).toBe(0);button().click();await waitFor(()=>button().textContent==="×");expect(publications).toBe(0);
});

test("unfollowing a subscription removes it after confirmation", async () => {
  const window = new Window({url:"http://localhost/"}); window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window,document:window.document});
  const sub={url:"https://example.test/feed",lastUpdated:new Date().toISOString(),feed:{title:"News",items:[{key:"one",title:"Post",starred:true}]}};
  let subscriptions=[sub], confirmation="";
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{
    switch(method) {
      case "Library.Sites":return [];
      case "Library.Subscriptions":return subscriptions;
      case "Library.Unfollow": subscriptions=subscriptions.filter(s=>s.url!==args[0]);return;
      default:throw new Error(`Unexpected call: ${method}`);
    }
  }};
  window.confirm=(message?:string)=>{confirmation=message ?? "";return true;};
  await import("../src/main.ts?unfollow"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector("#subscriptions")?.textContent?.includes("News")===true);
  window.document.querySelector<HTMLButtonElement>("#subscriptions button")!.click();
  const remove=window.document.querySelector<HTMLButtonElement>("#unfollow");
  expect(remove).not.toBeNull();
  remove!.click();
  await waitFor(()=>!window.document.querySelector("#subscriptions")?.textContent?.includes("News"));
  expect(confirmation).toContain("收藏");
});

test("a late refresh response cannot restore an unfollowed subscription in the sidebar", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const sub={url:"https://example.test/feed",lastUpdated:new Date().toISOString(),feed:{title:"News",items:[]}};
  let exists=true, calls=0, release:(()=>void)|undefined;
  (globalThis as any).mygo={call:async(method:string)=>{
    switch(method){
      case "Library.Sites":return [];case "Library.Subscriptions": {calls++;if(calls===2){await new Promise<void>(resolve=>{release=resolve});return [sub];}return exists?[sub]:[];}
      case "Library.RefreshSubscription":return sub;case "Library.Unfollow":exists=false;return;
      default:throw new Error(`Unexpected call: ${method}`);
    }
  }};
  window.confirm=()=>true;await import("../src/main.ts?unfollow-race"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector("#subscriptions")?.textContent?.includes("News")===true);
  window.document.querySelector<HTMLButtonElement>("#subscriptions button")!.click();
  window.document.querySelector<HTMLButtonElement>("#refresh-feed")!.click();await waitFor(()=>!!release);
  window.document.querySelector<HTMLButtonElement>("#unfollow")!.click();await waitFor(()=>!window.document.querySelector("#subscriptions")?.textContent?.includes("News"));
  release!();await new Promise(resolve=>setTimeout(resolve,40));expect(window.document.querySelector("#subscriptions")?.textContent).not.toContain("News");
});

test("newest subscription reload also updates the selected reader", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const oldA={url:"https://a.test/feed",lastUpdated:new Date().toISOString(),feed:{title:"Feed A",items:[{key:"one",title:"Old title",content:"Old"}]}},
    newA={...oldA,feed:{title:"Feed A",items:[{key:"one",title:"New title",content:"New"}]}}, b={url:"https://b.test/feed",lastUpdated:new Date().toISOString(),feed:{title:"Feed B",items:[]}};
  let calls=0,releaseOld:(()=>void)|undefined,releaseNew:(()=>void)|undefined;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":{calls++;if(calls===2){await new Promise<void>(r=>{releaseOld=r});return [oldA,b];}if(calls===3){await new Promise<void>(r=>{releaseNew=r});return [newA,b];}return [oldA,b];}
    case "Library.RefreshSubscription":return newA;case "Library.PinSubscription":return;default:throw new Error(method);}}};
  await import("../src/main.ts?subscription-reload-selection"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#subscriptions")?.textContent?.includes("Feed A")===true);
  const pick=(name:string)=>Array.from(window.document.querySelectorAll<HTMLButtonElement>("#subscriptions button")).find(x=>x.textContent?.includes(name))!.click();
  pick("Feed A");window.document.querySelector<HTMLButtonElement>("#refresh-feed")!.click();await waitFor(()=>!!releaseOld);
  pick("Feed B");window.document.querySelector<HTMLButtonElement>("#pin-subscription")!.click();await waitFor(()=>!!releaseNew);
  pick("Feed A");window.document.querySelector<HTMLButtonElement>("#articles button")!.click();expect(window.document.querySelector("#reader-title")?.textContent).toBe("Old title");
  releaseNew!();await waitFor(()=>window.document.querySelector("#subscriptions")?.textContent?.includes("Feed A")===true);await new Promise(r=>setTimeout(r,20));releaseOld!();await new Promise(r=>setTimeout(r,20));
  expect(window.document.querySelector("#reader-title")?.textContent).toBe("New title");
});

test("saving a new draft keeps previously saved drafts visible", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  let drafts:any[]=[{id:"a",siteId:"s",title:"Existing draft",content:"",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"}];
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{
    switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.Preview":return "";case "Library.Drafts":return drafts;
      case "Library.SaveDraft":{const d={id:"b",siteId:"s",title:args[3],content:args[4],created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};drafts=[...drafts,d];return d;}
      default:throw new Error(`Unexpected call: ${method}`);}
  }};
  await import("../src/main.ts?draft-existing"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#new-article")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#new-article")!.click();window.document.querySelector<HTMLInputElement>("#title")!.value="New draft";
  window.document.querySelector<HTMLButtonElement>("#save-draft")!.click();await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("New draft")===true);
  expect(window.document.querySelector("#articles")?.textContent).toContain("Existing draft");
});

test("late initialization cannot redirect a new site's article to an older site", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const old={id:"old",name:"Old",about:"",created:"2026-01-01T00:00:00Z"},created={id:"new",name:"New",about:"",created:"2026-01-01T00:00:00Z"};
  let sites=[old], release:(()=>void)|undefined, savedSite="";
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{
    switch(method){case "Library.Sites":return [...sites];case "Library.Subscriptions":await new Promise<void>(resolve=>{release=resolve});return [];
      case "Library.Articles":return [];case "Library.CreateSite":sites=[old,created];return created;
      case "Library.Preview":case "Library.PreviewArticle":return "";
      case "Library.SaveArticle":savedSite=args[0]!;return {id:"post",siteId:args[0],title:args[2],content:args[3],created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
      case "Library.PublishSite":return {ipns:"name",cid:"cid",revision:1,pending:false};
      default:throw new Error(`Unexpected call: ${method}`);}
  }};
  await import("../src/main.ts?init-site-race"); enterFirstSite(window);await waitFor(()=>!!release);
  window.document.querySelector<HTMLButtonElement>("#new-site")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('input[name="name"]')!.value="New";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));
  await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="New");window.document.querySelector<HTMLButtonElement>("#new-article")!.click();
  const title=window.document.querySelector<HTMLInputElement>("#title")!;title.value="New post";title.dispatchEvent(new window.Event("input"));
  release!();await new Promise(resolve=>setTimeout(resolve,40));window.document.querySelector<HTMLButtonElement>("#save")!.click();await waitFor(()=>savedSite!=="");
  expect(savedSite).toBe("new");
});

test("late initial article list cannot appear under a newly created site", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const old={id:"old",name:"Old",about:"",created:"2026-01-01T00:00:00Z"},created={id:"new",name:"New",about:"",created:"2026-01-01T00:00:00Z"};
  let sites=[old],release:(()=>void)|undefined;
  const oldPost={id:"old-post",siteId:"old",title:"Wrong site's post",content:"",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{
    switch(method){case "Library.Sites":return [...sites];case "Library.Subscriptions":return [];
      case "Library.Articles":if(args[0]==="old"){await new Promise<void>(resolve=>{release=resolve});return [oldPost];}return [];
      case "Library.CreateSite":sites=[old,created];return created;case "Library.Preview":return "";
      default:throw new Error(`Unexpected call: ${method}`);}
  }};
  await import("../src/main.ts?init-posts-race"); enterFirstSite(window);await waitFor(()=>!!release);
  window.document.querySelector<HTMLButtonElement>("#new-site")!.click();const form=window.document.querySelector<HTMLFormElement>("#site-form")!;
  form.querySelector<HTMLInputElement>('input[name="name"]')!.value="New";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));
  await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="New");release!();await new Promise(resolve=>setTimeout(resolve,40));
  expect(window.document.querySelector("#articles")?.textContent).not.toContain("Wrong site's post");
});

test("site settings update persists and marks the site unpublished", async () => {
  const window=new Window({url:"http://localhost/"}); window.document.write(await Bun.file("index.html").text()); Object.assign(globalThis,{window,document:window.document});
  let site={id:"site-1",name:"Old",about:"Before",created:"2026-01-01T00:00:00Z",contentRevision:0,publishedRevision:0};
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{
    switch(method){
      case "Library.Sites":return [site]; case "Library.Subscriptions":case "Library.Articles":return [];
      case "Library.UpdateSite":site={...site,name:args[1]!,about:args[2]!,contentRevision:1};return site;
      default:throw new Error(`Unexpected call: ${method}`);
    }
  }};
  await import("../src/main.ts?site-settings"); enterFirstSite(window); await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Old");
  const button=window.document.querySelector<HTMLButtonElement>("#edit-site"); expect(button).not.toBeNull(); button!.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!;
  const name=form.querySelector<HTMLInputElement>('input[name="name"]')!, about=form.querySelector<HTMLTextAreaElement>('textarea[name="about"]')!;
  expect(name.value).toBe("Old"); name.value="New";about.value="After";
  form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));
  try { await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="New"); }
  catch { throw new Error(`site update failed: ${window.document.querySelector("#error")?.textContent}; ${JSON.stringify(site)}`); }
  expect(window.document.querySelector("#save-state")?.textContent).toContain("尚未发布");
});

test("pending site settings prevent cancellation and conflicting template changes", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const initial={id:"s",name:"Imported",about:"",created:"2026-01-01T00:00:00Z",imported:true};let release:(()=>void)|undefined,selected=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [initial];case "Library.Subscriptions":case "Library.Articles":return [];
    case "Library.UpdateSite":await new Promise<void>(r=>{release=r});return {...initial,name:"Updated",contentRevision:1};case "Library.SelectTemplate":selected++;return {...initial,templateId:"template-1",templateName:"Plain",contentRevision:2};default:throw new Error(method);}}};
  await import("../src/main.ts?settings-concurrent"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Imported");
  window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('input[name="name"]')!.value="Updated";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!release);
  expect(window.document.querySelector<HTMLButtonElement>("#cancel-site")!.disabled).toBe(true);expect(window.document.querySelector<HTMLButtonElement>("#select-template")!.disabled).toBe(true);
  expect(form.querySelector<HTMLInputElement>('input[name="name"]')!.disabled).toBe(true);expect(form.querySelector<HTMLTextAreaElement>('textarea[name="about"]')!.disabled).toBe(true);
  window.document.querySelector<HTMLButtonElement>("#cancel-site")!.click();window.document.querySelector<HTMLButtonElement>("#select-template")!.click();expect(selected).toBe(0);
  release!();await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Updated");
});

test("same-revision publication address survives a late site settings response", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const initial={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z",contentRevision:0,publishedRevision:0};let releaseSettings:(()=>void)|undefined,releasePublish:(()=>void)|undefined,publishes=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [initial];case "Library.Subscriptions":case "Library.Articles":return [];
    case "Library.UpdateSite":await new Promise<void>(r=>{releaseSettings=r});return {...initial,name:"Updated",contentRevision:1,publishedRevision:0,publishPending:true};
    case "Library.PublishSite":publishes++;if(publishes===1){await new Promise<void>(r=>{releasePublish=r});return {ipns:"k51",cid:"bafy0",revision:0,pending:true};}throw new Error("second publish failed");default:throw new Error(method);}}};
  window.confirm=()=>true;window.alert=()=>{};await import("../src/main.ts?settings-equal-revision"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
  window.document.querySelector<HTMLButtonElement>("#publish-site")!.click();await waitFor(()=>!!releasePublish);window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();
  const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('input[name="name"]')!.value="Updated";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!releaseSettings);
  releasePublish!();await waitFor(()=>publishes===2);await waitFor(()=>window.document.querySelector<HTMLInputElement>("#published-url")!.value.includes("k51"));releaseSettings!();await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Updated");
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toContain("k51");
});

test("late site settings response keeps a newer publication result", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const initial={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z",contentRevision:0,publishedRevision:0};let releaseSettings:(()=>void)|undefined,releasePublish:(()=>void)|undefined,publishes=0;
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [initial];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.Preview":case "Library.PreviewArticle":return "";
    case "Library.SaveArticle":return {id:"p",siteId:"s",title:args[2],content:args[3],created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
    case "Library.UpdateSite":await new Promise<void>(r=>{releaseSettings=r});return {...initial,name:"Updated",contentRevision:2,publishedRevision:0,publishPending:true};
    case "Library.PublishSite":publishes++;if(publishes===1){await new Promise<void>(r=>{releasePublish=r});return {ipns:"k51",cid:"bafy1",revision:1,pending:true};}return {ipns:"k51",cid:"bafy2",revision:2,pending:false};
    default:throw new Error(method);}}};
  await import("../src/main.ts?settings-publication-merge"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
  window.document.querySelector<HTMLButtonElement>("#new-article")!.click();const title=window.document.querySelector<HTMLInputElement>("#title")!;title.value="Post";title.dispatchEvent(new window.Event("input"));window.document.querySelector<HTMLButtonElement>("#save")!.click();await waitFor(()=>!!releasePublish);
  await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#edit-site")!.disabled);window.document.querySelector<HTMLButtonElement>("#edit-site")!.click();const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('input[name="name"]')!.value="Updated";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!releaseSettings);
  releasePublish!();await waitFor(()=>publishes===2);await waitFor(()=>window.document.querySelector<HTMLInputElement>("#published-url")!.value.includes("k51"));
  releaseSettings!();await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Updated");
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toContain("k51");expect(window.document.querySelector("#save-state")?.textContent).toContain("已发布");
});

test("site removal is recoverable and does not republish on restore", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z",ipns:"k51",contentRevision:1,publishedRevision:0,publishPending:false};
  const article={id:"p",siteId:"s",title:"Saved article",content:"Content",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
  let removed=false,publishes=0,confirmation="";
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){
    case "Library.Sites":return removed?[]:[site];case "Library.Subscriptions":return [];case "Library.Articles":return [article];
    case "Library.RemoveSite":removed=true;return;case "Library.RemovedSites":return removed?[site]:[];case "Library.RestoreSite":removed=false;return site;
    case "Library.PublishSite":publishes++;return {ipns:"k51",cid:"bafy",revision:1,pending:false};default:throw new Error(method);
  }}};
  window.confirm=(message?:string)=>{confirmation=message ?? "";return true;};await import("../src/main.ts?site-removal"); enterFirstSite(window);
  await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");
  const button=(id:string)=>window.document.querySelector<HTMLButtonElement>(`#${id}`)!;
  expect(button("remove-site")).not.toBeNull();button("remove-site").click();await waitFor(()=>removed && !window.document.querySelector("#sites")?.textContent?.includes("Site"));
  expect(confirmation).toContain("IPNS");button("show-removed-sites").click();await waitFor(()=>window.document.querySelector("#removed-site-list")?.textContent?.includes("Site")===true);
  window.document.querySelector<HTMLButtonElement>("#removed-site-list button")!.click();await waitFor(()=>!removed && window.document.querySelector("#site-title")?.textContent==="Site");
  expect(window.document.querySelector("#articles")?.textContent).toContain("Saved article");expect(publishes).toBe(0);
});

test("successful site removal remains clear if the list refresh fails", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let calls=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":calls++;if(calls===1)return [site];throw new Error("list unavailable");case "Library.Subscriptions":case "Library.Articles":return [];case "Library.RemoveSite":return;default:throw new Error(method);}}};
  window.confirm=()=>true;await import("../src/main.ts?remove-list-failure"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Site");window.document.querySelector<HTMLButtonElement>("#remove-site")!.click();
  await waitFor(()=>window.document.querySelector("#error")?.textContent?.includes("list unavailable")===true);
  expect(window.document.querySelector("#sites")?.textContent).not.toContain("Site");expect(window.document.querySelector("#error")?.textContent).toContain("已移除");
});

test("site removal disables the editor while the operation is pending", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"},article={id:"p",siteId:"s",title:"Post",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
  let release:(()=>void)|undefined;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [article];case "Library.PreviewArticle":return "";
    case "Library.RemoveSite":await new Promise<void>(r=>{release=r});return;default:throw new Error(method);}}};
  window.confirm=()=>true;await import("../src/main.ts?remove-editor-race"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("Post")===true);
  window.document.querySelector<HTMLButtonElement>("#articles button")!.click();window.document.querySelector<HTMLButtonElement>("#remove-site")!.click();await waitFor(()=>!!release);
  expect(window.document.querySelector<HTMLInputElement>("#title")!.disabled).toBe(true);expect(window.document.querySelector<HTMLTextAreaElement>("#content")!.disabled).toBe(true);
  release!();
});

test("site restoration disables the previous site's editor while pending", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const a={id:"a",name:"A",about:"",created:"2026-01-01T00:00:00Z"},b={id:"b",name:"B",about:"",created:"2026-01-01T00:00:00Z"},article={id:"p",siteId:"a",title:"Post",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
  let release:(()=>void)|undefined;
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [a];case "Library.Subscriptions":return [];case "Library.Articles":return args[0]==="a"?[article]:[];case "Library.PreviewArticle":return "";
    case "Library.RemovedSites":return [b];case "Library.RestoreSite":await new Promise<void>(r=>{release=r});return b;default:throw new Error(method);}}};
  await import("../src/main.ts?restore-editor-race"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("Post")===true);
  window.document.querySelector<HTMLButtonElement>("#articles button")!.click();window.document.querySelector<HTMLButtonElement>("#show-removed-sites")!.click();await waitFor(()=>!!window.document.querySelector("#removed-site-list button"));
  window.document.querySelector<HTMLButtonElement>("#removed-site-list button")!.click();await waitFor(()=>!!release);
  expect(window.document.querySelector<HTMLInputElement>("#title")!.disabled).toBe(true);expect(window.document.querySelector<HTMLTextAreaElement>("#content")!.disabled).toBe(true);release!();
});

test("restored site can reload its articles after a temporary read failure", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const b={id:"b",name:"Restored",about:"",created:"2026-01-01T00:00:00Z"},article={id:"p",siteId:"b",title:"Recovered article",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};let reads=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":case "Library.Subscriptions":return [];case "Library.RemovedSites":return [b];case "Library.RestoreSite":return b;
    case "Library.Articles":reads++;if(reads===1)throw new Error("temporary failure");return [article];default:throw new Error(method);}}};
  await import("../src/main.ts?restore-retry"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#show-removed-sites")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#show-removed-sites")!.click();await waitFor(()=>!!window.document.querySelector("#removed-site-list button"));
  window.document.querySelector<HTMLButtonElement>("#removed-site-list button")!.click();await waitFor(()=>window.document.querySelector("#error")?.textContent?.includes("temporary failure")===true);
  window.document.querySelector<HTMLButtonElement>("#sites button")!.click();await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("Recovered article")===true);
  expect(reads).toBe(2);
});

test("creating a site locks cancellation and destructive navigation until its list refresh completes", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const a={id:"a",name:"Old",about:"",created:"2026-01-01T00:00:00Z"},b={id:"b",name:"New",about:"",created:"2026-01-01T00:00:00Z"};
  let removed=false,calls=0,release:(()=>void)|undefined;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":{calls++;if(calls===2){await new Promise<void>(r=>{release=r});return [a,b];}return removed?[b]:[a];}
    case "Library.Subscriptions":case "Library.Articles":return [];case "Library.CreateSite":return b;case "Library.RemoveSite":removed=true;return;default:throw new Error(method);}}};
  window.confirm=()=>true;await import("../src/main.ts?removed-sites-stale"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Old");
  window.document.querySelector<HTMLButtonElement>("#new-site")!.click();const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('input[name="name"]')!.value="New";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!release);
  const dialog=window.document.querySelector<HTMLDialogElement>("#site-dialog")!;
  expect(window.document.querySelector<HTMLButtonElement>("#cancel-site")!.disabled).toBe(true);
  window.document.querySelector<HTMLButtonElement>("#cancel-site")!.click();window.document.querySelector<HTMLButtonElement>("#remove-site")!.click();
  const escape=new window.Event("cancel",{cancelable:true});dialog.dispatchEvent(escape);
  expect(escape.defaultPrevented).toBe(true);expect(dialog.open).toBe(true);expect(removed).toBe(false);
  release!();await waitFor(()=>!dialog.open);expect(window.document.querySelector("#sites")?.textContent).toContain("New");
});

test("new site remains selectable if another site's publication finishes during its list refresh", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const a={id:"a",name:"Old",about:"",created:"2026-01-01T00:00:00Z"},b={id:"b",name:"New",about:"",created:"2026-01-01T00:00:00Z"};
  let calls=0,releaseSites:(()=>void)|undefined,releasePublish:(()=>void)|undefined;
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
    case "Library.Sites":calls++;if(calls===2){await new Promise<void>(r=>{releaseSites=r});return [a,b];}return [a];
    case "Library.Subscriptions":case "Library.Articles":return [];case "Library.Preview":case "Library.PreviewArticle":return "";
    case "Library.SaveArticle":return {id:"p",siteId:"a",title:args[2],content:args[3],created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
    case "Library.PublishSite":await new Promise<void>(r=>{releasePublish=r});return {ipns:"k51",cid:"bafy",revision:1,pending:false};
    case "Library.CreateSite":return b;default:throw new Error(method);
  }}};
  await import("../src/main.ts?create-during-publish"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Old");
  window.document.querySelector<HTMLButtonElement>("#new-article")!.click();const title=window.document.querySelector<HTMLInputElement>("#title")!;title.value="Post";title.dispatchEvent(new window.Event("input"));window.document.querySelector<HTMLButtonElement>("#save")!.click();await waitFor(()=>!!releasePublish);
  window.document.querySelector<HTMLButtonElement>("#new-site")!.click();const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('input[name="name"]')!.value="New";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));await waitFor(()=>!!releaseSites);
  releasePublish!();await waitFor(()=>window.document.querySelector("#save-state")?.textContent?.includes("正在发布")===false);releaseSites!();await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="New");
  expect(window.document.querySelector("#sites")?.textContent).toContain("New");
});

test("created site stays accessible when refreshing the site list fails", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const created={id:"new",name:"New",about:"",created:"2026-01-01T00:00:00Z"};let calls=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":calls++;if(calls===1)return [];throw new Error("list unavailable");case "Library.Subscriptions":return [];case "Library.CreateSite":return created;default:throw new Error(method);}}};
  await import("../src/main.ts?create-list-failure"); enterFirstSite(window);await new Promise(r=>setTimeout(r,20));window.document.querySelector<HTMLButtonElement>("#new-site")!.click();const form=window.document.querySelector<HTMLFormElement>("#site-form")!;form.querySelector<HTMLInputElement>('input[name="name"]')!.value="New";form.dispatchEvent(new window.Event("submit",{bubbles:true,cancelable:true}));
  await waitFor(()=>window.document.querySelector("#error")?.textContent?.includes("list unavailable")===true);
  expect(window.document.querySelector("#site-title")?.textContent).toBe("New");expect(window.document.querySelector("#sites")?.textContent).toContain("New");expect(window.document.querySelector("#error")?.textContent).toContain("已创建");
});

test("imported site remains selectable if another site's publication finishes during refresh", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const a={id:"a",name:"Old",about:"",created:"2026-01-01T00:00:00Z"},b={id:"b",name:"Imported",about:"",created:"2026-01-01T00:00:00Z",imported:true};
  let calls=0,releaseSites:(()=>void)|undefined,releasePublish:(()=>void)|undefined;
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
    case "Library.Sites":calls++;if(calls===2){await new Promise<void>(r=>{releaseSites=r});return [a,b];}return [a];
    case "Library.Subscriptions":case "Library.Articles":return [];case "Library.Preview":case "Library.PreviewArticle":return "";
    case "Library.SaveArticle":return {id:"p",siteId:"a",title:args[2],content:args[3],created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
    case "Library.PublishSite":await new Promise<void>(r=>{releasePublish=r});return {ipns:"k51",cid:"bafy",revision:1,pending:false};
    case "Library.ImportPlanet":return b;default:throw new Error(method);
  }}};
  await import("../src/main.ts?import-during-publish"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Old");
  window.document.querySelector<HTMLButtonElement>("#new-article")!.click();const title=window.document.querySelector<HTMLInputElement>("#title")!;title.value="Post";title.dispatchEvent(new window.Event("input"));window.document.querySelector<HTMLButtonElement>("#save")!.click();await waitFor(()=>!!releasePublish);
  window.document.querySelector<HTMLButtonElement>("#import-planet")!.click();await waitFor(()=>!!releaseSites);releasePublish!();await waitFor(()=>window.document.querySelector("#save-state")?.textContent?.includes("正在发布")===false);releaseSites!();await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Imported");
  expect(window.document.querySelector("#sites")?.textContent).toContain("Imported");
});

test("selecting a template updates the imported site without a second list request", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const initial={id:"s",name:"Imported",about:"",created:"2026-01-01T00:00:00Z",imported:true};let siteCalls=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":siteCalls++;if(siteCalls>1)throw new Error("list unavailable");return [initial];case "Library.Subscriptions":case "Library.Articles":return [];
    case "Library.SelectTemplate":return {...initial,templateId:"template-1",templateName:"Plain",contentRevision:1};default:throw new Error(method);}}};
  window.alert=()=>{};await import("../src/main.ts?template-update"); enterFirstSite(window);await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Imported");
  window.document.querySelector<HTMLButtonElement>("#new-article")!.click();window.document.querySelector<HTMLButtonElement>("#select-template")!.click();
  window.document.querySelector<HTMLButtonElement>("#template-import")!.click();
  await waitFor(()=>window.document.querySelector("#imported-warning")?.textContent?.includes("通过检查")===true);
  expect(siteCalls).toBe(1);
});

test("private draft is reopened and publishes only when requested", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"site-1",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let drafts:any[]=[],articles:any[]=[],publishes=0;
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{
    switch(method){
      case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return articles;
      case "Library.Preview":case "Library.PreviewArticle":return "";case "Library.Drafts":return drafts;
      case "Library.SaveDraft": {const d={id:"draft-1",siteId:args[0],articleId:args[2] || "",title:args[3],content:args[4],created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};drafts=[d];return d;}
      case "Library.PublishDraft": {const a={id:"post-1",siteId:args[0],title:args[2],content:args[3],created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};drafts=[];articles=[a];return a;}
      case "Library.PublishSite":publishes++;return {ipns:"k51",cid:"bafy",revision:1,pending:false};
      default:throw new Error(`Unexpected call: ${method}`);
    }
  }};
  await import("../src/main.ts?private-drafts"); enterFirstSite(window);
  const button=(id:string)=>window.document.querySelector<HTMLButtonElement>(`#${id}`)!;
  await waitFor(()=>!button("new-article").disabled);button("new-article").click();
  const title=window.document.querySelector<HTMLInputElement>("#title")!,content=window.document.querySelector<HTMLTextAreaElement>("#content")!;
  title.value="Draft";content.value="Private";title.dispatchEvent(new window.Event("input"));
  expect(button("save-draft")).not.toBeNull();button("save-draft").click();await waitFor(()=>drafts.length===1);
  expect(publishes).toBe(0);await waitFor(()=>!button("show-drafts").disabled);button("show-drafts").click();button("show-drafts").click();await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("Draft")===true);
  window.document.querySelector<HTMLButtonElement>("#articles button")!.click();expect(content.value).toBe("Private");
  button("save").click();await waitFor(()=>publishes===1);expect(drafts.length).toBe(0);expect(articles[0].content).toBe("Private");
});

test("leaving an edited draft after discarding clears the unsaved marker", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  const draft={id:"d",siteId:"s",title:"Draft",content:"Saved",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.Drafts":return [draft];case "Library.Preview":return "";default:throw new Error(method);}}};
  window.confirm=()=>true;await import("../src/main.ts?draft-discard"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#show-drafts")!.disabled);
  window.document.querySelector<HTMLButtonElement>("#show-drafts")!.click();await waitFor(()=>window.document.querySelector("#articles")?.textContent?.includes("Draft")===true);
  window.document.querySelector<HTMLButtonElement>("#articles button")!.click();const content=window.document.querySelector<HTMLTextAreaElement>("#content")!;content.value="Unsaved";content.dispatchEvent(new window.Event("input"));
  window.document.querySelector<HTMLButtonElement>("#show-drafts")!.click();expect(window.document.querySelector("#save-state")?.textContent).not.toContain("未保存");
});

test("saving and deleting an article restores site actions", async () => {
  const window = new Window({ url: "http://localhost/" });
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, { window, document: window.document });
  const site = { id: "site-1", name: "Test", about: "", created: "2026-01-01T00:00:00Z", contentRevision:0, publishedRevision:0, publishPending:false };
  const otherSite = { id: "site-2", name: "Other", about: "", created: "2026-01-01T00:00:00Z" };
  let copied = "";
  let confirmation = "";
  let publishedSnapshots: string[] = [];
  let releaseFirst: (() => void) | undefined;
  let failPublish = false;
  let failSave = false;
  let clipboardUnavailable = false;
  Object.defineProperty(window.navigator, "clipboard", { value: { writeText: async (text: string) => { if (clipboardUnavailable) throw new Error("unavailable"); copied = text; } } });
  let articles: Array<{ id: string; siteId: string; title: string; content: string; created: string; modified: string; attachments?: string[] }> = [];
  (globalThis as any).mygo = { call: async (method: string, ...args: string[]) => {
    switch (method) {
      case "Library.Sites": return [site, otherSite];
      case "Library.Subscriptions": return [];
      case "Library.Articles": return args[0] === "site-1" ? articles : [];
      case "Library.Preview": case "Library.PreviewArticle": return "";
      case "Library.SaveArticle": {
        if (failSave) throw new Error("save failed");
        const saved = { ...articles[0], id: "article-1", siteId: args[0]!, title: args[2]!, content: args[3]!, created: "2026-01-01T00:00:00Z", modified: "2026-01-01T00:00:00Z" };
        articles = [saved]; site.contentRevision++; site.publishPending=true; return saved;
      }
      case "Library.AddAttachment": { const updated = { ...articles[0]!, attachments: ["photo.png"] }; articles = [updated]; site.contentRevision++; return updated; }
      case "Library.RemoveAttachment": { const updated = { ...articles[0]!, attachments: [] }; articles = [updated]; site.contentRevision++; return updated; }
      case "Library.DeleteArticle": articles = []; site.contentRevision++; return;
      case "Library.PublishSite": {
        const revision=site.contentRevision;
        publishedSnapshots.push(articles[0]?.content ?? "");
        if (publishedSnapshots.length === 1) await new Promise<void>(resolve => { releaseFirst = resolve; });
        if (failPublish) throw new Error("network unavailable");
        site.publishedRevision=revision; site.publishPending=site.contentRevision!==revision;
        Object.assign(site, { ipns: "k51test", lastPublishedCID: "bafytest" }); return { ipns: "k51test", cid: "bafytest",revision,pending:site.publishPending };
      }
      default: throw new Error(`Unexpected call: ${method}`);
    }
  }};
  window.confirm = (message?: string) => { confirmation = message ?? ""; return true; };
  window.alert = () => {};
  await import("../src/main.ts"); enterFirstSite(window);
  const button = (id: string) => window.document.querySelector<HTMLButtonElement>(`#${id}`)!;
  await waitFor(() => !button("new-article").disabled);
  button("new-article").click();
  const title = window.document.querySelector<HTMLInputElement>("#title")!;
  title.value = "测试文章"; title.dispatchEvent(new window.Event("input"));
  button("save").click();
  await waitFor(() => publishedSnapshots.length === 1 && !!releaseFirst);
  expect(confirmation).toBe("");
  expect(window.document.querySelector("#save-state")?.textContent).toContain("正在发布");
  expect(button("remove-site").disabled).toBe(true);
  const content = window.document.querySelector<HTMLTextAreaElement>("#content")!;
  content.value = "连续保存的内容"; content.dispatchEvent(new window.Event("input"));
  await waitFor(() => !button("save").disabled);
  button("save").click();
  await waitFor(() => articles[0]?.content === "连续保存的内容");
  expect(publishedSnapshots.length).toBe(1);
  Array.from(window.document.querySelectorAll<HTMLButtonElement>(".site-item")).find(item => item.textContent?.includes("Other"))!.click();
  await waitFor(() => window.document.querySelector("#site-title")?.textContent === "Other");
  releaseFirst!();
  await waitFor(() => publishedSnapshots.length === 2);
  expect(window.document.querySelector("#site-title")?.textContent).toBe("Other");
  expect(publishedSnapshots[1]).toBe("连续保存的内容");
  Array.from(window.document.querySelectorAll<HTMLButtonElement>(".site-item")).find(item => item.textContent?.includes("Test"))!.click();
  await waitFor(() => window.document.querySelector("#site-title")?.textContent === "Test" && !button("publish-site").disabled);
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  button("edit-article").click();
  expect(window.document.querySelector("#save-state")?.textContent).toContain("已发布");
  failPublish = true;
  content.value = "断网时保存"; content.dispatchEvent(new window.Event("input"));
  button("save").click();
  await waitFor(() => window.document.querySelector("#error")?.textContent?.includes("network unavailable") === true);
  expect(articles[0]?.content).toBe("断网时保存");
  expect(window.document.querySelector("#save-state")?.textContent).toContain("发布未确认");
  failPublish = false;
  expect(button("publish-site").disabled).toBe(false);
  expect(button("export-site").disabled).toBe(false);
  expect(button("new-article").disabled).toBe(false);
  button("add-attachment").click();
  await waitFor(() => window.document.querySelector("#attachment-list")?.textContent?.includes("photo.png") === true);
  expect(window.document.querySelector("#attachment-help")!.textContent).toContain("立即保存");
  expect(window.document.querySelector<HTMLTextAreaElement>("#content")!.value).toContain("![photo.png](photo.png)");
  button("save").click();
  await waitFor(() => !button("publish-site").disabled && window.document.querySelector("#save-state")?.textContent?.includes("已发布") === true);
  button("publish-site").click();
  expect(confirmation).toContain("photo.png");
  await waitFor(() => (window.document.querySelector<HTMLInputElement>("#published-url")?.value ?? "") !== "");
  expect(window.document.querySelector<HTMLInputElement>("#published-url")!.value).toBe("https://ipfs.io/ipns/k51test/");
  button("copy-published-url").click();
  await waitFor(() => copied !== "");
  expect(copied).toBe("https://ipfs.io/ipns/k51test/");
  clipboardUnavailable = true;
  button("copy-published-url").click();
  await waitFor(() => window.document.querySelector("#copy-status")?.textContent === "请按 Ctrl+C 复制");
  const address = window.document.querySelector<HTMLInputElement>("#published-url")!;
  expect(address.selectionStart).toBe(0);
  expect(address.selectionEnd).toBe(address.value.length);
  window.document.querySelector<HTMLButtonElement>(".remove-attachment")!.click();
  await waitFor(() => !window.document.querySelector("#attachment-list")?.textContent?.includes("photo.png"));
  expect(confirmation).toContain("photo.png");
  expect(window.document.querySelector("#save-state")?.textContent).toContain("尚未发布");
  button("delete-article").click();
  await waitFor(() => window.document.querySelector("#post-count")?.textContent === "0");
  expect(window.document.querySelector("#save-state")?.textContent).toContain("尚未发布");
  expect(button("publish-site").disabled).toBe(false);
  expect(button("new-article").disabled).toBe(false);
  const countBeforeFailure = publishedSnapshots.length;
  failSave = true;
  button("new-article").click();
  title.value = "无法保存"; title.dispatchEvent(new window.Event("input"));
  button("save").click();
  await waitFor(() => window.document.querySelector("#error")?.textContent?.includes("save failed") === true);
  expect(publishedSnapshots.length).toBe(countBeforeFailure);
});

test("restart retries saved pending publications", async () => {
  const window = new Window({url: "http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document: window.document});
  const site = {id:"pending-site", name:"Pending", about:"", created:"2026-01-01T00:00:00Z", contentRevision:2, publishedRevision:1, publishPending:true};
  let calls=0;
  (globalThis as any).mygo = {call: async (method: string) => {
    switch(method) {
      case "Library.Sites": return [site];
      case "Library.Subscriptions": case "Library.Articles": return [];
      case "Library.PublishSite": calls++; site.publishPending=false; site.publishedRevision=2; return {ipns:"k51retry",cid:"bafyrecovered",revision:2,pending:false};
      default: throw new Error(`Unexpected call: ${method}`);
    }
  }};
  await import("../src/main.ts?restart"); enterFirstSite(window);
  await waitFor(() => calls === 1);
  expect(calls).toBe(1);
  await waitFor(() => window.document.querySelector<HTMLInputElement>("#published-url")?.value === "https://ipfs.io/ipns/k51retry/");
});

test("saving cancels an older pending site switch and still publishes", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const first={id:"first",name:"First",about:"",created:"2026-01-01T00:00:00Z"}, second={id:"second",name:"Second",about:"",created:"2026-01-01T00:00:00Z"};
  const post={id:"post",siteId:"first",title:"Initial",content:"initial",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};
  let releaseSwitch:(() => void)|undefined, releaseAttachment:(()=>void)|undefined, published="";
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{
    switch(method){
      case "Library.Sites":return [first,second];
      case "Library.Subscriptions":return [];
      case "Library.Articles":if(args[0]==="second"){await new Promise<void>(resolve=>{releaseSwitch=resolve});return []};return [post];
      case "Library.Preview":case "Library.PreviewArticle":return "";
      case "Library.SaveArticle":post.title=args[2]!;post.content=args[3]!;return {...post};
      case "Library.AddAttachment":await new Promise<void>(resolve=>{releaseAttachment=resolve});return {...post,attachments:["file.txt"]};
      case "Library.PublishSite":published=args[0]!;return {ipns:"k51first",cid:"bafyfirst",revision:1,pending:false};
      default:throw new Error(`Unexpected call: ${method}`);
    }
  }};
  await import("../src/main.ts?navigation-race"); enterFirstSite(window);
  await waitFor(()=>!!window.document.querySelector(".article-item"));
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  window.document.querySelector<HTMLButtonElement>("#edit-article")!.click();
  const content=window.document.querySelector<HTMLTextAreaElement>("#content")!;content.value="changed";content.dispatchEvent(new window.Event("input"));
  window.confirm=()=>true;
  Array.from(window.document.querySelectorAll<HTMLButtonElement>(".site-item")).find(item=>item.textContent?.includes("Second"))!.click();
  await waitFor(()=>!!releaseSwitch);
  window.document.querySelector<HTMLButtonElement>("#save")!.click();
  await waitFor(()=>published==="first");
  releaseSwitch!();await new Promise(resolve=>setTimeout(resolve,10));
  expect(window.document.querySelector("#site-title")?.textContent).toBe("First");
  expect(post.content).toBe("changed");
  releaseSwitch=undefined;
  Array.from(window.document.querySelectorAll<HTMLButtonElement>(".site-item")).find(item=>item.textContent?.includes("Second"))!.click();
  await waitFor(()=>!!releaseSwitch);
  window.document.querySelector<HTMLButtonElement>("#add-attachment")!.click();await waitFor(()=>!!releaseAttachment);
  releaseSwitch!();await new Promise(resolve=>setTimeout(resolve,10));
  releaseAttachment!();await waitFor(()=>content.value.includes("[file.txt](file.txt)"));
  expect(window.document.querySelector("#site-title")?.textContent).toBe("First");
});

test("pending publication is recovered even when subscriptions cannot load", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"pending",name:"Pending",about:"",created:"2026-01-01T00:00:00Z",contentRevision:1,publishedRevision:0,publishPending:true};let calls=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":throw new Error("subscriptions failed");case "Library.Articles":return [];case "Library.PublishSite":calls++;return {ipns:"k51",cid:"bafy",revision:1,pending:false};default:throw new Error(`Unexpected ${method}`)}}};
  await import("../src/main.ts?subscription-error"); enterFirstSite(window);await waitFor(()=>calls===1);expect(calls).toBe(1);
});

test("changes during publication trigger a follow-up snapshot", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"site",name:"Site",about:"",created:"2026-01-01T00:00:00Z",contentRevision:1,publishedRevision:1,publishPending:false};
  const post={id:"post",siteId:"site",title:"Post",content:"text",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",attachments:["photo.png"]};
  let release:(()=>void)|undefined, releaseAttachment:(()=>void)|undefined, count=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){
    case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [{...post}];case "Library.Preview":case "Library.PreviewArticle":return "";
    case "Library.RemoveAttachment":await new Promise<void>(resolve=>{releaseAttachment=resolve});post.attachments=[];site.contentRevision++;return {...post};
    case "Library.PublishSite":{count++;const revision=site.contentRevision;if(count===1)await new Promise<void>(resolve=>{release=resolve});const pending=revision!==site.contentRevision;site.publishedRevision=revision;site.publishPending=pending;return {ipns:"k51",cid:"bafy",revision,pending};}
    default:throw new Error(`Unexpected ${method}`);
  }}};
  window.confirm=()=>true;window.alert=()=>{};await import("../src/main.ts?mutation-race"); enterFirstSite(window);
  await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  window.document.querySelector<HTMLButtonElement>("#publish-site")!.click();await waitFor(()=>!!release);
  window.document.querySelector<HTMLButtonElement>(".remove-attachment")!.click();await waitFor(()=>!!releaseAttachment);
  expect(window.document.querySelector("#save-state")?.textContent).toContain("等待");
  releaseAttachment!();await waitFor(()=>site.contentRevision===2);
  release!();await waitFor(()=>count===2 && !window.document.querySelector<HTMLButtonElement>("#publish-site")!.disabled);
  expect(site.publishedRevision).toBe(2);
});

test("imported articles expose attachment editing", async () => {
  const window = new Window({url:"http://localhost/"}); window.document.write(await Bun.file("index.html").text()); Object.assign(globalThis,{window,document:window.document});
  const site={id:"imported",name:"Imported",about:"",created:"2026-01-01T00:00:00Z",imported:true};
  let post={id:"post",siteId:site.id,title:"Post",content:"Text",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",attachments:["old.txt"]};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){
    case "Library.Sites":return [site]; case "Library.Subscriptions":return []; case "Library.Articles":return [post]; case "Library.Preview":case "Library.PreviewArticle":return "";
    case "Library.AddAttachment":post={...post,attachments:[...post.attachments,"new.txt"]};return post;
    case "Library.RemoveAttachment":post={...post,attachments:post.attachments.filter(name=>name!=="old.txt")};return post;
    default:throw new Error(`Unexpected ${method}`);
  }}};
  window.confirm=()=>true; await import("../src/main.ts?imported-attachments"); enterFirstSite(window); await waitFor(()=>!!window.document.querySelector(".article-item")); window.document.querySelector<HTMLButtonElement>(".article-item")!.click();window.document.querySelector<HTMLButtonElement>("#edit-article")!.click();
  expect(window.document.querySelector<HTMLButtonElement>("#add-attachment")!.disabled).toBe(false);
  window.document.querySelector<HTMLButtonElement>(".remove-attachment")!.click(); await waitFor(()=>!window.document.querySelector("#attachment-list")?.textContent?.includes("old.txt"));
  window.document.querySelector<HTMLButtonElement>("#add-attachment")!.click(); await waitFor(()=>window.document.querySelector("#attachment-list")?.textContent?.includes("new.txt")===true);
  expect(window.document.querySelector<HTMLTextAreaElement>("#content")!.value).toContain("[new.txt](new.txt)");
});

test("saved article preview shows and removes a local image", async () => {
 const window=new Window({url:"http://localhost/"}); window.document.write(await Bun.file("index.html").text()); Object.assign(globalThis,{window,document:window.document});
 const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"}; let post={id:"p",siteId:"s",title:"Post",content:"![image](photo.png)",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",attachments:["photo.png"]};
 (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
 case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [post];case "Library.Preview":return '<img src="photo.png">';
 case "Library.PreviewArticle":return post.attachments.length ? '<img src="data:image/png;base64,aW1hZ2U=">' : '<img src="">';
 case "Library.RemoveAttachment":post={...post,attachments:[]};return post;
 default:throw new Error(`Unexpected ${method} ${args}`);
 }}};
 window.confirm=()=>true;await import("../src/main.ts?local-image-preview"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();window.document.querySelector<HTMLButtonElement>("#edit-article")!.click();
 const frame=window.document.querySelector<HTMLIFrameElement>("#preview")!;await waitFor(()=>frame.srcdoc.includes("data:image/png;base64,aW1hZ2U="));expect(frame.srcdoc).toContain("img-src data:");
 window.document.querySelector<HTMLButtonElement>(".remove-attachment")!.click();await waitFor(()=>!frame.srcdoc.includes("base64,aW1hZ2U="));
});

test("global audio player remains visible after switching sites", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const first={id:"first",name:"First",about:"",created:"2026-01-01T00:00:00Z"},second={id:"second",name:"Second",about:"",created:"2026-01-01T00:00:00Z"};
 const post={id:"post",siteId:"first",title:"Song",content:"",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",attachments:["sound.mp3"],audioFilename:"sound.mp3"};
 (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [first,second];case "Library.Subscriptions":return [];case "Library.Articles":return args[0]==="first"?[post]:[];case "Library.PreviewArticle":return "";case "Library.AudioURL":return "http://127.0.0.1:1234/token";case "Library.StopAudio":return;default:throw new Error(`Unexpected ${method}`)}}};
 await import("../src/main.ts?global-player"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
 window.document.querySelector<HTMLButtonElement>("#play-own-audio")!.click();await waitFor(()=>!window.document.querySelector<HTMLElement>("#audio-player")!.hidden);
 expect(window.document.querySelector<HTMLAudioElement>("#global-audio")!.src).toBe("http://127.0.0.1:1234/token");
 Array.from(window.document.querySelectorAll<HTMLButtonElement>(".site-item")).find(x=>x.textContent?.includes("Second"))!.click();await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="Second");expect(window.document.querySelector<HTMLElement>("#audio-player")!.hidden).toBe(false);
 window.document.querySelector<HTMLButtonElement>("#close-audio")!.click();expect(window.document.querySelector<HTMLElement>("#audio-player")!.hidden).toBe(true);
});

test("subscription audio opens in the global player", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const feed={url:"https://example.com/feed",feed:{title:"Show",description:"",link:"",items:[{id:"1",title:"Episode",link:"",description:"",content:"",published:"",audioUrl:"https://example.com/episode.mp3"}]}};
 (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return [feed];case "Library.StopAudio":return;default:throw new Error(`Unexpected ${method}`)}}};
 await import("../src/main.ts?feed-audio"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector("#subscriptions .site-item"));window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
 const button=window.document.querySelector<HTMLButtonElement>("#play-feed-audio")!;expect(button.hidden).toBe(false);button.click();await waitFor(()=>!window.document.querySelector<HTMLElement>("#audio-player")!.hidden);
 expect(window.document.querySelector<HTMLAudioElement>("#global-audio")!.src).toBe("https://example.com/episode.mp3");
});

test("subscription articles can be starred, found and shared, and Planet content can be pinned", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const post={id:"post",key:"post",title:"Readable",link:"https://example.test/post",description:"",content:"text",published:"",starred:false};
 const sub={url:"k51abcdefghijk",cid:"bafytest",pinnedCid:"",lastUpdated:new Date().toISOString(),feed:{title:"Planet",description:"",link:"",items:[post]}};let copied="";
 Object.defineProperty(window.navigator,"clipboard",{value:{writeText:async(value:string)=>{copied=value}}});
 (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return [sub];case "Library.PinSubscription":sub.pinnedCid=sub.cid;return;case "Library.StarSubscription":post.starred=args[2];return;default:throw new Error(`Unexpected ${method}`)}}};
 await import("../src/main.ts?favorites-pin"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector("#subscriptions .site-item"));window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();
 const pin=window.document.querySelector<HTMLButtonElement>("#pin-subscription")!;expect(pin.hidden).toBe(false);pin.click();await waitFor(()=>sub.pinnedCid==="bafytest");
 window.document.querySelector<HTMLButtonElement>(".article-item")!.click();window.document.querySelector<HTMLButtonElement>("#star-item")!.click();await waitFor(()=>post.starred);
 window.document.querySelector<HTMLButtonElement>("#share-item")!.click();await waitFor(()=>copied!=="");expect(copied).toBe("https://example.test/post");
 window.document.querySelector<HTMLButtonElement>("#show-favorites")!.click();expect(window.document.querySelector("#articles")?.textContent).toContain("Readable");
});

test("pinned Planet reader loads local images and ignores a stale preview", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const one={id:"one",key:"one",articleId:"one",contentCid:"bafy",title:"One",link:"https://example.test/one",description:"",content:"<img src=photo.png>",published:"",attachments:["photo.png"]};
 const two={...one,id:"two",key:"two",articleId:"two",title:"Two",content:"second"};const sub={url:"k51abcdefghijk",cid:"bafy",pinnedCid:"bafy",lastUpdated:new Date().toISOString(),feed:{title:"Planet",description:"",link:"",items:[one,two]}};
 let release:((value:string)=>void)|undefined;
 (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return [sub];case "Library.SubscriptionPreview":if(args[1]==="one")return await new Promise<string>(resolve=>{release=resolve});return "second";default:throw new Error(`Unexpected ${method}`)}}};
 await import("../src/main.ts?offline-preview"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector("#subscriptions .site-item"));window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();
 const posts=window.document.querySelectorAll<HTMLButtonElement>(".article-item");posts[0]!.click();await waitFor(()=>!!release);window.document.querySelectorAll<HTMLButtonElement>(".article-item")[1]!.click();await waitFor(()=>window.document.querySelector<HTMLIFrameElement>("#reader-preview")!.srcdoc.includes("second"));release!("data:image/png;base64,STALE");await new Promise(resolve=>setTimeout(resolve,10));
 expect(window.document.querySelector<HTMLIFrameElement>("#reader-preview")!.srcdoc).not.toContain("STALE");
});

test("Planet audio is opened from the local content store", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const item={id:"post",key:"post",articleId:"post",contentCid:"bafy",audioFilename:"sound.mp3",attachments:["sound.mp3"],title:"Episode",link:"https://example.test/post",description:"",content:"text",published:""};const sub={url:"k51abcdefghijk",cid:"bafy",pinnedCid:"bafy",lastUpdated:new Date().toISOString(),feed:{title:"Planet",description:"",link:"",items:[item]}};
 (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return [sub];case "Library.SubscriptionPreview":return "text";case "Library.SubscriptionAudioURL":return "http://127.0.0.1:1234/offline";case "Library.StopAudio":return;default:throw new Error(`Unexpected ${method} ${args}`)}}};
 await import("../src/main.ts?offline-audio"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector("#subscriptions .site-item"));window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
 const play=window.document.querySelector<HTMLButtonElement>("#play-feed-audio")!;expect(play.hidden).toBe(false);play.click();await waitFor(()=>!window.document.querySelector<HTMLElement>("#audio-player")!.hidden);expect(window.document.querySelector<HTMLAudioElement>("#global-audio")!.src).toBe("http://127.0.0.1:1234/offline");
});

test("feed media pin enables offline image and video playback", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
 const mediaURL="https://example.test/video.mp4";const item={id:"one",key:"one",title:"Episode",link:"https://example.test/post",description:"",content:"<img src='x'>",published:"",offlineBase:"https://example.test/",videoUrl:mediaURL,offlineMedia:{[mediaURL]:{snapshot:"snapshot-1",file:"x",contentType:"video/mp4",size:5}}};const sub={url:"https://example.test/feed",offlinePinned:false,lastUpdated:new Date().toISOString(),feed:{title:"Feed",description:"",link:"",items:[item]}};
 (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return [sub];case "Library.PinSubscription":sub.offlinePinned=true;return;case "Library.SubscriptionPreview":return '<img src="data:image/png;base64,aW1hZ2U=">';case "Library.SubscriptionMediaURL":return "http://127.0.0.1:1234/video";default:throw new Error(`Unexpected ${method}`)}}};
 await import("../src/main.ts?feed-video-offline"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector("#subscriptions .site-item"));window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();const pin=window.document.querySelector<HTMLButtonElement>("#pin-subscription")!;expect(pin.hidden).toBe(false);pin.click();await waitFor(()=>sub.offlinePinned);
 window.document.querySelector<HTMLButtonElement>(".article-item")!.click();await waitFor(()=>window.document.querySelector<HTMLIFrameElement>("#reader-preview")!.srcdoc.includes("data:image/png;base64"));window.document.querySelector<HTMLButtonElement>("#play-feed-video")!.click();await waitFor(()=>!window.document.querySelector<HTMLElement>("#audio-player")!.hidden);expect(window.document.querySelector<HTMLVideoElement>("#global-video")!.src).toBe("http://127.0.0.1:1234/video");
});

test("offline storage shows usage and refreshes after cleanup", async () => {
 const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});let cleaned=false;
 (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":case "Library.Subscriptions":return [];case "Library.OfflineMediaUsage":return {totalBytes:cleaned?4:12,reclaimableBytes:cleaned?0:8,managedFiles:cleaned?1:2,reclaimableFiles:cleaned?0:1};case "Library.CleanupOfflineMedia":cleaned=true;return {reclaimedBytes:8,reclaimedFiles:1};default:throw new Error(`Unexpected ${method}`)}}};
 await import("../src/main.ts?offline-cleanup"); enterFirstSite(window); window.document.querySelector<HTMLButtonElement>("#offline-storage")!.click();await waitFor(()=>window.document.querySelector("#offline-storage-usage")!.textContent!.includes("8 B"));window.document.querySelector<HTMLButtonElement>("#cleanup-offline")!.click();await waitFor(()=>cleaned && window.document.querySelector("#offline-storage-result")!.textContent!.includes("8 B"));expect(window.document.querySelector("#offline-storage-usage")!.textContent).toContain("0 B");
});
test("article tags stay local until save and trigger the normal publish", async () => {
  const window = new Window({url:"http://localhost/"}); window.document.write(await Bun.file("index.html").text()); Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  let post={id:"p",siteId:"s",title:"Post",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{go:"Go"} as Record<string,string>};
  let publishes=0, savedTags:Record<string,string>|undefined;
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
    case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [post];case "Library.PreviewArticle":case "Library.Preview":return "";
    case "Library.SaveArticleWithTags":savedTags=JSON.parse(args[4]!);post={...post,tags:savedTags!};return post;
    case "Library.PublishSite":publishes++;return {ipns:"k51",cid:"bafy",revision:1,pending:false};
    default:throw new Error(method);
  }}};
  await import("../src/main.ts?edit-tags"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();window.document.querySelector<HTMLButtonElement>("#edit-article")!.click();
  expect(window.document.querySelector("#tag-list")?.textContent).toContain("Go");
  const input=window.document.querySelector<HTMLInputElement>("#tag-input")!;input.value="New tag";window.document.querySelector<HTMLButtonElement>("#add-tag")!.click();
  expect(window.document.querySelector("#tag-list")?.textContent).toContain("New tag");expect(publishes).toBe(0);
  window.document.querySelector<HTMLButtonElement>('[aria-label="删除标签 Go"]')!.click();
  window.document.querySelector<HTMLButtonElement>("#save")!.click();await waitFor(()=>publishes===1);
  expect(savedTags).toEqual({"new-tag":"New tag"});
});

test("unadded tag input blocks saving and prompts before navigation", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};const post={id:"p",siteId:"s",title:"Post",content:"Body",created:site.created,modified:site.created};let saves=0, confirms=0;
  window.confirm=()=>{confirms++;return false;};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Articles":return [post];case "Library.Subscriptions":return [];case "Library.PreviewArticle":return "";case "Library.SaveArticle":case "Library.SaveDraft":saves++;return {};default:throw new Error(method);}}};
  await import("../src/main.ts?pending-tag-input"); enterFirstSite(window);await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();window.document.querySelector<HTMLButtonElement>("#edit-article")!.click();
  const input=window.document.querySelector<HTMLInputElement>("#tag-input")!;input.value="Pending";input.dispatchEvent(new window.Event("input"));
  window.document.querySelector<HTMLButtonElement>("#save")!.click();window.document.querySelector<HTMLButtonElement>("#save-draft")!.click();await new Promise(resolve=>setTimeout(resolve,20));
  expect(saves).toBe(0);expect(window.document.querySelector("#error")!.textContent).toContain("请先添加标签");expect(input.value).toBe("Pending");
  window.document.querySelector<HTMLButtonElement>("#show-favorites")!.click();expect(confirms).toBe(1);expect(input.value).toBe("Pending");
});

test("draft tags remain private until the draft is published", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};let stored:any, published:any, count=0;
  (globalThis as any).mygo={call:async(method:string,...args:string[])=>{switch(method){
    case "Library.Sites":return [site];case "Library.Subscriptions":case "Library.Articles":return [];case "Library.Drafts":return stored?[stored]:[];case "Library.Preview":case "Library.PreviewDraft":return "";
    case "Library.SaveDraftWithTags":stored={id:"d",siteId:"s",title:args[3],content:args[4],tags:JSON.parse(args[5]!),created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z"};return stored;
    case "Library.PublishDraftWithTags":published=JSON.parse(args[4]!);return {id:"p",siteId:"s",title:args[2],content:args[3],tags:published,created:stored.created,modified:stored.modified};
    case "Library.PublishSite":count++;return {ipns:"k51",cid:"bafy",revision:1,pending:false};default:throw new Error(method);
  }}};
  await import("../src/main.ts?draft-tags"); enterFirstSite(window);await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#new-article")!.disabled);window.document.querySelector<HTMLButtonElement>("#new-article")!.click();
  const title=window.document.querySelector<HTMLInputElement>("#title")!,input=window.document.querySelector<HTMLInputElement>("#tag-input")!;title.value="Post";title.dispatchEvent(new window.Event("input"));
  input.value="Private";window.document.querySelector<HTMLButtonElement>("#add-tag")!.click();window.document.querySelector<HTMLButtonElement>("#save-draft")!.click();await waitFor(()=>!!stored);
  expect(stored.tags).toEqual({private:"Private"});expect(count).toBe(0);
  await waitFor(()=>!window.document.querySelector<HTMLButtonElement>("#add-tag")!.disabled);input.value="Public";window.document.querySelector<HTMLButtonElement>("#add-tag")!.click();window.document.querySelector<HTMLButtonElement>('[aria-label="删除标签 Private"]')!.click();
  window.document.querySelector<HTMLButtonElement>("#save")!.click();await waitFor(()=>count===1);expect(published).toEqual({public:"Public"});
});
