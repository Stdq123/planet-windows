import { expect, test } from "bun:test";
import { Window } from "happy-dom";

let serial = 0;
async function until(predicate: () => boolean) {
  for (let i = 0; i < 100; i++) { if (predicate()) return; await Bun.sleep(10); }
  throw new Error("Favorite state did not settle");
}

async function fixture(options: { starred?: boolean; offline?: boolean; beforeStar?: () => Promise<void> } = {}) {
  const window = new Window({url:"http://localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document:window.document});
  const saved = ["first", "second"].map(name => ({
    url:`https://${name}.test/feed`, lastUpdated:new Date().toISOString(),
    feed:{title:name,link:`https://${name}.test/`,description:"",items:[{
      id:"one",key:"one",title:`${name} article`,link:`https://${name}.test/post/`,
      description:"",content:'<p>Article body</p><img src="image.png"><video controls src="video.mp4"></video>',
      published:"",read:true,starred:!!options.starred,
      ...(options.offline ? {offlineBase:`https://${name}.test/post/`} : {})
    }]}
  }));
  let starCalls = 0;
  (globalThis as any).mygo = {call:async(method:string,...args:any[]) => {
    switch(method) {
      case "Library.Sites":return [];
      // The bridge returns separate objects; changing the UI must not mutate saved data.
      case "Library.Subscriptions":return structuredClone(saved);
      case "Library.StarSubscription": {
        starCalls++;
        await options.beforeStar?.();
        saved.find(sub=>sub.url===args[0])!.feed.items.find(item=>item.key===args[1])!.starred=args[2];
        return;
      }
      case "Library.SetSubscriptionRead":saved.find(sub=>sub.url===args[0])!.feed.items[0]!.read=args[2];return;
      case "Library.SubscriptionPreview":return '<p>Offline article</p><img src="data:image/png;base64,aW1hZ2U=">';
      default:throw new Error(`Unexpected call: ${method}`);
    }
  }};
  await import(`../src/main.ts?favorite-reader-${++serial}`);
  const $ = <T extends HTMLElement = HTMLElement>(selector:string) => window.document.querySelector<T>(selector)!;
  const pick = (index:number) => {
    window.document.querySelectorAll<HTMLButtonElement>("#subscriptions .site-item")[index]!.click();
    $<HTMLButtonElement>(".article-item").click();
  };
  await until(() => window.document.querySelectorAll("#subscriptions .site-item").length===2);
  pick(0);
  const frame = $<HTMLIFrameElement>("#reader-preview");
  if (options.offline) await until(()=>frame.srcdoc.includes("data:image/png;base64"));
  const changes:any[]=[];
  const observer=new window.MutationObserver(records=>changes.push(...records));
  observer.observe(frame,{attributes:true,attributeFilter:["srcdoc","src"]});
  return {window,$,saved,pick,changes,starCalls:()=>starCalls,close:async()=>{observer.disconnect();await window.happyDOM.cancelAsync();}};
}

test("starring and unstarring update the count without reloading online or offline article media",async()=>{
  for (const offline of [false,true]) {
    const f=await fixture({offline});
    try {
      const star=f.$<HTMLButtonElement>("#star-item");
      star.click();
      await until(()=>star.textContent==="取消收藏" && !star.disabled);
      expect(f.saved[0]!.feed.items[0]!.starred).toBe(true);
      expect(f.$("#favorites-count").textContent).toBe("1");
      expect(f.changes).toHaveLength(0);
      star.click();
      await until(()=>star.textContent==="收藏" && !star.disabled);
      expect(f.saved[0]!.feed.items[0]!.starred).toBe(false);
      expect(f.$("#favorites-count").textContent).toBe("0");
      expect(f.changes).toHaveLength(0);
    } finally { await f.close(); }
  }
});

test("removing a favorite updates the favorites list while keeping its reader document",async()=>{
  const f=await fixture({starred:true});
  try {
    f.$<HTMLButtonElement>("#show-favorites").click();
    f.$<HTMLButtonElement>(".article-item").click();
    await Bun.sleep(10);f.changes.length=0;
    f.$<HTMLButtonElement>("#star-item").click();
    await until(()=>f.$("#favorites-count").textContent==="1");
    expect(f.$("#articles").textContent).not.toContain("first article");
    expect(f.$("#articles").textContent).toContain("second article");
    expect(f.$("#reader-title").textContent).toBe("first article");
    expect(f.$("#reader").hidden).toBe(false);
    expect(f.changes).toHaveLength(0);
  } finally { await f.close(); }
});

test("a failed favorite save keeps the article and favorite state unchanged and allows retry",async()=>{
  let fail=true;
  const f=await fixture({beforeStar:async()=>{if(fail)throw new Error("storage unavailable");}});
  try {
    const star=f.$<HTMLButtonElement>("#star-item");
    star.click();await until(()=>!f.$("#error").hidden && !star.disabled);
    expect(f.$("#error").textContent).toContain("storage unavailable");
    expect(star.textContent).toBe("收藏");
    expect(f.$("#favorites-count").textContent).toBe("0");
    expect(f.changes).toHaveLength(0);
    fail=false;star.click();await until(()=>star.textContent==="取消收藏" && !star.disabled);
    expect(f.$("#favorites-count").textContent).toBe("1");
    expect(f.changes).toHaveLength(0);
  } finally { await f.close(); }
});

test("pending favorite saves stay disabled across state updates and do not reload a different article",async()=>{
  let release:()=>void=()=>{};
  const pending=new Promise<void>(resolve=>{release=resolve;});
  const f=await fixture({beforeStar:()=>pending});
  try {
    const star=f.$<HTMLButtonElement>("#star-item");
    star.click();await until(()=>f.starCalls()===1);
    f.$<HTMLButtonElement>("#toggle-read").click();
    await until(()=>f.$("#toggle-read").textContent==="标为已读");
    expect(star.disabled).toBe(true);
    star.click();expect(f.starCalls()).toBe(1);
    f.pick(1);await Bun.sleep(10);f.changes.length=0;
    expect(star.disabled).toBe(false);
    release();await until(()=>f.$("#favorites-count").textContent==="1");
    expect(f.$("#reader-title").textContent).toBe("second article");
    expect(star.textContent).toBe("收藏");
    expect(f.saved[0]!.feed.items[0]!.starred).toBe(true);
    expect(f.saved[1]!.feed.items[0]!.starred).toBe(false);
    expect(f.changes).toHaveLength(0);
  } finally { release();await f.close(); }
});
