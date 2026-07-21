# Laju Go — Agent Instructions

High-performance SaaS boilerplate: Go Fiber + Svelte 5 + Inertia.js + Badger KV + templ.

> 🔴 **Never run `npm run dev:all` or any dev server.** The user handles dev servers manually.

## Architecture

Single-entry Go app at `cmd/laju-go/main.go`.

```
routes/web.go → app/handlers/ → app/services/ → app/repositories/ → Badger KV
                  ↕ app/models/        ↕            ↕
                  (domain contract)  Cache      (persistence shape)
```

- **Models**: `app/models/` — **shared domain contract** (zero dependency, zero logic). Entity structs (`User`, `Session`), DTOs (`RegisterRequest`, `UserResponse`), and enums (`UserRole`). Used by every layer as lingua franca. The `json:"-"` tag on sensitive fields (`Password`, `GoogleID`) guarantees they never leak into API responses. Repositories own their own persistence shape (`userRecord`, `repositories.Session`) and convert to/from `models.*`.
- **Handlers**: Parse requests, call services, return responses. No business logic. **Each module/feature must have its own handler file** — never bloat a single file with unrelated routes.
- **Services**: Business logic, auth flows, external APIs. May call `s.querier.*` **or** `s.cache.*` for reads.
- **Repositories**: `app/repositories/` is **hand-written** for Badger (pure-Go LSM-tree KV store via `github.com/dgraph-io/badger/v4`). Key-prefix indexing for email, google_id, per-user sessions. User IDs are ULIDs (`oklog/ulid/v2`). Schema-less — no migrations.
- **Cache**: `app/cache/` — In-memory session cache (sync.RWMutex + map).
- **Frontend**: Svelte 5 + Inertia.js. Entry at `frontend/src/main.ts`. Built to `dist/`.

| Task | npm | Make |
|------|-----|------|
| Dev (both) | `npm run dev:all` | — |
| Dev Go only | `npm run dev:go` | — |
| Build | `npm run build:all` | `make build` |
| Test | `go test ./...` | `make test` |
| Generate templ | `templ generate` | `make templ` |
| Reset DB | `npm run db:reset` | `make db-reset` |

> **Build order matters**: `vite build` must run before `go build` because the Go binary reads `dist/.vite/manifest.json`.
> **No migrations**: Badger is schema-less. To reset the DB, delete the `data/badger/` directory (`make db-reset`).

## Three-Tier Rule (🔴 CRITICAL)

**Handler → Service → Repository → DB.** No layer may skip.

| Layer | May | May Not |
|-------|-----|---------|
| **Models** | Define entity struct, DTO, enum. Used by every layer. Zero imports from other layers. | Import `repositories`, `services`, or `badger`. Any logic. |
| **Handler** | Parse request into `models.*DTO`, call **Service**, return response | Call Repository, access DB, business logic |
| **Service** | Business logic, call `s.querier.*` or `s.cache.*`, return `models.*` | `badger.Open`, raw KV ops |
| **Cache** | In-memory session cache (sync.RWMutex + map), called via Service | Direct access from Handler |
| **Repositories** | **The ONLY** layer that touches Badger (`*badger.Txn`, `txn.Get/Set/Delete`). Convert `models.*` ↔ persistence shape (`userRecord`, `repositories.Session`). | — |

⚠️ Exception: Test files (`*_test.go`) MAY call repositories directly.

🔴 **This is the most important rule — never violate it.**

## Models Convention (🔴 Important)

`app/models/` is the **shared domain contract** — the cheapest package (zero dependency) yet invaluable as an anti-corruption layer between domain and storage.

**Rules:**
- ✅ One file per **domain aggregate** (not per layer). Example: `user.go` contains entity `User` + enum `UserRole` + DTO `UserResponse` + `RegisterRequest`.
- ✅ DTO may be split into a separate file when the aggregate grows: `order.go` (entity) + `order_dto.go` (request/response).
- ✅ `json:"-"` tag on sensitive fields (`Password`, `GoogleID`) — a **security invariant** centralised here.
- ✅ Repository owns its persistence shape (`userRecord`, `repositories.Session`) and converts via `toModel()`/`fromModel()`. Storage changes do not break the domain contract.
- ❌ Do not put logic in `models/`. This package is structs + converter methods only.
- ❌ Do not import `models/` from the frontend (Go-only package).
- ❌ Do not define DTOs in handlers/services — put them in `models/` for typed cross-layer contracts.

**File growth as features are added:**

```
app/models/
├── user.go           # User entity + UserRole enum + UserResponse
├── session.go        # Session entity + SessionData
├── dto.go            # Auth/profile DTOs (Register, Login, UpdateProfile)
├── order.go          # NEW: Order entity + OrderStatus enum
├── order_dto.go      # NEW: CreateOrderRequest, OrderResponse
└── product.go        # NEW: Product entity + Category enum
```

## Handler Structure Rule (🔴 Important)

**Each module/feature must have its own handler file.** Never merge all routes into a single handler.

| ✅ Correct | ❌ Wrong |
|------------|---------|
| `app/handlers/auth.go` — login, register, OAuth | `app/handlers/handler.go` — 1000+ lines, all routes |
| `app/handlers/app.go` — dashboard, profile | |
| `app/handlers/password-reset.go` — forgot/reset password | |
| `app/handlers/upload.go` — file upload | |
| `app/handlers/public.go` — landing page, public routes | |

**Handler method pattern per feature:**

```go
// app/handlers/orders.go — example
func (h *OrderHandler) List(c *fiber.Ctx) error { ... }
func (h *OrderHandler) Create(c *fiber.Ctx) error { ... }
func (h *OrderHandler) Show(c *fiber.Ctx) error { ... }
func (h *OrderHandler) Cancel(c *fiber.Ctx) error { ... }
```

Each handler struct has its own dependencies — never pile everything into one giant struct.

## Design Principles (Required Reading Before Generating Frontend)

Before writing any frontend code (new pages, components, landing pages):

1. **`wiki_recall`** `design-principles` — read the brief inference, set three dials, apply anti-slop rules
2. **Pick a vibe** — refer to vibe pages: [[vibe-minimalist]], [[vibe-premium-consumer]], [[vibe-playful-experimental]], [[vibe-dark-tech]], [[vibe-brutalist]]
3. **Check `frontend/src/app.css`** — use existing color tokens (`brand-*`, `secondary-*`, `neutral-*`). Do not redefine.
4. **Apply `@theme` tokens** — all colors/shadows/fonts are in `@theme`. Do not hardcode hex.
5. **Pre-flight checklist** — from `design-principles` section 5, before declaring done.

## Svelte 5 Rules

- ❌ Do not use `$effect` for derived state → use `$derived()`
- ❌ Do not use `$effect` to init state from props → `$state(value ?? default)`
- ✅ `$effect` is only for side effects: `document.title`, `localStorage`
- ✅ Internal links MUST use `use:inertia` from `@inertiajs/svelte` — without it, a full page reload occurs
- 🔴 **fetch() CSRF header**: every `fetch()` to `/app/*` or `/admin/*` MUST include `X-XSRF-TOKEN` from `getCSRFToken()` (`lib/utils/csrf.ts`). Inertia's `router.*` handles this automatically.
- Form submissions use `router.post()`/`router.put()`, not plain `<form>`
- File uploads via `fetch() + FormData`; persist the resulting URL via `router.put()`
- OAuth links (`/auth/google`, `/auth/github`) use plain `<a>` without `use:inertia`

## HTTP Conventions

- POST/PUT redirect: `h.inertiaService.Redirect(c, path)` — automatically 303 See Other, Inertia-aware
- External redirect (OAuth, logout to external): `h.inertiaService.Location(c, url)` — 409 Conflict + `X-Inertia-Location` → triggers `window.location`
- Back navigation: `h.inertiaService.Back(c)` or `h.inertiaService.Back(c, "/fallback")`
- PUT/PATCH: return JSON for `fetch()`, 303 redirect for `router.put()`
- `fiber.Map` for adhoc response data. Typed structs for service boundaries.

## Testing

- `go test ./...` — unit/integration (in-memory Badger via `badger.DefaultOptions("").WithInMemory(true)`, no mock)
- **agent_browser E2E**: inject the session directly via Badger to skip login. Details in the wiki: [Agent Browser Testing](.llm-wiki/wiki/concepts/agent-browser-testing.md)

## Database Rules (Badger KV)

- 🔴 **Badger is schema-less — no migrations.** The `data/badger/` directory IS the database.
- **Key prefixes** are the "schema": `user:<id>`, `idx:user:email:<email>`, `idx:user:google:<gid>`, `session:<id>`, `idx:sess:u:<uid>:<sid>`, `pwreset:<token>`. See `app/repositories/db.go`.
- **User IDs are ULIDs** (`oklog/ulid/v2`) — string-typed, lexicographically sortable.
- To reset: `make db-reset` (deletes `data/badger/`).
- To inspect: use the `badger` CLI or write a one-off script that opens the DB read-only.

## Gotchas

- 🔴 **Edit `.templ` only, never `*_templ.go`.** Files `*_templ.go` are overwritten by `templ generate`.
- `.vite-port` stale? `rm .vite-port && restart Vite`
- `app/services/inertia.go` wraps `github.com/maulanashalihin/fiber-inertia` (published library) — all methods (Render, Redirect, Location, Back) are promoted via embedding
- `go.sum` is gitignored — `go mod tidy` if needed
- `dist/` is gitignored except `.gitkeep`
- Air does not watch `.templ` — regenerate manually

## Wiki (Further Details)

Further details (deployment, design standards, HTTP conventions, migration conventions, etc.) are in `.llm-wiki/wiki/`. Use the native wiki tools: `wiki_search`, `wiki_recall`, `wiki_ensure_page`, `wiki_observe`, `wiki_retro`.

Or directly: `read_file`, `grep`, `glob` on the `.llm-wiki/` path.
