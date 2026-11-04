export const gatewayKey = "planet.publicGateway";
export type Gateway = "ipfs.io" | "eth.sucks" | "dweb.link" | "aleph";

export function selectedGateway(value: string | null): Gateway {
  return value === "eth.sucks" || value === "dweb.link" || value === "aleph" ? value : "ipfs.io";
}

// Paths passed here are already URL-encoded paths from a verified Planet link.
export function gatewayURL(gateway: Gateway, name: string, path = "/"): string {
  if (!name || /[\\/\s?#@]/u.test(name) || !path.startsWith("/") || path.startsWith("//")) throw new Error("IPNS 地址无效");
  if (gateway === "eth.sucks") {
    let host: string;
    if (/^(?:k51|bafz)[a-z0-9]*$/.test(name) && name.length <= 63) host = `${name}.eth.sucks`;
    else if (/^[\p{L}\p{N}.-]+\.eth$/iu.test(name)) host = `${name}.sucks`;
    else throw new Error("此 IPNS 名称暂不支持 eth.sucks，请选择其他网关");
    return new URL(path.slice(1), `https://${host}/`).href;
  }
  const host = gateway === "aleph" ? "ipfs.aleph.cloud" : gateway;
  return `https://${host}/ipns/${encodeURIComponent(name)}${path}`;
}

export function sitePublicURL(gateway: Gateway, ipns: string, domain = "", path = "/"): string {
  if(!domain)return gatewayURL(gateway,ipns,path);
  const value=domain.toLowerCase();
  if(value.length>253 || !/^(?=.{1,253}$)[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/.test(value) || !/^[a-z]{2,63}$/.test(value.split(".").at(-1) || "")) throw new Error("站点域名无效");
  if(!path.startsWith("/") || path.startsWith("//")) throw new Error("站点路径无效");
  if(value.endsWith(".eth"))return gatewayURL(gateway,value,path);
  const host=value.endsWith(".sol") ? `${value}.build` : value.endsWith(".bit") ? `${value}.site` : value;
  return new URL(path.slice(1),`https://${host}/`).href;
}

// Only rewrite links generated for Planet/IPNS subscriptions, not ordinary feeds.
export function planetShareURL(gateway: Gateway, link: string, isPlanet: boolean): string {
  const url = new URL(link);
  if ((url.protocol !== "http:" && url.protocol !== "https:") || url.username || url.password) throw new Error("文章没有可分享的网页链接");
  if (!isPlanet || url.hostname !== "ipfs.io") return url.href;
  const match = /^\/ipns\/([^/]+)(\/.*)?$/.exec(url.pathname);
  if (!match) return url.href;
  return gatewayURL(gateway, decodeURIComponent(match[1]!), match[2] || "/") + url.search + url.hash;
}
