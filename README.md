# Laju Go

High-performance SaaS boilerplate built with **Go Fiber** + **Svelte 5** + **Inertia.js 3** + **Badger KV** (pure-Go, no CGO).

Build production-ready web applications faster with clean layered architecture that combines the speed of Go with the DX of modern frontend frameworks. Ships with **Svelte 5** by default, but Inertia.js makes it trivial to swap to **React** or **Vue** without changing any Go code.

## 🚀 Quick Start

```bash
git clone https://github.com/maulanashalihin/laju-go.git
cd laju-go
cp .env.example .env
go mod download && npm install
npm run dev:all
```

Visit `http://localhost:8080` to see your application running.

## ✨ Features

### File Upload (TUS Protocol)

- **TUS Resumable Upload** — Powered by [tusdfiber](https://github.com/maulanashalihin/tusdfiber), native Go Fiber implementation
- **Drag & Drop** — Test page at `/app/upload` with progress bars via `tus-js-client`
- **Large File Support** — Up to 1GB per upload, chunked via PATCH requests
- **Post-Processing** — Completed files copied to `storage/completed/` for easy access

### Authentication & Security

- **Email/Password** — Argon2id hashing, session management
- **Google OAuth 2.0** — One-click social login
- **Password Reset** — Email-based recovery with secure tokens
- **CSRF Protection** — Double-submit cookie pattern (stateless, no session I/O)
- **Rate Limiting** — Configurable throttling for auth, API, upload endpoints
- **Session Fixation Protection** — Session ID regenerated on privilege escalation

### User Management

- **Role-Based Access Control** — Admin/User roles with middleware guards
- **Profile Management** — Update name, email, avatar
- **Avatar Upload** — With file type/size validation

### Development Experience

- **Hot Module Replacement** — Vite HMR for instant frontend updates
- **Go Hot Reload** — Air rebuilds on Go file changes (~1-2s)
- **Clean Architecture** — Handler → Service → Query (hand-written Badger ops)
- **Full TypeScript** — Every `.svelte` file uses `<script lang="ts">`
- **Type-Safe Templates** — Go HTML via [templ](https://templ.guide/)

### Performance & Database

- **Badger KV (dgraph-io/badger/v4)** — Pure-Go embedded LSM-tree KV store, no CGO
- **Schema-less** — Key prefixes act as the schema (`user:<id>`, `idx:user:em:<email>`, etc.)
- **In-Memory Session Cache** — Fast sync.RWMutex+map cache for session data
- **Background Cleanup** — Expired sessions & tokens auto-purged every hour

## 📁 Project Structure

```
laju-go/
├── cmd/laju-go/main.go        # Application entry point
├── app/                       # Backend Go code
│   ├── handlers/              # HTTP request handlers
│   ├── services/              # Business logic layer
│   ├── repositories/           # Hand-written Badger KV operations
│   ├── middlewares/           # Auth, CSRF, rate limiting
│   ├── cache/                 # In-memory session cache
│   ├── models/                # Data structures + DTOs
│   ├── session/               # Session store (Badger + cache)
│   └── config/                # Env-based configuration
├── frontend/                  # Svelte 5 frontend
│   └── src/
│       ├── components/        # Header, DarkModeToggle
│       ├── pages/auth/        # Login, Register, ForgotPassword, ResetPassword
│       ├── pages/app/         # Dashboard, Profile, UploadTest
│       └── lib/i18n/          # Internationalization (en/id)
├── routes/                    # Route definitions
├── templates/                 # templ HTML components
├── docs/                      # Documentation
└── systemd/                   # Production service file
```

## 🛠️ Tech Stack

| Layer | Technology |
|-------|------------|
| **Backend** | Go 1.26+, Fiber v2 |
| **Database** | Badger KV via `dgraph-io/badger/v4` (pure-Go, no CGO) |
| **Data Access** | Hand-written Badger operations (no codegen) |
| **Migrations** | None — Badger is schema-less |
| **Frontend** | Svelte 5 (rune-based) |
| **Build Tool** | Vite 8 |
| **Styling** | Tailwind CSS 4 |
| **Templating** | templ — type-safe Go HTML |
| **SPA Bridge** | Inertia.js 3 via [fiber-inertia](https://github.com/maulanashalihin/fiber-inertia) |
| **Icons** | Lucide Svelte |

### Why `dgraph-io/badger/v4` (pure-Go)?

Badger is a pure-Go embedded LSM-tree key-value store — no CGO, no external C dependencies. Benefits:

- ✅ **No CGO** — `CGO_ENABLED=0`, trivial cross-compilation with stock `go build`
- 🛠️ **Cross-compile** via `make build-linux` (no `zig cc` needed)
- ➡️ **Static binary** by default — no C libraries to link

For development (macOS) and CI, builds are fast and reproducible with no toolchain setup.

## ⚡ Quick Reference

```bash
# Development
npm run dev:all                # Vite + Air (hot reload both)

# Build (production)
npm run build:all              # vite build → go build

# Verify (before commit)
npm run verify                 # templ → vite → go build → go vet → go test

# Build for Linux (from macOS)
make build-linux               # CGO_ENABLED=0, no zig cc needed

# Database
npm run db:reset               # delete data/badger/ and start fresh

# Templates
templ generate                 # regenerate templ Go files

# Verify (before commit/deploy)
npm run verify                 # templ → vite → go build → go vet

# Test
go test ./...                  # backend tests
```

### Testing Strategy

| Approach | For | Command |
|----------|-----|---------|
| Go unit/integration | Services, repositories, handlers | `go test ./...` |
| E2E / user flow | Visual regression, auth flows, form submission | `agent_browser` via pi |

> **E2E testing** dilakukan manual dengan `agent_browser` (buka browser, klik, isi form, verify redirect).
> Tidak perlu Cypress/Playwright — browser asli lebih realistik untuk project skala ini.

## 🚀 Deployment (Your Workflow)

```bash
# 1. Pull latest
git pull

# 2. Build
npm run build:all

# 3. Restart service
sudo systemctl restart laju-go
```

Only runtime artifacts needed on server:

- `laju-go` binary
- `dist/` — frontend assets
- `.env` — configuration
- `data/badger/` — Badger KV data directory (auto-created on startup)

> **Note**: No Go, Node, or npm needed on the server — just the binary + assets. No migrations to run.

## 🗄️ Database

Badger is schema-less — no migrations needed. The data directory (`data/badger/`) is created automatically on startup. Key prefixes act as the schema:

| Prefix | Purpose |
|--------|---------|
| `user:<id>` | User record (ULID key) |
| `idx:user:em:<email>` | Email → user ID index |
| `idx:user:go:<gid>` | Google ID → user ID index |
| `session:<id>` | Session record |
| `idx:sess:u:<uid>:<sid>` | User → session index |
| `pwreset:<token>` | Password reset token |

To reset the database (deletes all data):

```bash
npm run db:reset       # deletes data/badger/
# or
make db-reset
```

## 📖 Documentation

| Section | Description |
|---------|-------------|
| [Architecture](docs/guide/architecture.md) | Layered design, patterns, conventions |
| [Database](docs/guide/database.md) | Badger KV setup, key prefixes, repositories |
| [Frontend](docs/guide/frontend.md) | Svelte 5 + Inertia.js patterns |
| [Deployment](docs/deployment/production.md) | Systemd, Nginx, production setup |
| [Benchmarks](docs/benchmark/) | Badger KV performance data |

## 📄 License

MIT — see [LICENSE](LICENSE).
