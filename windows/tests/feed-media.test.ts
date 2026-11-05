import { enterFirstSite } from "./site-workflow";
import { expect, test } from "bun:test";
import { Window, type Document } from "happy-dom";

let serial = 0;
// happy-dom's media .src getters ignore <base>; resolve against the parsed
// document's baseURI as a browser does, then verify actual loading in WebView2.
function resourceURL(doc: Document, selector: string) {
  return new URL(doc.querySelector(selector)!.getAttribute("src")!, doc.baseURI).href;
}
async function openArticle(overrides: Record<string, any> = {}, subscription: Record<string, any> = {}) {
  const window = new Window({url:"http://mygo.localhost/"});
  window.document.write(await Bun.file("index.html").text());
  Object.assign(globalThis, {window, document:window.document});
  const item = {id:"one",key:"one",title:"Post",link:"https://example.test/posts/one/",description:"",content:'<img src="photo.png">',published:"",read:true,...overrides};
  const sub = {url:"https://example.test/rss.xml",lastUpdated:new Date().toISOString(),feed:{title:"Feed",link:"https://example.test/",description:"",items:[item]},...subscription};
  (globalThis as any).mygo = {call:async(method:string)=>{
    switch(method) {
      case "Library.Sites":return [];
      case "Library.Subscriptions":return [sub];
      case "Library.SubscriptionPreview":return '<img src="data:image/png;base64,aW1hZ2U=">';
      default:throw new Error(`Unexpected call ${method}`);
    }
  }};
  await import(`../src/main.ts?feed-media-${++serial}`); enterFirstSite(window);
  for(let i=0;i<100 && !window.document.querySelector("#subscriptions .site-item");i++) await Bun.sleep(10);
  window.document.querySelector<HTMLButtonElement>("#subscriptions .site-item")!.click();
  window.document.querySelector<HTMLButtonElement>(".article-item")!.click();
  await Bun.sleep(10);
  const frame=window.document.querySelector<HTMLIFrameElement>("#reader-preview")!;
  const doc=new window.DOMParser().parseFromString(frame.srcdoc,"text/html");
  const policy=doc.querySelector('meta[http-equiv="Content-Security-Policy"]')!.getAttribute("content")!;
  const directives=Object.fromEntries(policy.split(";").map(s=>s.trim().split(/\s+/)).map(([key,...values])=>[key,values]));
  await window.happyDOM.cancelAsync();
  return {frame,doc,directives};
}

test("online feed articles allow images and inline media in an isolated reader",async()=>{
  const {frame,doc,directives}=await openArticle({content:`<picture><source srcset="large.webp 2x"><img src="photo.png"></picture><video poster="poster.png"><source src="clip.mp4" type="video/mp4"></video><audio src="sound.mp3"></audio>`});
  expect(directives["img-src"] ?? []).toContain("https:");
  expect(directives["img-src"] ?? []).toContain("http:");
  expect(directives["media-src"] ?? []).toContain("https:");
  expect(frame.getAttribute("sandbox")).toBe("");
  expect(directives["default-src"]).toEqual(["'none'"]);
  expect(directives["script-src"]).toEqual(["'none'"]);
  expect(doc.querySelector("base")!.href).toBe("https://example.test/posts/one/");
  expect(resourceURL(doc,"img")).toBe("https://example.test/posts/one/photo.png");
  expect(doc.querySelector("picture source")!.getAttribute("srcset")).toBe("large.webp 2x");
  expect(resourceURL(doc,"video source")).toBe("https://example.test/posts/one/clip.mp4");
  expect(doc.querySelector("video")!.controls).toBe(true);
  expect(doc.querySelector("audio")!.controls).toBe(true);
});

test("relative article links resolve against the feed website instead of the app",async()=>{
  const {doc}=await openArticle({link:"posts/one/"});
  expect(resourceURL(doc,"img")).toBe("https://example.test/posts/one/photo.png");
});

test("articles without a valid link use the feed website as their media base",async()=>{
  for(const link of ["", "javascript:alert(1)", "https://user:pass@example.test/private/"]) {
    const {doc}=await openArticle({link});
    expect(resourceURL(doc,"img")).toBe("https://example.test/photo.png");
  }
});

test("feed markup cannot replace the reader base, redirect it or autoplay media",async()=>{
  const {doc}=await openArticle({content:`<base href="https://wrong.test/"><meta http-equiv="refresh" content="0;url=https://wrong.test/"><p>Article</p><img src="photo.png"><video autoplay src="clip.mp4"></video>`});
  expect(doc.querySelectorAll("base").length).toBe(1);
  expect(resourceURL(doc,"img")).toBe("https://example.test/posts/one/photo.png");
  expect(doc.querySelector('meta[http-equiv="refresh"]')).toBeNull();
  expect(doc.querySelector("video")!.autoplay).toBe(false);
  expect(doc.querySelector('meta[name="referrer"]')!.getAttribute("content")).toBe("no-referrer");
});

test("offline and Planet previews keep remote requests disabled",async()=>{
  for(const overrides of [{offlineBase:"https://example.test/"},{contentCid:"bafy",articleId:"one"}]) {
    const {doc,directives}=await openArticle(overrides);
    expect(directives["img-src"]).toEqual(["data:"]);
    expect(directives["media-src"] ?? []).not.toContain("https:");
    expect(doc.querySelector("img")!.src).toBe("data:image/png;base64,aW1hZ2U=");
  }
  const {directives}=await openArticle({}, {offlinePinned:true});
  expect(directives["img-src"] ?? []).not.toContain("https:");
});
