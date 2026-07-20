# Laju Go Documentation

Welcome to the Laju Go documentation. This folder contains guides for building applications with Laju Go — a high-performance SaaS boilerplate with Go Fiber, Svelte 5, Inertia.js, and Badger KV.

## 📚 Documentation Structure

### Guide

| Document | Description |
|----------|-------------|
| [Architecture](guide/architecture.md) | Layered architecture, design patterns, and best practices |
| [Routing](guide/routing.md) | Route definitions, middleware, and request handling |
| [Handlers](guide/handlers.md) | Building HTTP handlers, request/response handling |
| [Database](guide/database.md) | Badger KV setup, key prefixes, and hand-written repositories |
| [Templ](guide/templ.md) | Type-safe HTML components via templ |
| [Frontend](guide/frontend.md) | Svelte 5 components and Inertia.js integration |
| [File Upload](guide/file-upload.md) | Avatar upload, validation, storage |
| [Email](guide/email.md) | SMTP password reset email |
| [Validation](guide/validation.md) | Input validation techniques |
| [Forms](guide/forms.md) | Form handling with Inertia |
| [Styling](guide/styling.md) | Tailwind CSS styling |
| [Storage](guide/storage.md) | File storage management |
| [Data Protection](guide/data-protection.md) | Badger KV data protection and recovery |
| [Testing](guide/testing.md) | Testing strategies |

### Deployment

| Document | Description |
|----------|-------------|
| [Development Workflow](deployment/development.md) | Hot reload, scripts, and development best practices |
| [Production Deployment](deployment/production.md) | Ubuntu/Debian deployment with systemd and Nginx |
| [Badger Configuration](guide/database.md) | Badger KV setup, key prefixes, and tuning |

## 🚀 Quick Start

```bash
# Clone the repository
git clone https://github.com/maulanashalihin/laju-go.git
cd laju-go

# Install dependencies
go mod download && npm install

# Configure environment
cp .env.example .env

# Start development
npm run dev:all
```

Visit `http://localhost:8080` to see your application.

## 🎯 Common Tasks

### Development

| Task | Guide |
|------|-------|
| Set up development environment | [Development Workflow](deployment/development.md) |
| Configure environment variables | `.env.example` → `.env` |
| Run development servers | `npm run dev:all` |
| Create new route & handler | [Routing](guide/routing.md) + [Handlers](guide/handlers.md) |
| Add database model | [Database](guide/database.md) |

### Deployment

| Task | Guide |
|------|-------|
| Build for production | [Production Deployment](deployment/production.md) |
| Optimize Badger KV | [Badger Configuration](guide/database.md) |

## 🔧 Resources

### External Links

- [Go Fiber Documentation](https://docs.gofiber.io/)
- [Svelte Documentation](https://svelte.dev/docs)
- [Inertia.js Documentation](https://inertiajs.com/)
- [Tailwind CSS Documentation](https://tailwindcss.com/docs)
- [Badger KV Documentation](https://dgraph.io/docs/badger/)
- [ULID — Sortable Unique IDs](https://github.com/oklog/ulid)

---

**Last Updated**: July 2026
