# Koffan – Copilot Instructions

Koffan is a lightweight shopping-list PWA for couples and families with real-time sync.

## Tech Stack

- **Backend:** Go 1.25+ (Go 1.26.6 toolchain), Fiber v2, SQLite (WAL mode, CGO required)
- **Frontend:** HTMX + Alpine.js + Tailwind CSS (prebuilt `static/app.css`, rebuild with `npm run build:css`), SortableJS
- **Templates:** Go `html/template` with partials in `templates/partials/`
- **Real-time:** WebSocket via Fiber, broadcast to the clients of the workspace that changed
- **Offline:** Service Worker + IndexedDB queue, optimistic UI updates
- **i18n:** Embedded JSON files in `i18n/`, 18 languages, `t(key)` on client

## Project Structure

```
main.go              # Entry point: init → DB → templates → routes → serve
api/                 # REST API v1 (enabled via API_TOKEN env var)
db/                  # SQLite schema, migrations, queries (db.go + queries.go)
handlers/            # Route handlers (auth, lists, items, sections, ws, workspaces, etc.); routes behind auth in routes.go
i18n/                # Locale JSON files + Go loader (locales.go)
static/              # JS assets (app.js, offline-storage.js, sw.js)
templates/           # Go HTML templates + partials/
```

## Build & Run

```bash
# Local dev (requires CGO for SQLite)
CGO_ENABLED=1 go build -o shopping-list .
APP_PASSWORD=test ./shopping-list

# Docker
docker compose -f docker-compose.local.yaml up --build

# Production build (version injection)
VERSION=$(cat VERSION)
CGO_ENABLED=1 go build -ldflags "-X shopping-list/handlers.AppVersion=$VERSION" -o shopping-list .
```

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `PORT` | `3000` | HTTP listen port |
| `APP_PASSWORD` | `shopping123` | Login password |
| `SESSION_DURATION` | `168h` (7 days) | Login session lifetime; duration syntax, e.g. `30m`, `12h`, `1d`, or `7d` |
| `DB_PATH` | `./shopping.db` | SQLite file path |
| `APP_ENV` | `dev` | `dev` reloads templates on each request |
| `API_TOKEN` | *(unset)* | Set to enable REST API at `/api/v1/*` |
| `DISABLE_AUTH` | `false` | Skip auth (dev only) |
| `MULTI_USER` | `false` | Exactly `true` enables accounts and workspaces (see below) |
| `ADMIN_USER` | `admin` | With `MULTI_USER`: first administrator, created with `APP_PASSWORD` on first start |
| `API_WORKSPACE_ID` | `1` | With `MULTI_USER`: workspace the REST API token works on |
| `DEFAULT_LANG` | `en` | Fallback UI language |

## Architecture Decisions

### Server returns HTML, not JSON
All HTMX endpoints return rendered HTML partials. The REST API (`/api/v1/*`) is the public JSON interface and is gated behind `API_TOKEN`; the web UI also uses a few internal JSON endpoints (`/api/offline/*`, `/api/data`, `/api/history`).

### Single-password auth, or accounts with `MULTI_USER=true`
By default one shared password for all users, and everything lives in workspace 1. With `MULTI_USER=true` there are accounts and workspaces: lists, templates and item history belong to a workspace, queries take a `workspaceID`, and every route in `handlers/routes.go` is guarded by the workspace of the session (`handlers/scope.go`). Sessions stored in SQLite (64-byte hex token, 7-day expiry, HTTPOnly cookie). Rate limiting per IP on `POST /login`.

### Database migrations run on startup
`db.Init()` creates tables and applies sequential migrations automatically. Never remove or reorder migrations in `db/db.go`.

### WebSocket broadcast pattern
Data mutations in handlers call `BroadcastUpdate(c, eventType, data)` to push changes to the connected clients of the request's workspace (`WorkspaceID(c)`). Each socket is bound to its session, user and workspace when it connects; code that revokes access (sign-out, removing a member, deleting an account or workspace, a new password) closes the affected sockets with `closeWebSockets`. Event types follow the pattern `entity_action` (e.g., `item_created`, `section_deleted`).

### Offline-first with queue
`offline-storage.js` queues actions in IndexedDB when offline; `processOfflineQueue()` replays them on reconnect. UI updates are optimistic.

### i18n: client-side with server injection
All translations are injected into `window.translations` in `layout.html`. Use `t('section.key')` in Alpine.js expressions. Add new keys to all JSON files in `i18n/`.

## Conventions

- **Handlers** return Fiber responses (`c.Render()`, `c.JSON()`, `c.Redirect()`). One handler per logical action. Function names follow `EntityActionHandler` (e.g., `CreateItemHandler`, `ToggleItemHandler`).
- **DB queries** live in `db/queries.go`. Use transactions (`DB.Begin()`) for multi-step writes. Always `defer tx.Rollback()` before operations, only `tx.Commit()` on full success. Use `COALESCE()` for nullable columns. Use `COLLATE NOCASE` for case-insensitive name matching.
- **Templates** use `{{template "partials/name" .}}` for composition. Pass data via `fiber.Map{}`. Template functions: `dict`, `add`/`sub`/`mul`/`div`, `gt`/`lt`/`eq`/`ne`, `T` (i18n), `toJSON`, `asset`, `initial`.
- **HTMX responses:** Return rendered HTML partials (never JSON). Use `outerHTML` swap strategy. Use `HX-Trigger-After-Settle` header for client-side events (e.g., `statsRefresh`). For HTMX 401, return `HX-Redirect: /login` header.
- **Alpine components:** Main shopping list logic in `shoppingList()` function in `static/app.js`. Do NOT call `Alpine.initTree()` manually — Alpine auto-initializes via MutationObserver.
- **CSS:** Tailwind utility classes; the few custom components live in `tailwind/input.css`. Extended palette: primary = pink, uncertain = amber. After changing classes run `npm run build:css`; `test/tailwind-css.test.js` detects a stale `static/app.css`.
- **Validation limits:** List/Section name ≤ 100 chars, Item name ≤ 200 chars, Description ≤ 500 chars.
- **Timestamps:** Stored as Unix seconds (integer) in SQLite. Use `strftime('%s', 'now')` in SQL.
- **Struct tags:** All domain types use `json:"field_name"` tags for REST API serialization.
- **SQLite resilience:** Use `retryOnBusy()` wrapper for operations prone to `SQLITE_BUSY`. WAL mode + `_busy_timeout=5000` are set on connection.
- **Item deduplication:** On create, check for existing item by name (case-insensitive) in the section. Reactivate if completed instead of creating a duplicate.
- **Embedded assets:** Templates and static files are embedded via `//go:embed` for production builds.
- **Tests:** run `go test ./...` and `npm test` (Node test runner for the browser scripts). Workspace isolation, auth and offline sync have regression tests in `handlers/` and `db/`.

## Common Tasks

### Adding a new i18n key
1. Add key to all JSON files in `i18n/` (18 files)
2. Use `t('section.key')` in templates or `i18n.Get(lang, "section.key")` in Go
3. Run `go test ./i18n` to check that every file has the same keys and placeholders

### Adding a new DB migration
Append to the migrations slice in `db/db.go`. Migrations are idempotent ALTER TABLE / CREATE TABLE IF NOT EXISTS statements.

### Adding a REST API endpoint
1. Add route in `api/api.go`; guard `:id` parameters with `handlers.Require*` (they answer 404 for another workspace's data)
2. Implement handler in the appropriate `api/*.go` file; scope queries with `handlers.WorkspaceID(c)` and resolve IDs from the body with `ownedList` / `ownedSection` (`api/scope.go`)
3. Authentication is automatic via `TokenAuthMiddleware`, and `UseAPIWorkspace` pins the request to `API_WORKSPACE_ID`

Web routes follow the same rule: register them in `handlers/routes.go` with the matching `Require*` guard.

### Adding a WebSocket event
1. Trigger `BroadcastUpdate(c, "entity_action", data)` in the handler
2. Handle the event type in `app.js` WebSocket message listener

## Pitfalls

- **CGO is required.** `CGO_ENABLED=1` must be set for SQLite builds. On macOS this needs Xcode CLI tools; on Alpine it needs `gcc musl-dev`.
- **Template changes** need server restart in production (`APP_ENV=production`). In dev mode templates reload automatically. Note: the check is `APP_ENV != "production"` — reload is the default.
- **The go module is named `shopping-list`**, not `koffan`. Import paths use `shopping-list/...`.
- **Export/import** has a 5 MB file size limit. Validation is case-insensitive for list name conflicts.
- **CSRF:** with `MULTI_USER=true`, state-changing requests, WebSocket handshakes and logins from other origins are refused in `handlers/auth.go` (`Sec-Fetch-Site`, falling back to comparing `Origin` with `Host` / `X-Forwarded-Host`). In shared-password mode only SameSite=Lax cookies and the `csrf-token` of *Clear database* protect writes.
- **Offline race conditions:** Local action tracking via timestamps suppresses WebSocket updates during pending HTMX requests to avoid double-updating the UI. Be careful when modifying the offline queue logic in `offline-storage.js`.
- **Sort modes:** Sections support `manual`, `alphabetical`, `alphabetical_desc` sorting (stored in DB). Reordering operations only apply to active items (`completed = FALSE`).
- **Service Worker:** Static assets use `Cache-Control: no-cache`. The SW handles offline fallback — changes to `sw.js` affect all users on next visit.
