# Live server snapshots

The application opens one cookie-authenticated native `EventSource` at
`/api/v1/events` for the active session. Events use the published S24 contract;
they contain topics, timestamps and UUIDs, never replacement business DTOs.
The shared transport imports no application, feature or entity code.

The application maps topics to existing RTK Query tags and merges bursts over
150 ms. Inventory changes also refresh current supply balances, loss reports
and forecasts; orders refresh successful/failed history and shortage analytics.
Ledger invalidation uses `StockMovements`, losses use `WriteOffReport`.
Role filtering reuses the navigation permission matrix. Only subscribed reads
refetch; inactive cached records are invalidated. Detached form revisions have
no tags and are never silently replaced.

The `Products` read tag is permitted for every role, matching published
`GET /products`, which also feeds reception choices. Its refresh does not grant
access to administrative routes or product writes.

`inventory.time_changed` refreshes expiry snapshots only when a cached batch
crosses its expiry or restaurant warning boundary. Ordinary clock ticks do not
trigger REST requests. Stock arithmetic remains on the server. Ingredient, recipe and time changes invalidate the current
whole-basket check marker and trigger the existing debounced check. The basket,
accepted recipe snapshots and unresolved immutable write attempts are retained.
Recipe changes still require explicit review. Events never reserve stock or
replace final server validation when submitting an order.

Cursors are exact decimal int64 values. Duplicate and older deliveries are
ignored; malformed messages are not applied. Native reconnect on the same
instance sends `Last-Event-ID` automatically. No unsupported query parameter
is sent. A replacement instance starts at the current server tail and performs
a full REST refresh. Cursor is scoped to one stream/session and never persisted
or transferred between owners. `resync.required` rereads all role-permitted
active snapshots and invalidates the basket check.

Initial reads belong to the page queries. Reconnect and online recovery refresh
REST independently of notifications; focus or visibility restoration while the
stream is healthy does not trigger another read. Existing mutation invalidations
and mount refresh remain in place. Page-specific polling is consolidated here:
while disconnected, visible online reads refresh every 10 seconds, up to six
attempts. Recovery cancels that timer. Five consecutive stream errors close the
native retrying instance; online/visibility restoration permits a fresh
attempt and resets the polling budget. Offline/hidden pages do not poll.
After exhaustion, manual page retry or online/visibility restoration can refresh;
cached data is not asserted to be current indefinitely.

Stream failures cannot reveal HTTP status through native EventSource. They
therefore confirm the session through `/auth/me`; confirmations are coalesced.
Only confirmed 401/403 expires local authentication; transport/server failures
preserve work. External settings/identity changes update safe session values.
Changed roles clear protected cache and restart the role-scoped subscription.
Logout, expiry, account switch and provider unmount close all stream resources.

Explicit development/test MSW handles the stream and provides Replay/duplicate,
resync and disconnect scenarios alongside existing REST fixtures. An isolated
local SSE browser fixture verifies native Last-Event-ID transmission without
service-worker interception, using the same shared connection controller. Production
never substitutes mocks. Tests of synthetic notifications verify frontend
behavior. The [F31 production acceptance](acceptance.md) verifies authenticated
native SSE delivery through the existing nginx and matching Traefik flush settings
with a disposable real backend/database, without MSW. Durable replay across a
long deployment, real worker time markers and deployment-specific session
revocation still require separate server integration checks.
