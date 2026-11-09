# Workspace Dashboard Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans for inline implementation. The user authorized direct execution.

**Goal:** Make an actionable all-sites overview the startup view.
**Architecture:** A read-only store metadata snapshot feeds a focused dashboard module; main.ts connects existing navigation/actions and preserves guards.
**Tech Stack:** Go, TypeScript, existing CSS, Bun/happy-dom, MyGo/WebView2.
**Spec:** ../specs/2026-11-05-workspace-dashboard-design.md

## Global Constraints

- No new dependencies; use the light sidebar and amber overview design with Windows system fonts.
- Metadata only; exclude removed/deleting sites; at most six recent drafts and articles.
- No fabricated metrics or automatic external analytics queries.
- Preserve unsaved edits, latest-request ownership, safe text rendering and keyboard access.
- Keep 860px minimum desktop width usable; quality and security CI remain enabled.

## Review Focus

- Removed sites and orphaned content must not affect totals or recent rows (store test).
- Identical modification times must yield stable ordering and bounded recent lists (store test).
- A slow navigation/refresh must not override the user's later selection (UI test).
- Failed/unconfigured analytics and failed overview reads must not look like zero activity (UI test).
- Returning home with unsaved edits must honor cancellation and preserve text (integration test).

## Task 1: Metadata snapshot

- [x] Add failing store tests for totals, exclusions, ordering, limits and no body serialization.
- [x] Implement Store.Dashboard() DashboardSnapshot in internal/store/dashboard.go; expose Library.Dashboard() (store.DashboardSnapshot, error) in dashboard.go.
- [x] Run go test ./internal/store -run Dashboard and regenerate MyGo bindings.
- [x] Commit the verified API increment.

## Task 2: Dashboard view and navigation

- [x] Add dashboard module tests for loading/error/retry, recent row actions, analytics state and stale responses.
- [x] Implement src/dashboard.ts, src/dashboard.css, semantic markup and navigation wiring in main.ts/index.html. Entry defaults to overview; existing tests explicitly enter the site workflow they exercise.
- [x] Add integration tests for startup, cross-site draft/article entry, navigation races and unsaved-edit guards.
- [x] Run focused Bun suites, then bun run check. Commit the feature with CHANGELOG entry.

## Task 3: Verification and delivery

- [x] Review the complete branch independently; address substantive findings with regression coverage.
- [x] Verify populated/empty/error states, narrow layouts, focus and increased scale in Edge; build the native app. The first native empty-state inspection passed; subsequent desktop interaction was stopped by the user. Keep that scope explicit in the verification record.
Delivery: push the branch, open a PR, require passing CI, merge and sync main, then confirm main CI. The PR and Actions runs record delivery status.

## Verification record

- `bun run check`: 133 frontend tests pass; Go tests, formatting, vet, TypeScript and production build pass.
- Independent review caught two navigation/read-status races, both reproduced in failing tests and fixed. The visual revision received a further independent review with no findings.
- Edge rendering at 1440, 1280 and 860 CSS pixels: no horizontal overflow; draft navigation, analytics configuration feedback and empty-workspace create action pass. Static/hover clip audit reports zero clipped or hidden targets.
- 125% and 150% browser zoom have no horizontal overflow. Keyboard focus is visible and Enter opens the selected-site editor. Actual Chinese font use was verified as Microsoft YaHei, with Segoe UI numerals.
- `bun run build` passes the native bundle and installer payload audit. The installer was not installed as part of this feature verification.
- Browser preview data is synthetic. Native desktop interaction beyond the initial empty view was stopped; no real account, publishing or installation acceptance is claimed.
