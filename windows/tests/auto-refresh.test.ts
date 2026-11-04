import { expect, test } from "bun:test";
import { refreshDue } from "../src/auto-refresh";

test("refreshes stale subscriptions while continuing after failures", async () => {
 const visited:string[]=[];const now=Date.parse("2026-01-01T01:00:00Z");
 await refreshDue([{url:"stale",lastUpdated:"2026-01-01T00:00:00Z"},{url:"fresh",lastUpdated:"2026-01-01T00:55:00Z"},{url:"never"}],async address=>{visited.push(address);if(address==="stale")throw new Error("offline")},now,15*60*1000);
 expect(visited).toEqual(["stale","never"]);
});
