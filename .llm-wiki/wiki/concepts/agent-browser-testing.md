---
type: concept
status: stub
---

# Agent Browser Testing

Laju Go uses pi's built-in `agent_browser` for E2E testing — without Playwright/Cypress.

## Skip Login — Inject Session Directly

To test protected pages without manual login:

1. Create a session in Badger
2. Set the cookie via `agent_browser eval`
3. Open the protected page

Full details: [[concept-e2e-browser-testing]]

## Source

Migrated from AGENTS.md.
