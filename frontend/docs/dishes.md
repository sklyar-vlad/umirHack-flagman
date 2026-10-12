# Administrative dish catalogue (F19)

`/admin/dishes` is restricted to admin by the shared role matrix; `/admin/recipes`
remains a compatible list alias. `/dishes` and the order composer retain their
existing permissions and requests. The administrative page issues published
`GET /api/v1/dishes` with `is_active=all`, `availability=all`, `sort=name`,
`page=1`, `page_size=50` by default. Search, activity, availability and pagination
are URL state. Changing a filter resets the page; back/forward restores it.
Only allowlisted filters survive login and editor return links.

Names/descriptions, activity, current recipe version, ingredient count and exact
integer portion strings come from the published Dish DTO. The browser never
recalculates or sorts availability. `recipe_missing` and nullable recipe versions
are shown explicitly. Portions describe each dish independently and reserve
nothing. There is no deletion action or endpoint on this screen.

Composition preview performs a fresh `GET /dishes/{id}` and shows each server
ingredient quantity per portion. Loading, 403, 404, malformed replies and network
failures hide old preview contents. Escape closes the modal and restores focus.
List transient failures retain only the last successful data for the same
filters with a stale warning; 403 hides cached content. Mount, focus, reconnect,
manual refresh and the `Dishes` cache tag refresh subscribed lists after changes.
Confirmed F20 mutations invalidate this tag and preserve the allowlisted list
return destination. Creation/edit links open the F20 editor at
`/admin/dishes/new` and `/admin/dishes/:id/edit`.

Development/test MSW is explicitly enabled; production always makes real API
requests. F19 extends the existing scenario infrastructure with synthetic
administrative pagination and a new-version snapshot. The existing order
composition fixtures remain available for their regression scenarios. A mock
new-version snapshot verifies list refresh and preview behavior; it does not
exercise a real recipe save. Real S08/S18/S15 session, RBAC, snapshots, recipe
mutation and cache refresh across clients remain unverified.

## Dish and recipe editor (F20)

`/admin/dishes/new` and `/admin/dishes/:id/edit` now implement the published
S18 writes, guarded by the existing admin role and shared session/CSRF transport.
Names use 1–200 Unicode characters after trimming; description is up to 2000
characters or explicit null for empty input. Recipes contain 1–100 distinct
active product IDs. Consumption is a positive decimal string in the product's
base unit, with at most 11 integer digits and 3 decimal places; pcs are integers.
The UI accepts a decimal comma without floating-point conversion. Historical
product selections remain visible while active options load or become unavailable.
A metadata-only PATCH omits recipe; recipe changes send the entire new array.
PATCH uses the independently held `expected_revision`; focus, catalogue refresh
and reconnect never replace an editor's proposed values or accepted revision.

`POST /dishes/preview` sends only recipe and requires no dish name. It shows
server requirements, shortages, expiration reasons, available portions and
server snapshot time. Editing any field, adding/removing a row, save or explicit
reload invalidates the preview and aborts an outstanding read. Request generation
and input fingerprints reject late results, even after changing values back.
Preview never uses an idempotency key, saves a record or reserves stock.

422 and DUPLICATE_INGREDIENT preserve the draft and focus published field paths.
REVISION_CONFLICT requires explicit fresh reading and review of all current
fields before a new PATCH; proposed fields remain intact. Successful saves show
the returned revision/recipe version and immutable new composition, clear only
that form draft and invalidate Dishes, StopList, HotOffers, Procurement and
Dashboard. An entity change counter invalidates mounted whole-basket checks;
changed accepted recipes still need explicit review before rechecking. Historical
order snapshots and the existing immutable-attempt behavior remain unchanged.

Only allowlisted, non-secret dish form data is persisted in owner-scoped memory
and sessionStorage. Expiration/reload can restore it for the same user; success,
explicit logout or another user clear it. A network/5xx/malformed success leaves
the outcome unknown and prevents automatic replay. Existing dishes require a
fresh read and explicit review. For unknown creation, a separate catalogue tab
and an explicit acknowledgement allow a new write after manual verification.
Route departure protects dirty/busy input and offers an explicit discard.

Production requests remain real. Opt-in development/test MSW extends the
scenario infrastructure with synthetic create/PATCH, preview, revision conflicts,
field errors, delays and lost responses. Synthetic saved snapshots update the
catalogue, order catalogue and stop-list; they do not exercise S18 transactions,
audit/outbox, cookies/CSRF enforcement, real stock locking or cross-client refresh.
Real backend integration remains unverified.
