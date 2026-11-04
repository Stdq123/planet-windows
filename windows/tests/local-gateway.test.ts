import {expect, test} from "bun:test";
import {localGatewayResource} from "../src/local-gateway";

test("local gateway resolves IPFS and IPNS resource paths without changing origin",()=>{
  expect(localGatewayResource("http://127.0.0.1:1234", "ipfs://bafy123/hello world.png")).toBe("http://127.0.0.1:1234/ipfs/bafy123/hello%20world.png");
  expect(localGatewayResource("http://127.0.0.1:1234", "/ipns/example.eth/a/index.html")).toBe("http://127.0.0.1:1234/ipns/example.eth/a/index.html");
  expect(localGatewayResource("http://localhost:1234", "/ipfs/bafy123")).toBeNull();
  expect(localGatewayResource("http://127.0.0.1:1234", "/ipfs/bafy123/../api")).toBeNull();
  expect(localGatewayResource("http://127.0.0.1:1234", "https://example.com/a")).toBeNull();
});
