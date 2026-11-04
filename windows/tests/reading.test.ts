import {expect, test} from "bun:test";
import {Window} from "happy-dom";

async function waitFor(predicate:()=>boolean) {
  for (let i=0;i<150;i++) { if(predicate()) return; await new Promise(resolve=>setTimeout(resolve,10)); }
  throw new Error("Timed out waiting for reading interface");
}

test("unread feed marks an opened item read and can restore unread status",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const post={id:"1",key:"1",title:"今日新闻",link:"https://example.test/a",description:"正文",content:"正文",published:new Date().toISOString(),read:false};
  const sub={url:"https://example.test/feed",feed:{title:"示例订阅",description:"",link:"",items:[post]}};
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return [sub];case "Library.SetSubscriptionRead":post.read=args[2];return;default:throw new Error(method);}}};
  await import("../src/main.ts?smart-read");await waitFor(()=>!!window.document.querySelector("#subscriptions .site-item"));
  window.document.querySelector<HTMLButtonElement>("#show-unread")!.click();
  expect(window.document.querySelector("#articles")!.textContent).toContain("今日新闻");
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  await waitFor(()=>post.read===true);
  expect(window.document.querySelector("#articles")!.textContent).not.toContain("今日新闻");
  const toggle=window.document.querySelector<HTMLButtonElement>("#toggle-read")!;
  await waitFor(()=>!toggle.disabled);toggle.click();await waitFor(()=>post.read===false);
  expect(window.document.querySelector("#articles")!.textContent).toContain("今日新闻");
});

test("mark all read applies only to the selected subscription",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const subscriptions=["a","b"].map(name=>({url:`https://${name}.test/feed`,feed:{title:name,description:"",link:"",items:[{id:name,key:name,title:name,published:new Date().toISOString(),read:false}]}}));
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [];case "Library.Subscriptions":return subscriptions;case "Library.MarkSubscriptionRead":subscriptions.find(s=>s.url===args[0])!.feed.items.forEach(item=>item.read=true);return;default:throw new Error(method);}}};
  await import("../src/main.ts?mark-all-read");await waitFor(()=>window.document.querySelectorAll("#subscriptions .site-item").length===2);
  window.document.querySelectorAll<HTMLButtonElement>("#subscriptions .site-item")[0]!.click();
  window.document.querySelector<HTMLButtonElement>("#mark-feed-read")!.click();
  await waitFor(()=>subscriptions[0]!.feed.items[0]!.read);
  expect(subscriptions[1]!.feed.items[0]!.read).toBe(false);
});

test("global search selects a saved local article",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"My site",about:"",created:"2026-01-01T00:00:00Z"};
  const post={id:"a",siteId:"s",title:"搜索结果",content:"hello",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{}};
  (globalThis as any).mygo={call:async(method:string,...args:any[])=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [post];case "Library.SearchArticles":return args[0]==="hello"?[{title:post.title,sourceName:site.name,snippet:post.content,siteId:"s",articleId:"a"}]:[];case "Library.PreviewArticle":return "hello";default:throw new Error(method);}}};
  await import("../src/main.ts?global-search");await waitFor(()=>window.document.querySelector("#site-title")?.textContent==="My site");
  window.document.querySelector<HTMLButtonElement>("#show-search")!.click();
  const input=window.document.querySelector<HTMLInputElement>("#search-input")!;input.value="hello";input.dispatchEvent(new window.Event("input"));
  await waitFor(()=>!!window.document.querySelector("#search-results button"));
  window.document.querySelector<HTMLButtonElement>("#search-results button")!.click();
  await waitFor(()=>window.document.querySelector("#own-reader-title")!.textContent==="搜索结果");
  expect(window.document.querySelector<HTMLElement>("#own-reader")!.hidden).toBe(false);
  expect(window.document.querySelector<HTMLDialogElement>("#search-dialog")!.open).toBe(false);
});

test("IPFS storage usage is displayed and garbage collection requires confirmation",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  let confirmed=false,collections=0;window.confirm=()=>confirmed;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":case "Library.Subscriptions":return [];case "Library.IPFSStatus":return {disabled:false,running:true,online:true,version:"0.38",peers:0,rateIn:0,rateOut:0,totalIn:0,totalOut:0,gateway:"http://127.0.0.1:1234"};case "Library.IPFSRepoSize":return 4096;case "Library.IPFSGarbageCollect":collections++;return;default:throw new Error(method);}}};
  await import("../src/main.ts?ipfs-storage");window.document.querySelector<HTMLButtonElement>("#show-ipfs-status")!.click();
  await waitFor(()=>window.document.querySelector("#ipfs-repo-size")?.textContent?.includes("4.0 KiB")===true);
  const gc=window.document.querySelector<HTMLButtonElement>("#ipfs-gc")!;gc.click();expect(collections).toBe(0);
  confirmed=true;gc.click();await waitFor(()=>collections===1);window.document.querySelector<HTMLButtonElement>("#close-ipfs-status")!.click();
});

test("a saved local article opens for reading before entering the editor",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",created:"2026-01-01T00:00:00Z"};
  const post={id:"a",siteId:"s",title:"本地文章",content:"正文内容",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{}};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [post];case "Library.PreviewArticle":return "<p>正文内容</p>";default:throw new Error(method);}}};
  await import("../src/main.ts?own-article-reading");await waitFor(()=>!!window.document.querySelector(".article-item"));
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  await waitFor(()=>window.document.querySelector<HTMLIFrameElement>("#own-reader-preview")?.srcdoc.includes("正文内容")===true);
  expect(window.document.querySelector<HTMLElement>("#own-reader")!.hidden).toBe(false);
  expect(window.document.querySelector<HTMLElement>("#editor")!.hidden).toBe(true);
  window.document.querySelector<HTMLButtonElement>("#edit-article")!.click();
  expect(window.document.querySelector<HTMLInputElement>("#title")!.value).toBe("本地文章");
  expect(window.document.querySelector<HTMLElement>("#editor")!.hidden).toBe(false);
});

test("copying an owned article uses the selected gateway and offers a fallback",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",ipns:"k51example",lastPublishedCID:"bafyexample",created:"2026-01-01T00:00:00Z"};
  const post={id:"abc-123",siteId:"s",title:"本地文章",content:"正文",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{}};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [post];case "Library.PreviewArticle":return "正文";default:throw new Error(method);}}};
  await import("../src/main.ts?own-article-share");await waitFor(()=>!!window.document.querySelector(".article-item"));
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  const gateway=window.document.querySelector<HTMLSelectElement>("#public-gateway")!;gateway.value="eth.sucks";gateway.dispatchEvent(new window.Event("change"));
  window.document.querySelector<HTMLButtonElement>("#share-own-article")!.click();
  await waitFor(()=>window.document.querySelector<HTMLInputElement>("#own-share-url")!.value!=="");
  expect(window.document.querySelector<HTMLInputElement>("#own-share-url")!.value).toBe("https://k51example.eth.sucks/abc-123/");
});

test("copying a custom article path warns when the site has unpublished changes",async()=>{
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"s",name:"Site",about:"",ipns:"k51example",lastPublishedCID:"bafyexample",contentRevision:4,publishedRevision:3,created:"2026-01-01T00:00:00Z"};
  const post={id:"abc",slug:"my-article",siteId:"s",title:"Post",content:"Body",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",tags:{}};
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [post];case "Library.PreviewArticle":return "Body";default:throw new Error(method);}}};
  await import("../src/main.ts?own-custom-article-share");await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();window.document.querySelector<HTMLButtonElement>("#share-own-article")!.click();
  await waitFor(()=>!!window.document.querySelector("#own-share-status")?.textContent);expect(window.document.querySelector<HTMLInputElement>("#own-share-url")!.value).toContain("/my-article/");expect(window.document.querySelector("#own-share-status")!.textContent).toContain("未发布");
});
