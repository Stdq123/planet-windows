// Only the daemon's loopback gateway may be used as the origin.
export function localGatewayResource(gateway: string, input: string): string | null {
  const origin = /^http:\/\/127\.0\.0\.1:([0-9]{1,5})$/.exec(gateway);
  if (!origin || Number(origin[1]) < 1 || Number(origin[1]) > 65535) return null;
  const value = input.trim().replace(/^(ipfs|ipns):\/\//i, "/$1/");
  const match = /^\/(ipfs|ipns)\/([^/]+)(?:\/(.*))?$/i.exec(value);
  if (!match || !/^[A-Za-z0-9][A-Za-z0-9.-]*$/.test(match[2]!)) return null;
  try {
    const segments = (match[3] ?? "").split("/").map(segment=>decodeURIComponent(segment));
    if (segments.some(segment=>segment === "." || segment === ".." || /[\/\\\u0000-\u001f\u007f]/.test(segment))) return null;
    const suffix = match[3] === undefined ? "" : "/" + segments.map(segment=>encodeURIComponent(segment)).join("/");
    return `${gateway}/${match[1]!.toLowerCase()}/${match[2]}${suffix}`;
  } catch { return null; }
}
