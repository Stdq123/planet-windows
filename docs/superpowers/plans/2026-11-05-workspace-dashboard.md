# Workspace Dashboard Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans for inline implementation. The user authorized direct execution.

**Goal:** Make an actionable all-sites overview the startup view.
**Architecture:** A read-only store metadata snapshot feeds a focused dashboard module; main.ts connects existing navigation/actions and preserves guards.
**Tech Stack:** Go, TypeScript, existing CSS, Bun/happy-dom, MyGo/WebView2.
**Spec:** ../specs/2026-11-05-workspace-dashboard-design.md

## Global Constraints

- No new dependencies; preserve existing light theme and Windows system fonts.
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

- [ ] Add failing store tests for totals, exclusions, ordering, limits and no body serialization.
- [ ] Implement Store.Dashboard() DashboardSnapshot in internal/store/dashboard.go; expose Library.Dashboard() (store.DashboardSnapshot, error) in dashboard.go.
- [ ] Run go test ./internal/store -run Dashboard and regenerate MyGo bindings.
- [ ] Commit the verified API increment.

## Task 2: Dashboard view and navigation

- [ ] Add dashboard module tests for loading/error/retry, recent row actions, analytics state and stale responses.
- [ ] Implement src/dashboard.ts, src/dashboard.css, semantic markup and navigation wiring in main.ts/index.html. Entry defaults to overview; existing tests explicitly enter the site workflow they exercise.
- [ ] Add integration tests for startup, cross-site draft/article entry, navigation races and unsaved-edit guards.
- [ ] Run focused Bun suites, then bun run check. Commit the feature with CHANGELOG entry.

## Task 3: Verification and delivery

- [ ] Review the complete branch independently; address substantive findings with regression coverage.
- [ ] Verify the native desktop empty/populated/error views and narrow layout using an isolated profile. Capture evidence.
- [ ] Push, create PR, wait for mandatory CI, merge and sync main; confirm main CI.
