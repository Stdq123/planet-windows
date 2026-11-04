import { defineConfig } from "mygo-cli";
import release from "./release.json";

export default defineConfig({
  name: "Planet Windows",
  identifier: "org.planetable.planetwindows",
  version: release.version,
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "powershell -NoProfile -File scripts/prepare-notices.ps1 && bun run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",
  windows: { installerInclude: "scripts/installer-safe-uninstall.nsh" },
});
