# CrossTalk SPA House Conformance Ledger

**Branch:** `feature/spa-house-design`  
**Base:** `production-integration@2b627f80e38aa0369dc1e7fb28e96ba7e934fb80`  
**Accent:** cyan `#3DE0F0` (sole product accent)  
**Token source:** `packages/theme/theme.css` (only)

## Surface classification

| Route | Class | Status |
|---|---|---|
| Admin login | Operate / Access | PASS |
| Admin dashboard | Monitor | PASS (truthful collection snapshot; no hard-coded health) |
| Admin sessions | Operate / Explore | PASS (server q/sort/cursor) |
| Admin session detail | Operate / Inspect | PASS |
| Admin ABCs | Operate / Monitor | PASS (session_name first) |
| Admin ABC detail | Operate / Inspect | PASS (minimal visual; no K2B audio controls) |
| Admin translators | Operate / Explore | PASS |
| Admin debug | Monitor / Inspect | PASS (name-first; debug REST still handwritten) |
| Translator login | Operate / Access | PASS |
| Translator sessions | Operate | PASS |
| Translator connect | Operate / Monitor | PASS |
| Broadcast listen | Monitor / Listen | PASS |

## Matrix rows

| Dimension | Verdict | Evidence |
|---|---|---|
| One accent #3DE0F0 | PASS | `pnpm --filter @crosstalk/theme check:house` GREEN |
| One token source | PASS | theme.css only; adapters map aliases |
| Fonts Archivo/Newsreader/IBM Plex Mono | PASS | self-hosted WOFF2; body computed Archivo in E2E |
| Server-owned collections | PASS | ListSessions/ABCs/Translators q/sort/limit/cursor + next_cursor/total |
| Human-readable identity | PASS | names lead lists/details; CopyableId secondary |
| Auth / tickets / WebRTC / reconnect | PASS | `task test:e2e` 16/16 including golden-audio 440/880 |
| Loading/empty/error/denied/stale/success | PASS | DataState/Status across SPAs |
| Keyboard / focus / mobile floors | PASS | shell dialog trap; 44px mobile tokens |
| Dark canonical | PASS | screenshots dark desktop+mobile |
| Light parity toggle | INTENTIONAL | tokens include light; no half-supported user toggle |
| Compact density | INTENTIONAL | comfortable only |
| Debug generated REST | BLOCKED residual | debug peers still use owned handwritten fetch; functional |
| Axe serious/critical | PASS | accessibility-visual.spec.ts |
| Golden audio | PASS | floor→translator 439Hz; translator→broadcast 879Hz |

## Gates (final SHA)

See implementation run report for command outputs and screenshot checksums.
