# Editorial Instrument design language

This reference is normative. Projects may select one allowed accent; every other visual foundation is rigid unless a local design authority records an explicit exception. Comfortable density is the default; compact density requires an explicit task or local product requirement.

## Character

Editorial Instrument combines editorial hierarchy with instrument-like controls:

- precise, serious, and calm;
- dense without visual clutter;
- distinctive without ornament;
- technical without looking like an IDE;
- suitable for professional and consumer-facing products.

Dark mode is canonical. Light mode keeps identical geometry, hierarchy, component states, and semantic roles.

## Typography

Load these exact families:

| Role | Family | Use |
|---|---|---|
| Product | Archivo | UI, body, controls, section/page titles |
| Editorial | Newsreader | Display copy on Decide/Learn or artifact-centric introductions only |
| Technical | IBM Plex Mono | IDs, timestamps, units, metadata, code, compact uppercase labels |

Fallbacks exist only for font-load failure; they are not alternatives.

### Fixed scale

| Token | Desktop | Mobile | Weight / line height | Family |
|---|---:|---:|---|---|
| display | 48px | 32px | 400 / 1.05 | Newsreader |
| page-title | 30px | 22px | 700 / 1.12, -0.022em | Archivo |
| section | 15px | 15px | 600 / 1.2 | Archivo |
| lede | 14px | 14px | 400 / 1.55 | Archivo |
| body | 14px | 14px | 400 / 1.6 | Archivo |
| body-compact | 13px | 13px | 400 / 1.5 | Archivo |
| control | 13px | 13px | 500 / 1.0 | Archivo |
| label | 12px | 12px | 500 / 1.3 | Archivo |
| metadata | 11px | 11px | 400 / 1.5 | IBM Plex Mono |
| eyebrow | 10px | 10px | 500 / 1.0, 0.16em uppercase | IBM Plex Mono |
| data | 30px | 22px | 500 / 1.0, tabular nums | IBM Plex Mono |
| code | 12px | 12px | 400 / 1.7 | IBM Plex Mono |

Do not invent intermediate font sizes. Recompose when content does not fit.

## Spacing and density

Normative scale: `4, 8, 12, 16, 24, 32, 48px`.

Fixed profiles (comfortable is mandatory by default; compact requires an explicit task or local product requirement):

| Token | Comfortable | Compact | Mobile floor |
|---|---:|---:|---:|
| control height | 34px | 28px | 44px |
| collection row | 40px | 30px | 48px |
| page gutter | 28px | 20px | 16px |
| nav width | 218px | 218px | bottom/overlay nav |
| tablet nav | 186px | 186px | — |
| reading width | 960px | 960px | 100% |

Density changes component spacing, not arbitrary typography. Agents never select compact density merely to fit more content or preserve an old layout.

## Color

Use semantic aliases only. The dark and light ramps live in the scaffold tokens.

Choose exactly one accent:

- violet `#B98BFF`
- cyan `#3DE0F0`
- mint `#7CFFB2`

All three use dark ink on bright fills. A product must not use multiple accents. Status colors are fixed and never repurposed as brand/category colors.

Surface roles:

1. canvas — page ground;
2. sunken — navigation, wells, code, recessed controls;
3. surface — panels, modal body;
4. raised — hover, selected-neutral, skeleton.

Text roles: primary, secondary, tertiary. Rule roles: subtle and strong.

## Shape and elevation

- radius small/medium: 2px;
- pill: only tags, compact status labels, and switch tracks;
- borders: 1px; selected/section rules may use 2px;
- no card shadow;
- no gradient, glass, blur, glow, or ornamental texture;
- modal and toast are the only elevated components.

Use a flat surface budget: if spacing and a rule can establish hierarchy, do not add another container.

## Icon system

- 24px design grid;
- 1.75px stroke;
- round caps and joins;
- no icon-specific backgrounds or rounded-square containers;
- render 16px default, 14px compact, 18px emphasized;
- `currentColor` only;
- status icons accompany text and color;
- navigation and actions pair icons with human labels;
- icon-only controls are reserved for universal actions and require accessible names.

The SVG sprite is in `assets/house-icons.svg`. A third-party set may substitute only with a documented one-to-one semantic map and comparable optical weight.

## Component rules

### Navigation

Use icon + human-readable label. Active state uses a 2px accent rule plus a low-chroma selected ground. Do not rely on icon or color alone.

### Page header

Operational surfaces use:

1. eyebrow/kicker;
2. page title;
3. concise lede;
4. actions;
5. optional ruled metadata strip.

No centered hero outside Decide/Learn.

### Buttons

- primary: accent fill + dark accent ink;
- secondary: transparent + strong rule;
- ghost: transparent, no default border;
- destructive: transparent danger treatment until final confirmation;
- minimum mobile target 44px;
- icons are 14–16px and do not replace non-universal text labels.

### Inputs

Sunken ground, strong rule, 2px focus outline with 2px offset. Labels and help remain outside the control. Validation includes icon/text, not red border alone.

### Status

Fixed roles: ok, warning, danger, info. Always icon + label; optional tinted ground stays restrained.

### Lists and tables

The row's human-readable name/title is primary. IDs, hashes, owner, timestamps, and state are secondary. Selection uses accent rule + ground shift, not elevation. Keep results bounded and server-owned.

### Forms

Use label/help column and control column on wide screens; stack on mobile. Put one subtle rule between fields. Advanced behavior lives behind disclosure. Keep save state persistent and explicit.

### Empty/error/denied/stale

Explain what happened, what remains valid, and the action that resolves it. Include trace/request metadata only as secondary mono text.

### Modals and toasts

These are the only elevated components. Destructive confirmation names action, human-readable target, scope, and impact. Toasts announce with the correct live-region role.

## Anti-patterns

Reject:

- stock framework neutral theme;
- Inter/system font as the product face;
- centered max-width card stacks for every workflow;
- equal-weight card grids;
- blue/violet AI gradients or glassmorphism;
- icon-topped prose cards;
- decorative left accent rails;
- huge fake metrics;
- excessive pills or rounded rectangles;
- arbitrary shadows;
- emoji product iconography;
- machine IDs as primary labels when a name exists;
- browser-side resource collection search/sort/filter/pagination.
