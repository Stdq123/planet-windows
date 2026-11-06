# Quiet writing workspace implementation plan

**Goal:** Implement the approved editable Figma proposal as a working Windows overview, on `feat/figma-writing-workspace`, then package a local preview.

**Design:** https://www.figma.com/design/C7q0U2bLBOQihrNC6hYKjT?node-id=3-51
**Architecture:** Keep Go, MyGo/WebView2, the existing TypeScript dashboard controller and local metadata API. Replace the overview composition and navigation styling; preserve existing editing, publishing, subscriptions and analytics.
**Constraints:** No merge or push to main, no dependency/framework migration, no real user data in fixtures. Native Windows title bar remains native. Preserve Windows fonts, with Figma geometry checked in the actual renderer.

- [x] Apply local Figma SVG assets and tokens; rebuild the compact header, inline totals, writing rows, site rail, responsive layout and empty state.
- [x] Wire site selection for writing, recent article filters, expandable recent lists, error/retry and live publication states. Add tests for changed interactions and retain stale-request safety.
- [x] Run `bun run check`; verify populated/empty/error screens and interactions at 1280, 1024 and 860 widths, including clipping and assets.
- [ ] Update Unreleased changelog, commit the reviewed changes on the feature branch, run `bun run build`, and launch an isolated local preview from its exact build directory.

## Review focus
- Long titles/site names and small native windows must not hide controls.
- Multi-site writing must use the selected site; no selection means no accidental composition.
- Switching screens or refreshing must discard obsolete asynchronous responses.
- Publish states must reflect local/backend state; saved private drafts must not be described as automatically published or autosaved.
- Native window controls, existing sidebar tools and editor navigation must remain usable.
