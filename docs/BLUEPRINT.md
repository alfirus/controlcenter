# Control Center — Blueprint

## 1. Vision
Programmer-focused control center: Mattermost-grade communication, GitHub-linked Projects/Tasks, shared+personal Calendar with Google sync, inviteable AI personas (e.g. Sofia via local Hermes Agent), and user management. Pure black & white, minimalist, native clients everywhere.

## 2. Folders (6 + backend)
```
controlcenter/
  backend/   Go + Supabase, Docker
  macos/     Swift/SwiftUI (native macOS)
  windows/   WinUI 3 C# (native Windows)
  ubuntu/    GTK4/libadwaita (native Ubuntu, .deb + Flatpak covers Arch/Omarchy)
  android/   Java (native Android)
  ios/       Swift/SwiftUI (native iOS)
  docs/
  design-tokens.json
  docker-compose.yml
```
Omarchy intentionally omitted (Flatpak from ubuntu covers it). AGPL-3.0. No hosted Supabase dependency; `supabase/cli` runs locally via Docker.

## 3. Backend (Go + Supabase Realtime, Docker)
- Go 1.22, `chi`, `pgx/v5`, `sqlc`, `golang-migrate`, `oapi-codegen`, `go-github`, `google calendar API`.
- `docker-compose.yml` runs `supabase` (db/auth/storage/realtime) + `backend`. `supabase/config.toml` committed.
- Realtime: Supabase Realtime `postgres_changes` on `messages`, `tasks`, `events`, `channel_members`. No custom WS hub.
- OpenAPI 3.1 at `backend/api/openapi.yaml` — contract for all clients. `make gen` regenerates server + client stubs.

### 3.1 Schema (RLS everywhere)
- `profiles(id FK auth.users)`, `workspaces`, `workspace_members(role: admin|member|guest)`
- `channels(kind: open|private|dm|gm)`, `channel_members`, `channel_permissions`
- `messages(channel_id, user_id|agent_id, body, root_id)` threads via `root_id`, FTS `tsvector`, `attachments` in Storage bucket `attachments`
- `webhooks`, `slash_commands`, `bots`
- `projects`, `tasks(github_issue_id, github_repo)`, `task_comments`
- `github_installations`, `github_repo_links(workspace_id, installation_id, repo)`
- `calendars(kind: team|personal, google_calendar_id)`, `events(google_event_id, etag)`
- `agents(hermes_endpoint, persona, display_name, capabilities)`, `agent_memberships(role=agent, workspace/channel/project scope)`
- `invites`, `oauth_states`

### 3.2 Services & Routes (`/v1/...`)
- `auth`: Supabase Auth (email/pass + GitHub OAuth), invite, RBAC middleware (`admin/member/guest/agent`).
- `comm`: workspaces/channels/messages/threads/DMs/GMs, `GET /search`, file upload, webhooks/bots/slash, presence/typing via Realtime.
- `projects/tasks`: CRUD + `POST /projects/:id/link-github` (multi-org GitHub App install `GET /github/install`, `POST /webhooks/github` HMAC, reconciliation every 5m, idempotent on `github_issue_id`).
- `calendar`: `GET/POST /calendars|/events`, Google OAuth `GET /oauth/google/*`, sync via `watch` + `syncToken`/`etag`, team vs personal visibility.
- `agent-gateway`: `POST /agents` (register `hermes_endpoint+persona` e.g. Sofia), `POST /agents/:id/invite` scoped to workspace/channel/project/calendar, proxy `POST /agents/:id/message` → Hermes, inbound `POST /agents/:id/events` → writes as agent identity, SSE streaming proxied to Realtime.
- `user-mgmt`: `GET/PATCH /workspaces/:id/members`.

### 3.3 GitHub (bi-directional, multi-org)
GitHub App per org (`issues:write, pull_requests:read, metadata:read`). Link repos→projects. Outbound `PATCH /repos/:repo/issues/:n` on task change; inbound webhook upserts task. Dedup by `updated_at`.

### 3.4 Calendar (Google sync, shared+personal)
OAuth calendar scope, `calendarList.list` + `events.list/watch`, renewal cron, last-write-wins on `etag`, ICS export.

## 4. Clients (native, thin, token-driven)
Shared IA: left sidebar Workspaces→Channels/DMs/GMs + Projects | center messages/tasks/calendar | right thread/agent panel. All subscribe to Realtime filtered by `channel_id`/`workspace_id`.
- macOS: SwiftUI+AppKit, menubar extra.
- Windows: WinUI 3 MSIX.
- Ubuntu: GTK4/libadwaita, `.deb` + Flatpak.
- Android: Java, Material 3 monochrome, Retrofit, WorkManager.
- iOS: SwiftUI, SPM `DesignTokens` shared with macOS.

## 5. AI Personas (Hermes)
Hermes owns persistent identity/persona lifecycle (Sofia etc.). Control Center stores `agent_id = hermes_endpoint + persona`. Agents participate in comm/tasks/calendar with `agent` role scoped via `agent_memberships`. Hermes → Control Center via inbound events; Control Center → Hermes via proxy.

## 6. Phases
0 Scaffolding (Go skeleton, migrations, OpenAPI, Supabase local, tokens, stub clients, CI)
1 Auth + RBAC
2 Communication MVP (Mattermost parity: channels/threads/DMs/GMs/search/files/webhooks/slash/permissions)
3 Projects/Tasks + GitHub bi-directional multi-org
4 Calendar + Google sync
5 Agent Gateway + Hermes personas
6 Native clients (parallel from Phase 2, packaging/signing)

## 7. API Conventions
- Base: `/v1`, JSON, `application/json`. Envelope: `{ data, error: { code, message } }`.
- Pagination: `?cursor=<opaque>&limit=50` (cursor = `created_at,id`), `Link` header + `next_cursor` in response. Sorting `?sort=-created_at`.
- Errors: `400` validation, `401` unauth, `403` RLS/role, `404`, `409` conflict (etag), `429` rate-limited, `500`.
- Versioning: URL version `/v1`; breaking changes bump minor in OpenAPI; `X-API-Version` header echoed.
- Idempotency: `Idempotency-Key` header on POST/PATCH for tasks/events/messages.

## 8. Auth, RLS & Security
- Supabase Auth: email/pass + GitHub OAuth. JWT (RS256) with `role`, `workspace_roles` claims. Refresh via Supabase.
- RLS: enabled on all tables. Example: `messages` policy `USING (channel_id IN (SELECT channel_id FROM channel_members WHERE user_id = auth.uid()))`. `agent` role scoped via `agent_memberships`.
- RBAC matrix: `admin` (invite, role change, delete workspace), `member` (create channels/tasks), `guest` (read only explicitly added channels), `agent` (only subscribed channels/projects).
- Invites: `invites(token, workspace_id, role, expires_at)` → email link → `POST /v1/invites/:token/accept`.
- Secrets: `.env` never committed, `backend/.env.example` documents keys; Docker injects via env. GitHub App private key mounted as secret volume.
- Hardening: CORS allowlist, CSP on any web preview, rate limit 100 req/min/IP (Redis or in-memory token bucket), JWT expiry 15m.

## 9. Realtime (Supabase postgres_changes)
- Enabled on `messages`, `tasks`, `events`, `channel_members`, `reactions`. Clients subscribe with `filter: workspace_id=eq.<id>` + RLS ensures only member rows replicate.
- Presence/typing: Supabase Realtime `presence` channel per `channel_id` (ephemeral, not DB).
- Ordering: DB `created_at` is source of truth; client reconciles optimistic insert by `id`.

## 10. Search, Storage, Offline
- Search: Postgres `tsvector` on `messages.body` (GIN), ranking `ts_rank`, `GET /v1/search?q=&channel_id=&from=&before=`. Future: `pg_trgm` for fuzzy.
- Storage: bucket `attachments` (limit 25MB/file, 100MB/message batch). RLS by channel membership; signed URLs 1h.
- Offline: clients queue mutations with `client_id` + `Idempotency-Key`, replay on reconnect; show `◌ unsent` state.

## 11. Observability & Ops
- Logging: structured JSON (slog), `request_id` propagation. Tracing optional OTel.
- Health: `GET /healthz` (db + realtime + storage ping), `GET /readyz`.
- Metrics: `GET /metrics` (Prometheus) — request latency, realtime fanout, webhook lag.
- Backups: Supabase PITR; migrations via `golang-migrate` (`backend/migrations/*.sql`), forward-only.

## 12. Testing & CI
- Backend: `go test` unit + `testcontainers` integration against real Postgres + Supabase local. Contract tests against `openapi.yaml`.
- Clients: per-platform unit tests (XCTest/JUnit/pytest for GTK). E2E: API smoke in CI.
- CI (GitHub Actions): `lint` (golangci-lint, swiftlint, ktlint), `gen` drift check, `test`, `build` (Docker, .deb, Flatpak, MSIX, .apk/.ipa). Branching: `main` protected, PR required.

## 13. Deployment Topology (Docker)
```
docker-compose.yml: db, auth, storage, realtime, backend, (optional) github-worker, calendar-worker
backend: Go binary, `DATABASE_URL`, `SUPABASE_JWT_SECRET`, `GITHUB_APP_ID/PRIVATE_KEY/WEBHOOK_SECRET`, `GOOGLE_CLIENT_ID/SECRET`
```
Workers share DB; horizontally scalable (stateless). Secrets via env file.

## 14. Data Retention
Soft delete (`deleted_at`) for messages/tasks; hard purge after 30d by cron. Audit log table `audit_log` for admin actions.

## 15. Risks
Realtime filtering/pagination + k6 load test; webhook idempotency + dead-letter table `webhook_dead_letters`; Google `watch` renewal cron; keep clients thin; RLS policy tests in CI.
