import { expect, test } from "bun:test";
import { gatewayURL, planetShareURL, selectedGateway, sitePublicURL } from "../src/gateway";

test("custom domain addresses use the chosen gateway for ENS and HTTPS for DNS",()=>{
  expect(sitePublicURL("eth.sucks","k51example","example.eth","/post/")).toBe("https://example.eth.sucks/post/");
  expect(sitePublicURL("ipfs.io","k51example","example.eth","/post/")).toBe("https://ipfs.io/ipns/example.eth/post/");
  expect(sitePublicURL("aleph","k51example","site.example.com","/post/")).toBe("https://site.example.com/post/");
  expect(sitePublicURL("aleph","k51example","example.sol","/")).toBe("https://example.sol.build/");
  expect(()=>sitePublicURL("aleph","k51example","example.com@evil.test","/")).toThrow();
});

test("site gateway addresses follow each provider's URL format", () => {
  expect(gatewayURL("ipfs.io", "k51example")).toBe("https://ipfs.io/ipns/k51example/");
  expect(gatewayURL("eth.sucks", "k51example")).toBe("https://k51example.eth.sucks/");
  expect(gatewayURL("dweb.link", "k51example")).toBe("https://dweb.link/ipns/k51example/");
  expect(gatewayURL("aleph", "k51example")).toBe("https://ipfs.aleph.cloud/ipns/k51example/");
  expect(gatewayURL("eth.sucks", "vitalik.eth", "/article/")).toBe("https://vitalik.eth.sucks/article/");
});

test("only supported Planet links change gateway and article paths are retained", () => {
  const link="https://ipfs.io/ipns/k51example/posts/%E4%BD%A0%E5%A5%BD/";
  expect(planetShareURL("eth.sucks",link,true)).toBe("https://k51example.eth.sucks/posts/%E4%BD%A0%E5%A5%BD/");
  expect(planetShareURL("aleph",link,true)).toBe("https://ipfs.aleph.cloud/ipns/k51example/posts/%E4%BD%A0%E5%A5%BD/");
  expect(planetShareURL("eth.sucks",link,false)).toBe(link);
  expect(planetShareURL("eth.sucks","https://example.com/post",true)).toBe("https://example.com/post");
  expect(() => planetShareURL("aleph","javascript:alert(1)",false)).toThrow();
});

test("unsupported names and stored preferences do not produce unsafe links", () => {
  expect(() => gatewayURL("eth.sucks","QmLegacyIdentity")).toThrow();
  expect(() => gatewayURL("eth.sucks","k51example.attacker.test")).toThrow();
  expect(() => gatewayURL("aleph","../bad")).toThrow();
  expect(selectedGateway("unknown")).toBe("ipfs.io");
  expect(selectedGateway(null)).toBe("ipfs.io");
});
