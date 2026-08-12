# Collections and human-readable identity

## Human-readable identity

Whenever the API exposes both a machine identifier and a human-readable name/title:

- list/table primary cell: name/title;
- detail heading: name/title;
- selector option: name/title;
- breadcrumb: name/title;
- ID/hash/key: secondary monospace metadata;
- include a copy affordance for long identifiers;
- use the ID as fallback only when no human-readable value exists.

Do not truncate a name to make room for a full opaque ID. Preserve the name and truncate/copy the ID.

If a related entity is represented only by an ID but a lookup endpoint exists, resolve the display name through the owned data layer. Batch lookups or expand/include fields are preferred over one request per row.

## Server-owned operations

Resource collections are searched, sorted, filtered, and paginated by the server.

The browser may:

- render search/filter/sort/page controls;
- debounce user input;
- encode query state in the URL;
- send query parameters or typed RPC fields;
- abort stale requests;
- render a bounded response;
- cache bounded responses according to repository policy.

The browser must not:

- fetch all rows and slice pages locally;
- sort the full resource collection locally;
- filter/search the full resource collection locally;
- silently apply a different ordering than the server;
- retain unbounded prior pages;
- claim total counts it cannot establish.

Tiny fixed option lists that are part of the shipped UI are not resource collections. A live list of users, jobs, sessions, documents, media, runs, or other domain entities is a resource collection.

## Contract shape

Prefer typed server contracts supporting:

- bounded `limit` with a server maximum;
- cursor or offset pagination;
- explicit sort field/direction allowlists;
- typed/allowlisted filters;
- free-text query when needed;
- total count only when the server can provide it efficiently and honestly;
- expanded human-readable related names or a batch lookup path.

URL state can preserve `q`, filters, sorts, cursor/offset, limit, scope, visible columns, and selected detail. Changing query/sort/filter/scope resets pagination. Presentation-only changes do not refetch.

## Missing backend capability

If the API does not support required search/sort/filter/pagination:

1. document the missing operation/fields;
2. add or request the backend contract;
3. generate/update the owned client;
4. keep the UI bounded until the server behavior exists.

Do not use a browser-side workaround that becomes invisible product architecture.

## Verification

- inspect network calls and prove query controls change server requests;
- reload/back/forward and confirm URL state restores the same bounded result where applicable;
- plant more rows than one page and prove the browser never receives the full collection;
- verify primary labels use names/titles and IDs remain available secondarily;
- test missing-name fallback without fabricating content.
