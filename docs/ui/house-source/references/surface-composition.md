# Surface composition

Choose one primary class before writing tokens or components. If a screen spans two classes, name the secondary class but let the primary class control the composition.

## Monitor

**Job:** watch changing state.

- Begin with one plain-language status sentence.
- Show only measured values that support a decision; use a ruled band, not arbitrary stat cards.
- Follow with activity and detail regions of unequal weight.
- Mark stale, partial, or cached data explicitly.
- Actions are secondary to comprehension.

Reject heroes, centered intros, decorative charts, and grids of equal metric cards.

## Operate

**Job:** act on entities or workflows.

- Keep scope and selection visible near the collection.
- Lead with search/filter controls that execute on the server.
- Use dense ruled rows; human-readable names lead, IDs are secondary.
- Separate routine, privileged, and destructive actions.
- Preview destructive effect before confirmation.

Reject card-per-row layouts, hidden scope, and browser-owned collection logic.

## Configure

**Job:** set up or edit behavior.

- Group fields by outcome, not database schema.
- Wide screens use label/help and control columns; mobile stacks.
- Use progressive disclosure for advanced settings.
- Keep dirty/saving/saved/error state persistent.
- Secret controls explain storage/return behavior.

Reject ornamental cards around every field and surprise autosave.

## Explore

**Job:** browse or search a collection.

- Search leads; filters follow as compact, readable controls.
- Results use human-readable titles and secondary metadata.
- Use a preview/peek panel when it reduces navigation churn.
- URL state may preserve the query, but the server applies it.
- Cursor/offset controls render bounded server responses.

Reject downloading the corpus, client-side transforms, and filter sidebars that dominate the results.

## Command / Inspect

**Job:** drive quickly or examine one object deeply.

- Give the inspected object's human-readable name/title first; ID follows in mono.
- Commands stay near the object and expose keyboard shortcuts.
- Use an event timeline, definition-list metadata, and on-demand raw payload.
- Keep context in a persistent side panel/drawer when possible.
- Show partial provenance and permission constraints.

Reject mini-card metadata and commands detached from target/scope.

## Decide / Learn

**Job:** teach, explain, or persuade.

- This is the only class where a hero is normally appropriate.
- Use Newsreader selectively for display hierarchy.
- One idea lands per section.
- Evidence and explanatory content use aligned columns and rules.
- Keep one primary call to action.

Reject generic three-feature grids, fake testimonials, and operational chrome.

## Responsive rules

- Desktop target: 1280px and wider.
- Tablet target: around 834px.
- Mobile target: around 390px.
- Mobile controls are at least 44px; rows at least 48px.
- Recompose rather than scaling the desktop canvas.
- Preserve information hierarchy and semantic order.
- Dark mode is reviewed first at every width; light is parity.
