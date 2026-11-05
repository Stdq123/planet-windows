import { readingStyles } from "./ui";

function webURL(value: string, base?: string): string {
  if (!value.trim()) return "";
  try {
    const url = new URL(value, base || undefined);
    return (url.protocol === "http:" || url.protocol === "https:") && !url.username && !url.password ? url.href : "";
  } catch { return ""; }
}

export function feedArticleBase(articleLink: string, feedLink: string, source: string): string {
  const sourceURL = webURL(source);
  const feedURL = webURL(feedLink, sourceURL) || sourceURL;
  return webURL(articleLink, feedURL) || feedURL;
}

// This document is always displayed in an iframe with an empty sandbox.
// Web feeds may load media; fixed and Planet previews only load resolved data URLs.
export function feedReaderDocument(html: string, localImages: boolean, remoteBase = ""): string {
  const baseURL = webURL(remoteBase);
  const online = !localImages && !!baseURL;
  const images = localImages ? "img-src data:;" : online ? "img-src data: https: http:; media-src https: http:;" : "";
  const content = document.createElement("template");
  content.innerHTML = html;
  // Feed markup must not replace the trusted base or navigate the reader via refresh.
  for (const node of content.content.querySelectorAll("base, meta")) node.remove();
  for (const media of content.content.querySelectorAll("audio, video")) {
    media.setAttribute("controls", "");
    media.setAttribute("preload", "none");
    media.removeAttribute("autoplay");
  }
  const base = document.createElement("base");
  base.href = baseURL;
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'none'; ${images} style-src 'unsafe-inline'; form-action 'none'; base-uri ${online ? "https: http:" : "'none'"}">${online ? base.outerHTML : ""}<style>${readingStyles}</style></head><body>${content.innerHTML}</body></html>`;
}
