import { expect, test } from "bun:test";
import { Window } from "happy-dom";

const waitFor = async (condition: () => boolean) => {
  for (let i = 0; i < 100; i++) {
    if (condition()) return;
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  throw new Error("Timed out waiting for the interface");
};

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
      case "Library.Preview": return "";
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
  await import("../src/main.ts");
  const button = (id: string) => window.document.querySelector<HTMLButtonElement>(`#${id}`)!;
  await waitFor(() => !button("new-article").disabled);
  button("new-article").click();
  const title = window.document.querySelector<HTMLInputElement>("#title")!;
  title.value = "测试文章"; title.dispatchEvent(new window.Event("input"));
  button("save").click();
  await waitFor(() => publishedSnapshots.length === 1 && !!releaseFirst);
  expect(confirmation).toBe("");
  expect(window.document.querySelector("#save-state")?.textContent).toContain("正在发布");
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
  await import("../src/main.ts?restart");
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
      case "Library.Preview":return "";
      case "Library.SaveArticle":post.title=args[2]!;post.content=args[3]!;return {...post};
      case "Library.AddAttachment":await new Promise<void>(resolve=>{releaseAttachment=resolve});return {...post,attachments:["file.txt"]};
      case "Library.PublishSite":published=args[0]!;return {ipns:"k51first",cid:"bafyfirst",revision:1,pending:false};
      default:throw new Error(`Unexpected call: ${method}`);
    }
  }};
  await import("../src/main.ts?navigation-race");
  await waitFor(()=>!!window.document.querySelector(".article-item"));
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
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
  await import("../src/main.ts?subscription-error");await waitFor(()=>calls===1);expect(calls).toBe(1);
});

test("changes during publication trigger a follow-up snapshot", async () => {
  const window=new Window({url:"http://localhost/"});window.document.write(await Bun.file("index.html").text());Object.assign(globalThis,{window,document:window.document});
  const site={id:"site",name:"Site",about:"",created:"2026-01-01T00:00:00Z",contentRevision:1,publishedRevision:1,publishPending:false};
  const post={id:"post",siteId:"site",title:"Post",content:"text",created:"2026-01-01T00:00:00Z",modified:"2026-01-01T00:00:00Z",attachments:["photo.png"]};
  let release:(()=>void)|undefined, releaseAttachment:(()=>void)|undefined, count=0;
  (globalThis as any).mygo={call:async(method:string)=>{switch(method){
    case "Library.Sites":return [site];case "Library.Subscriptions":return [];case "Library.Articles":return [{...post}];case "Library.Preview":return "";
    case "Library.RemoveAttachment":await new Promise<void>(resolve=>{releaseAttachment=resolve});post.attachments=[];site.contentRevision++;return {...post};
    case "Library.PublishSite":{count++;const revision=site.contentRevision;if(count===1)await new Promise<void>(resolve=>{release=resolve});const pending=revision!==site.contentRevision;site.publishedRevision=revision;site.publishPending=pending;return {ipns:"k51",cid:"bafy",revision,pending};}
    default:throw new Error(`Unexpected ${method}`);
  }}};
  window.confirm=()=>true;window.alert=()=>{};await import("../src/main.ts?mutation-race");
  await waitFor(()=>!!window.document.querySelector(".article-item"));window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  window.document.querySelector<HTMLButtonElement>("#publish-site")!.click();await waitFor(()=>!!release);
  window.document.querySelector<HTMLButtonElement>(".remove-attachment")!.click();await waitFor(()=>!!releaseAttachment);
  expect(window.document.querySelector("#save-state")?.textContent).toContain("等待");
  releaseAttachment!();await waitFor(()=>site.contentRevision===2);
  release!();await waitFor(()=>count===2 && !window.document.querySelector<HTMLButtonElement>("#publish-site")!.disabled);
  expect(site.publishedRevision).toBe(2);
});
