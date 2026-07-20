# Database

This guide covers database setup, key-prefix schema design, and the repository layer in Laju Go.

## Overview

Laju Go uses **Badger** (`github.com/dgraph-io/badger/v4`) as the database — a pure-Go embedded LSM-tree + value log KV store. This combination provides:

- **Zero configuration** — No database server to manage, no CGO toolchain required
- **Schema-less** — No migrations needed; the "schema" is encoded in key prefixes
- **Pure-Go** — `CGO_ENABLED=0` produces static binaries
- **Production-ready** — Badger manages its own goroutines; no WAL mode, PRAGMA, or connection pooling to tune

## Database Setup

### Connection Initialization

```go
// cmd/laju-go/main.go
import (
    "github.com/dgraph-io/badger/v4"
)

func initDatabase(dbPath string) (*badger.DB, error) {
    // dbPath is a directory (e.g. ./data/badger), not a file
    db, err := badger.Open(badger.DefaultOptions(dbPath))
    if err != nil {
        return nil, err
    }

    return db, nil
}
```

Badger stores data in a directory (default `./data/badger`), not a single file. The directory holds the LSM-tree (`.sst` files) and the value log (`vlog` files).

### Badger Internals (No Tuning Needed)

Badger is an LSM-tree + value log store. Unlike SQLite, there are no PRAGMAs, no WAL mode toggle, and no connection pool to configure — Badger manages its own background goroutines for compaction and value-log garbage collection.

| Aspect | Badger | SQLite (previous) |
|--------|--------|-------------------|
| Storage | LSM-tree + value log (directory) | Single file |
| Schema | Schema-less (key prefixes) | Tables + indexes |
| Concurrency | Single process, internal goroutines | WAL mode + busy_timeout |
| Tuning | Defaults are production-ready | PRAGMAs + connection pool |
| CGO | None (pure-Go) | Required (mattn/go-sqlite3) |

### Why dgraph-io/badger/v4 (pure-Go, no CGO)?

The project migrated from `github.com/mattn/go-sqlite3` (CGO-based) to `github.com/dgraph-io/badger/v4` (pure-Go). The trade-off is worth it for deployment simplicity:

| Factor | `dgraph-io/badger/v4` |
|--------|------------------------|
| CGO | Not required — `CGO_ENABLED=0` static binaries |
| Dockerfile | No gcc/musl-dev/sqlite-static needed |
| Cross-compile | Standard `go build` works everywhere |
| Dev setup | Works out of the box on any platform with a Go toolchain |

> **DB path**: Use a directory path (e.g. `./data/badger`), not a file. Badger creates the directory and its SST/value-log files inside it.

## Schema Design (No Migrations)

Badger is schema-less — there are no migrations, no `CREATE TABLE` statements, and no goose. The "schema" is encoded in key prefixes, and entities are stored as JSON-encoded values.

### Key-Prefix Schema

| Key | Value | Purpose |
|-----|-------|---------|
| `user:<id>` | JSON User | Primary user record (ULID id) |
| `idx:user:em:<email>` | `<id>` | Email → user id index |
| `idx:user:go:<gid>` | `<id>` | Google ID → user id index |
| `session:<id>` | JSON Session | Primary session record |
| `idx:sess:u:<uid>:<sid>` | (empty) | Per-user session index (list/delete all) |
| `pwreset:<token>` | JSON PasswordReset | Password reset record |

### Why No Migrations?

Because Badger is schema-less, schema evolution happens in application code:

- **Add a field** — add it to the Go struct; old records decode with zero values
- **Rename a field** — handle in code (read old key, write new key)
- **Add an index** — backfill `idx:` keys in a one-time script

There is no `goose_db_version` table, no `migrations/` directory, and no startup migration step. The application starts and opens Badger directly.

### Background Cleanup

A background goroutine runs hourly to delete expired sessions and password resets:

```go
// Hourly cleanup of expired sessions and password resets
go func() {
    ticker := time.NewTicker(time.Hour)
    for range ticker.C {
        cleanupExpiredSessions(db)
        cleanupExpiredPasswordResets(db)
    }
}()
```

## Repository Layer — Hand-Written Badger Operations

Laju Go writes Badger operations directly in `app/repositories/`. Instead of generating code from SQL with sqlc, you write Go functions that read/write JSON-encoded values under structured keys with key-prefix indexing.

### Workflow

```
1. Write Badger ops → app/repositories/user.go
2. Use code         → s.repository.GetUserByEmail(ctx, email)
```

### Why Hand-Written Badger Ops Instead of Squirrel/ORM?

| Approach | Runtime Safety | Performance | Boilerplate |
|----------|---------------|-------------|-------------|
| **Badger ops** | Compile-time (Go structs) | Native KV | Low |
| Squirrel | Runtime errors | SQL building overhead | Manual per query |
| GORM | Reflection bugs | Slow (reflect) | Minimal |

### Writing Badger Operations

Write operations in `app/repositories/*.go`:

```go
// app/repositories/user.go

// GetUserByEmail looks up the email index, then fetches the user record.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
    var user models.User
    err := r.db.View(func(txn *badger.Txn) error {
        // 1. Resolve email → id via index key
        item, err := txn.Get([]byte("idx:user:em:" + email))
        if err != nil {
            if errors.Is(err, badger.ErrKeyNotFound) {
                return ErrUserNotFound
            }
            return err
        }
        var id string
        if err := item.Value(func(val []byte) error {
            id = string(val)
            return nil
        }); err != nil {
            return err
        }
        // 2. Fetch the user record
        item, err = txn.Get([]byte("user:" + id))
        if err != nil {
            return err
        }
        return item.Value(func(val []byte) error {
            return json.Unmarshal(val, &user)
        })
    })
    if err != nil {
        return nil, err
    }
    return &user, nil
}

// CreateUser writes the user record plus email/google_id index keys.
func (r *Repository) CreateUser(ctx context.Context, user *models.User) error {
    return r.db.Update(func(txn *badger.Txn) error {
        data, _ := json.Marshal(user)
        if err := txn.Set([]byte("user:"+user.ID), data); err != nil {
            return err
        }
        txn.Set([]byte("idx:user:em:"+user.Email), []byte(user.ID))
        if user.GoogleID != "" {
            txn.Set([]byte("idx:user:go:"+user.GoogleID), []byte(user.ID))
        }
        return nil
    })
}
```

### Files

```
app/repositories/
├── db.go                    # Badger DB init + helpers
├── models.go                # Go structs for stored entities (JSON values)
├── repository.go            # Repository wrapper (what services use) + sentinel errors
├── user.go                  # User CRUD operations (key-prefix indexed)
├── session.go               # Session CRUD operations (key-prefix indexed)
└── session_helpers.go       # Helper functions
```

### Using Repositories in Services

```go
type AuthService struct {
    repository *repositories.Repository
}

func (s *AuthService) Login(email, password string) (*models.User, error) {
    user, err := s.repository.GetUserByEmail(context.Background(), email)
    if err != nil {
        if errors.Is(err, repositories.ErrUserNotFound) {
            return nil, ErrInvalidCredentials
        }
        return nil, err
    }
    // ... validate password ...
    return user, nil
}
```

### Repository Wrapper Pattern

The `app/repositories/repository.go` wraps `*badger.DB` to add convenience methods and sentinel errors:

```go
type Repository struct {
    db *badger.DB
}

func NewRepository(db *badger.DB) *Repository {
    return &Repository{db: db}
}
```

### Complete CRUD Example

**Operations** (`app/repositories/user.go`):

```go
func (r *Repository) GetUserByID(ctx context.Context, id string) (*models.User, error)
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error)
func (r *Repository) GetUserByGoogleID(ctx context.Context, gid string) (*models.User, error)
func (r *Repository) CreateUser(ctx context.Context, user *models.User) error
func (r *Repository) UpdateUser(ctx context.Context, user *models.User) error
func (r *Repository) DeleteUser(ctx context.Context, id string) error
```

**Usage in service** (`app/services/auth.go`):

```go
func (s *AuthService) Register(name, email, password string) (*models.User, error) {
    // Check if user already exists
    _, err := s.repository.GetUserByEmail(context.Background(), email)
    if err == nil {
        return nil, repositories.ErrUserAlreadyExists
    }

    // Hash password
    hashedPassword, _ := hashPassword(password)

    // Create user (ULID id generated inside)
    user := &models.User{
        Email:    email,
        Name:     name,
        Password: hashedPassword, // plain string; empty = null
        Role:     models.RoleUser,
    }

    if err := s.repository.CreateUser(context.Background(), user); err != nil {
        return nil, err
    }

    return user, nil
}
```

### Error Handling with Sentinel Errors

Badger returns `badger.ErrKeyNotFound` for missing keys. The Repository wrapper converts these to domain-specific sentinel errors:

```go
var (
    ErrUserNotFound         = errors.New("user not found")
    ErrUserAlreadyExists    = errors.New("user already exists")
    ErrSessionNotFound      = errors.New("session not found")
    ErrPasswordResetNotFound = errors.New("password reset not found")
)

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
    // ... inside r.db.View ...
    if errors.Is(err, badger.ErrKeyNotFound) {
        return nil, ErrUserNotFound
    }
    // ...
}
```

### Adding a New Operation

1. Add a method to `app/repositories/*.go`:

```go
func (r *Repository) GetUserCount(ctx context.Context) (int, error) {
    count := 0
    err := r.db.View(func(txn *badger.Txn) error {
        opts := badger.DefaultIteratorOptions
        opts.Prefix = []byte("user:")
        it := txn.NewIterator(opts)
        defer it.Close()
        for it.Rewind(); it.Valid(); it.Next() {
            count++
        }
        return nil
    })
    return count, err
}
```

2. Use in service:

```go
count, err := s.repository.GetUserCount(ctx)
```

## Key-Prefix Schema

### Users

```
user:<id>              → JSON User
idx:user:em:<email>    → <id>   (email index)
idx:user:go:<gid>      → <id>   (google_id index)
```

User IDs are ULIDs (`github.com/oklog/ulid/v2`) stored as strings. `password` and `google_id` use empty string for null (OAuth vs email auth).

### Sessions

```
session:<id>                    → JSON Session
idx:sess:u:<uid>:<sid>          → (empty)   (per-user session index)
```

Sessions are stored as JSON under `session:<id>`. The per-user index `idx:sess:u:<uid>:<sid>` enables listing and deleting all sessions for a user. User IDs in sessions are strings (ULID).

### Password Resets

```
pwreset:<token>                 → JSON PasswordReset
```

Password resets are stored as JSON under `pwreset:<token>`. Expired resets are cleaned up hourly by the background goroutine.

## Transactions

Badger provides read-only (`View`) and read-write (`Update`) transactions. Use `Update` for atomic multi-key writes (e.g. writing a user record plus its index keys together):

```go
// Atomic write of user record + index keys
err := db.Update(func(txn *badger.Txn) error {
    data, _ := json.Marshal(user)
    if err := txn.Set([]byte("user:"+user.ID), data); err != nil {
        return err
    }
    txn.Set([]byte("idx:user:em:"+user.Email), []byte(user.ID))
    return nil
})
```

Use `View` for read-only operations (consistent snapshot):

```go
err := db.View(func(txn *badger.Txn) error {
    item, err := txn.Get([]byte("user:" + id))
    if err != nil {
        return err
    }
    return item.Value(func(val []byte) error {
        return json.Unmarshal(val, &user)
    })
})
```

> Badger transactions are optimistic — long-running write transactions may conflict and need retry. Keep transactions short. Iteration over a key prefix uses `txn.NewIterator` with `Prefix` option.

## Best Practices

### 1. Use the Repository Instead of Direct Badger Access

All database operations should go through the `Repository` wrapper. Avoid calling `db.View`/`db.Update` directly in services:

```go
// Bad: Direct Badger access in service
db.View(func(txn *badger.Txn) error {
    item, _ := txn.Get([]byte("idx:user:em:" + email))
    // ...
})

// Good: Repository method
user, err := s.repository.GetUserByEmail(ctx, email)
```

### 2. Handle badger.ErrKeyNotFound with Domain Errors

```go
// The Repository wrapper already converts badger.ErrKeyNotFound to domain errors:
if errors.Is(err, repositories.ErrUserNotFound) {
    return nil, services.ErrInvalidCredentials
}
```

### 3. Use Update Transactions for Multiple Writes

```go
// Good: Atomic transaction for data integrity
err := db.Update(func(txn *badger.Txn) error {
    data, _ := json.Marshal(user)
    txn.Set([]byte("user:"+user.ID), data)
    txn.Set([]byte("idx:user:em:"+user.Email), []byte(user.ID))
    return nil
})
```

### 4. Maintain Index Keys for Frequently Queried Fields

Index keys are maintained alongside primary records. Always write/delete index keys in the same `Update` transaction as the primary record:

```
user:<id>              → primary record
idx:user:em:<email>    → email index
idx:user:go:<gid>      → google_id index
idx:sess:u:<uid>:<sid> → per-user session index
```

### 5. Keep Transactions Short

Badger uses optimistic concurrency control. Long-running write transactions may conflict and require retry. Keep `Update` transactions short and avoid holding iterators open across slow operations.

### 6. Close Iterators After Use

When iterating over a key prefix, always close the iterator:

```go
opts := badger.DefaultIteratorOptions
opts.Prefix = []byte("user:")
it := txn.NewIterator(opts)
defer it.Close()

for it.Rewind(); it.Valid(); it.Next() {
    // process item
}
```

> Iterators hold resources; failing to close them leaks memory and can block compaction.

## Troubleshooting

### Transaction Conflict

**Problem**: `Transaction Conflict. Please retry`

**Solutions**:

1. Keep write transactions short
2. Retry the operation on conflict
3. Avoid long-running read transactions that overlap with writes
4. Use prefix iterators instead of full scans where possible

### Key Not Found

**Problem**: `Key not found` errors unexpectedly

**Solutions**:

1. Verify the key prefix is correct (e.g. `user:` vs `users:`)
2. Check that index keys are written alongside primary records
3. Ensure the Badger directory (`./data/badger`) is not shared between instances

### Database Open Issues

**Problem**: `unable to open database` / Badger fails to start

**Solutions**:

1. Ensure directory exists: `mkdir -p data/badger`
2. Check permissions: `chmod 755 data`
3. Verify DB_PATH in .env points to a directory, not a file
4. Ensure no other process holds a lock on the directory

## Next Steps

- [Authentication Guide](authentication.md) - User authentication and sessions
- [Architecture Guide](architecture.md) - Badger KV and Repository pattern in context
- [Deployment Guide](../deployment/production.md) - Production database setup
