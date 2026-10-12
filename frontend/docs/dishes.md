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
A future F20 confirmed mutation must invalidate this tag and return to the
allowlisted list destination. Creation/edit links open explicit F20 entry screens
at `/admin/dishes/new` and `/admin/dishes/:id/edit`; the editor is separate scope.

Development/test MSW is explicitly enabled; production always makes real API
requests. F19 extends the existing scenario infrastructure with synthetic
administrative pagination and a new-version snapshot. The existing order
composition fixtures remain available for their regression scenarios. A mock
new-version snapshot verifies list refresh and preview behavior; it does not
exercise a real recipe save. Real S08/S18/S15 session, RBAC, snapshots, recipe
mutation and cache refresh across clients remain unverified.
