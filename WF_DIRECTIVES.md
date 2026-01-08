# Directives

## Migrations

When creating database migrations, use story numbers to prevent conflicts
between parallel workstreams:
- STORY-0023 -> migrations/000023_*.sql
- This ensures concurrent features don't create conflicting sequence numbers

## HTMX-First Design Principles

This codebase uses HTMX for interactivity. Follow these principles:

### Prefer Declarative Over Imperative
- Use `hx-*` attributes on HTML elements instead of JavaScript event listeners
- Use `hx-on:*` attributes for event handling (e.g., `hx-on:htmx:after-swap`)
- Let the server render HTML in the correct state rather than manipulating DOM with JS

### Avoid JavaScript Event Listener Accumulation
- NEVER add event listeners to `document` or `document.body` inside components that re-render
- If you must use JS listeners, use named functions with `removeEventListener` before `addEventListener`
- Prefer `{ once: true }` option for one-time listeners
- Best: use `hx-on:*` attributes which are automatically cleaned up on element removal

### Server-Side State
- Server should return HTML fragments ready to display, not JSON for client processing
- Calculated values (like end times, totals) should be computed server-side
- Form validation feedback should come from server responses

### HTMX Patterns to Follow
```html
<!-- Good: declarative event handling -->
<select hx-on:change="updateEndTime(this)">

<!-- Good: server handles state -->
<div hx-get="/api/slots?date={date}" hx-trigger="change from:#date-picker">

<!-- Bad: JS listener in script tag that runs on every render -->
<script>
  document.body.addEventListener("htmx:afterSwap", ...) // AVOID
</script>
```

### When JavaScript is Necessary
- Keep it minimal and idempotent
- Use `htmx.onLoad()` for initialization that should run once per element
- Scope listeners to specific elements, not document/body
