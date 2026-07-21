---
type: concept
status: stable
---

# Three-Tier Architecture (Handler → Service → Repository)

Laju Go enforces a strict layered architecture: **Handler → Service → Repository → Badger KV**, with `app/models/` as the shared domain contract that every layer agrees on.

## The Rule

Handlers must **never** call repositories directly. All database access must go through services. Repositories are the **only** layer allowed to touch Badger. This is the most important rule in the project.

```
routes/web.go → app/handlers/ → app/services/ → app/repositories/ → Badger KV
                  ↕ app/models/        ↕            ↕
                  (domain contract)  Cache      (persistence shape)
```

## Layer Responsibilities

- **Models** (`app/models/`): Shared domain contract — entity structs (`User`, `Session`), DTOs (`RegisterRequest`, `UserResponse`), enums (`UserRole`). Zero dependency, zero logic. Tag `json:"-"` on sensitive fields (`Password`, `GoogleID`) is a security invariant. Repositories own their own persistence shape (`userRecord`, `repositories.Session`) and convert to/from `models.*`.
- **Handlers**: Parse requests into `models.*DTO`, call services, return responses. Zero business logic. One handler file per feature module — never bloat a single file with unrelated routes.
- **Services**: Business logic, auth flows, external API integration. Call `s.querier.*` or `s.cache.*` for reads. Return `models.*` to handlers.
- **Repositories**: **The only layer** that touches Badger (`*badger.Txn`, `txn.Get/Set/Delete`). Hand-written (not generated). Key-prefix indexing (`user:`, `idx:user:em:`, `session:`, etc.) is the "schema". Convert `models.*` ↔ persistence shape via `toModel()`/`fromModel()`.
- **Cache** (`app/cache/`): In-memory session cache (`sync.RWMutex` + map). Called via services only.

## Why `app/models/` Exists

Without a shared domain package, each layer would define its own `User` struct with different JSON tags, causing drift and forcing converter boilerplate at every boundary. `app/models/` is the **anti-corruption layer**:

- **Single source of truth** — all layers agree on one `User` definition.
- **Security boundary** — `json:"-"` on `Password`/`GoogleID` guarantees no leak to API response, regardless of which layer serializes.
- **DTO separation** — `RegisterRequest`, `UserResponse`, etc. are distinct from the entity, so handlers can `c.BodyParser(&req)` without worrying about which fields are user-settable.
- **Storage independence** — repository's `userRecord` can change (add `IPHash`, `DeviceFingerprint`) without breaking the domain contract. Badger could be swapped for Postgres/Redis by only rewriting `app/repositories/`.

## Exception

Test files (`*_test.go`) may call repositories directly for test data setup. This is the only sanctioned layer-skipping.

## File Growth Convention

`app/models/` grows **one file per domain aggregate** (not per layer):

```
app/models/
├── user.go           # User entity + UserRole enum + UserResponse
├── session.go        # Session entity + SessionData
├── dto.go            # Auth/profile DTOs (Register, Login, UpdateProfile)
├── order.go          # NEW: Order entity + OrderStatus enum
├── order_dto.go      # NEW: CreateOrderRequest, OrderResponse (split when aggregate grows)
└── product.go        # NEW: Product entity + Category enum
```

Rules:
- ✅ One file per aggregate. DTO may live in same file or split (`*_dto.go`) when aggregate grows.
- ✅ `json:"-"` on sensitive fields — security invariant, centralised here.
- ❌ No logic in `models/`. Only structs + converter methods (`ToResponse()`, etc.).
- ❌ No imports from `repositories`, `services`, or `badger`.
- ❌ Don't define DTOs in handlers/services — put them in `models/` for typed cross-layer contracts.

## Source

- Captured from [[sources/SRC-2026-07-06-001]] (original three-tier rule).
- Updated 2026-07-21: Badger migration, `app/models/` documentation, persistence-shape separation.
