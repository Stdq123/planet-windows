// Refresh each stale subscription without letting a failing source block the rest.
export async function refreshDue(entries: ReadonlyArray<{url:string;lastUpdated?:string}>, refresh:(url:string)=>Promise<unknown>, now:number, interval:number):Promise<void> {
  for (const entry of entries) {
    const updated=entry.lastUpdated ? Date.parse(entry.lastUpdated) : Number.NaN;
    if (Number.isFinite(updated) && now >= updated && now - updated < interval) continue;
    try { await refresh(entry.url); } catch { /* The caller records and displays the failure. */ }
  }
}
