# Web UI

[← Back to AGENTS.md](../AGENTS.md)

## Stack

- React + Vite + TypeScript (strict mode)
- shadcn/ui components, dark mode
- Vitest for testing
- ESLint for linting
- pnpm for package management

## Directory Structure

```
web/
├── src/
│   ├── components/      # Reusable UI components
│   ├── pages/           # Route-level page components
│   ├── lib/
│   │   └── api/         # Generated TypeScript API client
│   ├── App.tsx
│   └── main.tsx
├── package.json
├── vite.config.ts
└── tsconfig.json         # References tsconfig.app.json (strict mode)
```

## Hosting

The web UI is **not** a standalone app. It is served by the Go server:

- **Production**: `web/dist/` is embedded in `ct-server` via `go:embed`
- **Dev**: Vite runs on the host, Go server proxies non-API requests to it
- **No CORS** in any mode — same origin always

This means: no `VITE_API_URL` config in production. API calls use relative paths (`/api/...`).

## Commands

```bash
task dev:vite          # Start Vite dev server
task test:unit:web     # Run Vitest
task lint:web          # ESLint + typecheck
task build:web         # Production build → web/dist/
```

## TypeScript Conventions

- **Strict mode** is enabled (`"strict": true` in tsconfig.app.json)
- No `any` — use proper types or `unknown`
- Prefer named exports over default exports
- API client is generated — don't edit `web/src/lib/api/` by hand

## Testing

- Vitest with jsdom environment
- Test files next to source: `Foo.tsx` → `Foo.test.tsx`
- Use `@testing-library/react` for component tests
- Run: `task test:unit:web` or `cd web && pnpm test run`

## Adding a New Page

1. Create `web/src/pages/MyPage.tsx`
2. Add route in the router
3. Write `MyPage.test.tsx`
4. Run `task test:unit:web && task lint:web`
