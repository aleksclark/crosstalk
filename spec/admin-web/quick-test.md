# Quick-Test Flow

[← Back to Index](../index.md) · [Admin Web Overview](overview.md)

---

## Motivation

Rapid iteration is critical during development. Starting a test session should be one click, not a multi-step workflow through template selection and role assignment.

## The Button

On the dashboard, a prominent button:

> **[Quick Test]** — Creates a new session from the default template and connects as "translator"

## Behavior

1. `POST /api/sessions` with the default template ID, auto-generated name (e.g., `"Quick Test 2026-04-21 10:30"`)
2. Redirect to `/sessions/:id/connect?role=translator`
3. Auto-request mic permission
4. Establish WebRTC connection
5. Join session as `translator` role
6. Full session connect view is active and ready

## Requirements

- Requires exactly one template flagged as `is_default`
- If no default template exists, button is disabled with tooltip: "Set a default template first"
- The `translator` role must exist in the default template — the quick-test always connects as `translator`

## Session Cleanup

Quick-test sessions are normal sessions — they persist until explicitly ended. The session connect view has an "End Session" button that tears everything down.

No auto-expiry, but quick-test sessions could be visually distinguished in the session list (e.g., with a "quick test" badge).
