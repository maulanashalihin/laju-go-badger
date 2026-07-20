# Laju Go — Agent Instructions

High-performance SaaS boilerplate: Go Fiber + Svelte 5 + Inertia.js + Badger KV + templ.

> 🔴 **JANGAN jalankan `npm run dev:all` atau dev server apapun.** User yang handle dev server secara manual.

## Architecture

Single-entry Go app at `cmd/laju-go/main.go`.

```
routes/web.go → app/handlers/ → app/services/ → app/repositories/ → Badger KV
                                    ↕
                              Cache (in-memory)
```

- **Handlers**: Parse requests, call services, return responses. No business logic. **Setiap module/feature harus handler file terpisah** — jangan bloat satu file dengan banyak route berbeda.
- **Services**: Business logic, auth flows, external APIs. Boleh panggil `s.querier.*` **atau** `s.cache.*` untuk read.
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

**Handler → Service → Repository → DB.** Tidak ada layer yang boleh lompat.

| Layer | Boleh | Tidak Boleh |
|-------|-------|-------------|
| **Handler** | Parse request, panggil **Service**, return response | Panggil Repository, akses DB, business logic |
| **Service** | Business logic, panggil `s.querier.*` atau `s.cache.*` | `badger.Open`, raw KV ops |
| **Cache** | In-memory session cache (sync.RWMutex + map), dipanggil via Service | Akses langsung dari Handler |
| **Repositories** | **SATU-SATUNYA** yang touch Badger (`*badger.Txn`, `txn.Get/Set/Delete`) | — |

⚠️ Pengecualian: File test (`*_test.go`) BOLEH panggil repositories langsung.

🔴 **Ini aturan paling penting — jangan pernah dilanggar.**

## Handler Structure Rule (🔴 Penting)

**Setiap module/feature harus handler file terpisah.** Jangan satukan semua route ke satu handler.

| ✅ Benar | ❌ Salah |
|----------|----------|
| `app/handlers/auth.go` — login, register, OAuth | `app/handlers/handler.go` — 1000+ line semua route |
| `app/handlers/app.go` — dashboard, profile | |
| `app/handlers/password-reset.go` — forgot/reset password | |
| `app/handlers/upload.go` — file upload | |
| `app/handlers/public.go` — landing page, public routes | |

**Pattern handler method per feature**:

```go
// app/handlers/orders.go — contoh
func (h *OrderHandler) List(c *fiber.Ctx) error { ... }
func (h *OrderHandler) Create(c *fiber.Ctx) error { ... }
func (h *OrderHandler) Show(c *fiber.Ctx) error { ... }
func (h *OrderHandler) Cancel(c *fiber.Ctx) error { ... }
```

Setiap handler struct punya dependency sendiri, jangan numpuk di satu struct raksasa.

## Design Principles (Wajib Dibaca Sebelum Generate Frontend)

Sebelum nulis kode frontend apapun (halaman baru, komponen, landing page):

1. **`wiki_recall`** `design-principles` — baca brief inference, set three dials, apply anti-slop rules
2. **Pilih vibe** — refer ke vibe pages: [[vibe-minimalist]], [[vibe-premium-consumer]], [[vibe-playful-experimental]], [[vibe-dark-tech]], [[vibe-brutalist]]
3. **Cek `frontend/src/app.css`** — gunakan token warna yang sudah ada (`brand-*`, `secondary-*`, `neutral-*`). Jangan define ulang
4. **Apply `@theme` tokens** — semua warna/shadow/font sudah di `@theme`. Jangan hardcode hex
5. **Pre-flight checklist** — dari `design-principles` section 5, sebelum declare selesai

## Svelte 5 Rules

- ❌ Jangan `$effect` untuk derived state → ganti `$derived()`
- ❌ Jangan `$effect` untuk init state dari props → `$state(value ?? default)`
- ✅ `$effect` hanya untuk side effects: `document.title`, `localStorage`
- ✅ Internal link WAJIB `use:inertia` dari `@inertiajs/svelte` — tanpanya full page reload
- 🔴 **fetch() CSRF header**: tiap `fetch()` ke `/app/*` atau `/admin/*` WAJIB `X-XSRF-TOKEN` dari `getCSRFToken()` (`lib/utils/csrf.ts`). Inertia's `router.*` auto-handle ini.
- Form submission pake `router.post()`/`router.put()`, bukan `<form>` biasa
- File upload via `fetch() + FormData`, simpan URL hasil via `router.put()`
- OAuth links (`/auth/google`, `/auth/github`) pake `<a>` biasa tanpa `use:inertia`

## HTTP Conventions

- POST/PUT redirect: `h.inertiaService.Redirect(c, path)` — otomatis 303 See Other, Inertia-aware
- External redirect (OAuth, logout ke external): `h.inertiaService.Location(c, url)` — 409 Conflict + `X-Inertia-Location` → trigger `window.location`
- Back navigation: `h.inertiaService.Back(c)` atau `h.inertiaService.Back(c, "/fallback")`
- PUT/PATCH: return JSON untuk `fetch()`, redirect 303 untuk `router.put()`
- `fiber.Map` untuk adhoc response data. Typed structs untuk service boundaries.

## Testing

- `go test ./...` — unit/integration (in-memory Badger via `badger.DefaultOptions("").WithInMemory(true)`, no mock)
- **agent_browser E2E**: inject session langsung via Badger untuk skip login. Detail di wiki: [Agent Browser Testing](.llm-wiki/wiki/concepts/agent-browser-testing.md)

## Database Rules (Badger KV)

- 🔴 **Badger is schema-less — no migrations.** The `data/badger/` directory IS the database.
- **Key prefixes** are the "schema": `user:<id>`, `idx:user:email:<email>`, `idx:user:google:<gid>`, `session:<id>`, `idx:sess:u:<uid>:<sid>`, `pwreset:<token>`. See `app/repositories/db.go`.
- **User IDs are ULIDs** (`oklog/ulid/v2`) — string-typed, lexicographically sortable.
- To reset: `make db-reset` (deletes `data/badger/`).
- To inspect: use `badger` CLI or write a one-off script that opens the DB read-only.

## Gotchas

- 🔴 **Edit `.templ` saja, jangan `*_templ.go`.** File `*_templ.go` akan ditimpa `templ generate`
- `.vite-port` stale? `rm .vite-port && restart Vite`
- `app/services/inertia.go` wraps `github.com/maulanashalihin/fiber-inertia` (published library) — semua method (Render, Redirect, Location, Back) di-promote via embedding
- `go.sum` is gitignored — `go mod tidy` if needed
- `dist/` gitignored kecuali `.gitkeep`
- Air tidak watch `.templ` — regenerate manual

## Wiki (Detail Lebih Lanjut)

Detail lebih lanjut (deployment, design standards, HTTP conventions, migration convention, dsb) ada di `.llm-wiki/wiki/`. Gunakan tools wiki native: `wiki_search`, `wiki_recall`, `wiki_ensure_page`, `wiki_observe`, `wiki_retro`.

Atau langsung: `read_file`, `grep`, `glob` pada path `.llm-wiki/`.
