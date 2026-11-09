# Workspace dashboard

The user selected an all-sites startup overview and authorized direct implementation. Use light navigation with amber accents, following the user's preference for a light sidebar. Preserve Windows typography and existing editor/reader workflows.

The default view is Overview. Show active-site, article, private-draft and unread counts; six recently modified drafts and six articles across active sites; site publication states; an on-demand IPFS status panel; and optional per-site Plausible queries using the existing configured service. Never invent analytics or replace missing/error results with zero. Show a useful empty state with create/import actions.

Use a read-only Store snapshot under one lock, returning metadata only (no bodies, attachments, credentials or network requests). Exclude removed/deleting sites and their content. Sort recent items by modification time with deterministic ties. Reuse existing navigation and unsaved-change guards. Entering the dashboard invalidates pending navigation, and late responses cannot switch views or overwrite a newer refresh. Refresh on dashboard entry and when existing publication/subscription state changes.

Use 24/18/14/12px typography, 4px spacing units, 8px controls and 16px panels on a cold light-gray canvas. The shared sidebar is near-white with pale amber selection and dark amber selected text. The overview leads with an amber writing area and a local decorative SVG, then four pastel metrics. Drafts form a compact two-column grid; articles remain scannable rows, with publication/service information alongside. Empty workspaces have a direct create action, three-step introduction and import action. Keep navigation labels at the normal 1280px window size. Collapse the dashboard auxiliary column before navigation; metrics become two columns on narrow windows. Validate the 860px minimum window and increased text/display scale. All actions are keyboard accessible with visible focus, safe text rendering and explicit loading/error/retry states.

Approaches considered: repurpose the current three-column reader (poor space use for cross-site content); load all articles/drafts through frontend calls (copies private bodies and scales poorly); a dedicated metadata snapshot and dashboard module (selected implementation).

No new dependencies, account/notification system, revenue/likes/comments, scheduled publishing, automatic publishing triggered by the dashboard, or editor/reader layout replacement. Existing startup publication behavior remains separate.
