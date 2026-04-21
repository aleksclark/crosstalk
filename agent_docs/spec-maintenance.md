# Spec Maintenance

[← Back to AGENTS.md](../AGENTS.md)

The design spec lives in `spec/`, indexed at `spec/index.md`. It is a living document that must stay in sync with the code.

## Before Implementing

Read the relevant spec section. Check the confidence score — low scores mean the design is untested and may need adjustment during implementation.

## Confidence Scores

Every section in `spec/index.md` has a confidence score (0–10):

| Score | Meaning |
|-------|---------|
| **0** | Untested idea, no code |
| **1–3** | Code being written, not validated |
| **4–6** | Partially validated |
| **7–8** | Mostly proven, edge cases tested |
| **9** | Fully implemented and tested |
| **10** | Battle-tested in real usage |

Update scores when:
- **Increase**: code implements the design, tests pass, real usage confirms
- **Decrease**: implementation reveals design doesn't work, user corrects something

## Commit Discipline

**Every `spec/` change must be in its own commit**, separate from code commits.

Commit message format:
```
spec: <section> — <what changed> (<provenance>)
```

Provenance tags:

| Tag | Use |
|-----|-----|
| `initial` | First draft |
| `clarification` | Ambiguity resolved by user |
| `correction` | Fix something wrong |
| `implemented` | Code now matches design — score up |
| `tested` | Tests confirm behavior — score up |
| `redesign` | Design didn't work — score down + rewrite |
| `expansion` | New detail added |
| `new-section` | Entirely new section |

## When Code Diverges From Spec

If you implement something differently than the spec describes:

1. Make it work in code first
2. In a **separate commit**, update the spec to match what you built
3. Adjust the confidence score (redesign usually means score goes down then back up as the new design is tested)
4. Explain **why** the design changed in the commit message

Never leave the spec and code out of sync.

## Adding New Areas

When you discover something not covered:

1. Create the file in the appropriate `spec/` subfolder
2. Add an entry to `spec/index.md` with confidence `0`
3. Commit with the `new-section` tag
