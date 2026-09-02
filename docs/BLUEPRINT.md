# Control Center — Blueprint

## 1. Vision
Programmer-focused control center: Mattermost-grade communication, GitHub-linked Projects/Tasks, shared+personal Calendar with Google sync, Termius-grade Terminal (host/SSH management + multiplexed sessions), Zed-grade IDE (collaborative code editing, LSP, fast buffer), inviteable AI personas (e.g. Sofia via local Hermes Agent), and user management. Pure black & white, minimalist, native clients everywhere.

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
- `hosts(id, workspace_id, label, hostname, port, username, auth_kind, vault_ref, tags, jump_host_id)`, `host_groups`, `ssh_keys(vault_ref, fingerprint)`, `terminal_sessions(id, host_id, user_id|agent_id, status, started_at, ended_at, recording_ref)`, `terminal_recordings(session_id, chunk_seq, data)`
- `ide_workspaces(id, workspace_id, project_id, root_path)`, `ide_buffers(id, ide_workspace_id, path, content_hash)`, `ide_collab_sessions(id, buffer_id, crdt_state)`, `lsp_servers(id, language, command, config)`
- `invites`, `oauth_states`

### 3.2 Services & Routes (`/v1/...`)
- `auth`: Supabase Auth (email/pass + GitHub OAuth), invite, RBAC middleware (`admin/member/guest/agent`).
- `comm`: workspaces/channels/messages/threads/DMs/GMs, `GET /search`, file upload, webhooks/bots/slash, presence/typing via Realtime.
- `projects/tasks`: CRUD + `POST /projects/:id/link-github` (multi-org GitHub App install `GET /github/install`, `POST /webhooks/github` HMAC, reconciliation every 5m, idempotent on `github_issue_id`).
- `calendar`: `GET/POST /calendars|/events`, Google OAuth `GET /oauth/google/*`, sync via `watch` + `syncToken`/`etag`, team vs personal visibility.
- `terminal` (Termius-like): host inventory + vault-backed SSH auth, `GET/POST /hosts`, `GET/POST /host-groups`, `POST /ssh-keys` (vault ref only, never raw private key in DB), `POST /terminal/sessions` (create multiplexed PTY via backend gateway), `GET /terminal/sessions/:id/stream` (WebSocket/SSE PTY stream, resize, keepalive), `GET /terminal/sessions/:id/recording`, SFTP `POST /terminal/:id/sftp/*`. Gateway isolates secrets; agents can be granted scoped host access via `agent_memberships`.
- `ide` (Zed-like): `GET/POST /ide/workspaces`, `GET/PUT /ide/buffers` (content + hash, chunked for large files), `POST /ide/collab/sessions` + `WS /ide/collab/:id` (CRDT sync, presence cursors), LSP proxy `POST /ide/lsp/:language` (backend spawns `lsp_servers` per language, streams diagnostics/completions), file tree `GET /ide/workspaces/:id/tree`, Git status via linked `github_repo_links`. Offline: local buffer cache, sync on reconnect.
- `agent-gateway`: `POST /agents` (register `hermes_endpoint+persona` e.g. Sofia), `POST /agents/:id/invite` scoped to workspace/channel/project/calendar/host/ide, proxy `POST /agents/:id/message` → Hermes, inbound `POST /agents/:id/events` → writes as agent identity (including terminal commands and ide edits), SSE streaming proxied to Realtime.
- `user-mgmt`: `GET/PATCH /workspaces/:id/members`.

### 3.3 GitHub (bi-directional, multi-org)
GitHub App per org (`issues:write, pull_requests:read, metadata:read`). Link repos→projects. Outbound `PATCH /repos/:repo/issues/:n` on task change; inbound webhook upserts task. Dedup by `updated_at`.

### 3.4 Calendar (Google sync, shared+personal)
OAuth calendar scope, `calendarList.list` + `events.list/watch`, renewal cron, last-write-wins on `etag`, ICS export.

### 3.5 Terminal (Termius-like)
Host inventory per workspace (Termius parity: groups/tags, jump hosts, vault-backed keys/passwords, agent forwarding toggle). Backend **gateway** holds PTY: client opens `POST /terminal/sessions` → backend dials SSH (Go `golang.org/x/crypto/ssh`, `creack/pty`-style multiplexing), streams PTY over `WS /terminal/sessions/:id/stream` (binary + JSON control frames for resize). Features: multi-tab sessions, split panes (client-side), snippet library (`hosts` tags → snippets), SFTP browser, port forwarding config, session recording to `terminal_recordings` + Storage, shareable read-only session link. Secrets: private keys/passwords stored in vault (Supabase Vault or external), only `vault_ref` in DB; gateway fetches at dial time, never logs. RBAC: `admin` manages hosts/keys, `member` connects to allowed groups, `guest` no access, `agent` only if explicitly invited per host/group.

### 3.6 IDE (Zed-like)
Zed parity: fast native buffer (rope + tree-sitter on client), collaborative CRDT (`ide_collab_sessions`, e.g. `y-crdt`/`automerge` synced via `WS /ide/collab/:id`), live cursors/selections via Realtime presence, LSP per language (`lsp_servers`: gopls, rust-analyzer, typescript-language-server, etc. — backend spawns, proxies JSON-RPC over WS), file tree with Git status (reuses `github_repo_links`), command palette, minimap off (B&W minimal), vim mode toggle. Backend stores buffer snapshots (`ide_buffers` + `content_hash`) and streams patches; conflict: CRDT wins, with `If-Match: content_hash` guard on save. Large files: chunked `PUT` + `Range`. Search: ripgrep-style `GET /ide/workspaces/:id/search?q=`. Agents: can read/edit buffers and run LSP actions within granted `ide_workspace_id` scope.

### 3.7 Terminal + IDE Integration
Terminal and IDE share `ide_workspaces` ↔ `hosts` linkage (workspace root may be remote host path). `Open in Terminal` from IDE file context, `Open in IDE` from terminal cwd. Agents participate in both: `agent_memberships` scope covers `host_id` and `ide_workspace_id`; agent terminal commands and buffer edits are audited in `audit_log` + `terminal_recordings`.

## 4. Clients (native, thin, token-driven)
Shared IA: left sidebar Workspaces→Channels/DMs/GMs + Projects | center messages/tasks/calendar/terminal/ide | right thread/agent panel. All subscribe to Realtime filtered by `channel_id`/`workspace_id`. Terminal and IDE are top-level nav items (alongside Chat/Tasks/Calendar).
- macOS: SwiftUI+AppKit, menubar extra. Terminal via `swift-pty`/`Process` + xterm.js-equivalent Swift view; IDE via native `NSTextView` + tree-sitter (B&W theme).
- Windows: WinUI 3 MSIX. Terminal via ConPTY + Windows Terminal control; IDE via WinUI editor + LSP client.
- Ubuntu: GTK4/libadwaita, `.deb` + Flatpak. Terminal via `libvte`; IDE via `GtkSourceView` + LSP.
- Android: Java, Material 3 monochrome, Retrofit, WorkManager. Terminal via `ssh` + `terminal-view`; IDE read-only + review (full edit deferred).
- iOS: SwiftUI, SPM `DesignTokens` shared with macOS. Terminal via `Blink`-style PTY view (read + limited input); IDE read-only + review (full edit deferred).

## 5. AI Personas (Hermes)
Hermes owns persistent identity/persona lifecycle (Sofia etc.). Control Center stores `agent_id = hermes_endpoint + persona`. Agents participate in comm/tasks/calendar with `agent` role scoped via `agent_memberships`. Hermes → Control Center via inbound events; Control Center → Hermes via proxy.

## 6. Phases
0 Scaffolding (Go skeleton, migrations, OpenAPI, Supabase local, tokens, stub clients, CI)
1 Auth + RBAC
2 Communication MVP (Mattermost parity: channels/threads/DMs/GMs/search/files/webhooks/slash/permissions)
3 Projects/Tasks + GitHub bi-directional multi-org
4 Calendar + Google sync
5 Agent Gateway + Hermes personas
6 Terminal (Termius parity: hosts/groups/SSH gateway/PTY WS/SFTP/recording)
7 IDE (Zed parity: buffers/CRDT collab/LSP proxy/file tree)
8 Native clients (parallel from Phase 2, packaging/signing) — terminal PTY and IDE editor per platform

## 7. API Conventions
- Base: `/v1`, JSON, `application/json`. Envelope: `{ data, error: { code, message } }`.
- Pagination: `?cursor=<opaque>&limit=50` (cursor = `created_at,id`), `Link` header + `next_cursor` in response. Sorting `?sort=-created_at`.
- Errors: `400` validation, `401` unauth, `403` RLS/role, `404`, `409` conflict (etag), `429` rate-limited, `500`.
- Versioning: URL version `/v1`; breaking changes bump minor in OpenAPI; `X-API-Version` header echoed.
- Idempotency: `Idempotency-Key` header on POST/PATCH for tasks/events/messages.

## 8. Auth, RLS & Security
- Supabase Auth: email/pass + GitHub OAuth. JWT (RS256) with `role`, `workspace_roles` claims. Refresh via Supabase.
- RLS: enabled on all tables. Example: `messages` policy `USING (channel_id IN (SELECT channel_id FROM channel_members WHERE user_id = auth.uid()))`. `agent` role scoped via `agent_memberships`. Hosts/buffers: `USING (workspace_id IN (SELECT workspace_id FROM workspace_members WHERE user_id = auth.uid()))` plus group/ide membership checks.
- RBAC matrix: `admin` (invite, role change, delete workspace, manage hosts/LSP), `member` (create channels/tasks, connect to allowed host groups, edit buffers), `guest` (read only explicitly added channels), `agent` (only subscribed channels/projects/hosts/ide).
- Invites: `invites(token, workspace_id, role, expires_at)` → email link → `POST /v1/invites/:token/accept`.
- Secrets: `.env` never committed, `backend/.env.example` documents keys; Docker injects via env. GitHub App private key mounted as secret volume.
- Hardening: CORS allowlist, CSP on any web preview, rate limit 100 req/min/IP (Redis or in-memory token bucket), JWT expiry 15m. SSH: no raw keys in DB (vault only), audit all terminal commands + ide edits, PTY stream authenticated via JWT, idle timeout 15m.

## 9. Realtime (Supabase postgres_changes)
- Enabled on `messages`, `tasks`, `events`, `channel_members`, `reactions`. Clients subscribe with `filter: workspace_id=eq.<id>` + RLS ensures only member rows replicate.
- Presence/typing: Supabase Realtime `presence` channel per `channel_id` (ephemeral, not DB).
- Ordering: DB `created_at` is source of truth; client reconciles optimistic insert by `id`.

## 10. Search, Storage, Offline
- Search: Postgres `tsvector` on `messages.body` (GIN), ranking `ts_rank`, `GET /v1/search?q=&channel_id=&from=&before=`; IDE `GET /ide/workspaces/:id/search?q=` (ripgrep-style). Future: `pg_trgm` for fuzzy.
- Storage: buckets `attachments` (25MB/file) + `terminal-recordings` + `ide-snapshots`. RLS by workspace/channel membership; signed URLs 1h.
- Offline: clients queue mutations with `client_id` + `Idempotency-Key`, replay on reconnect; show `◌ unsent` state. Terminal requires live WS (no offline); IDE buffers cache locally, CRDT sync on reconnect.

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
Soft delete (`deleted_at`) for messages/tasks/hosts/buffers; hard purge after 30d by cron. Terminal recordings retained per workspace policy (default 30d). Audit log table `audit_log` for admin actions + terminal commands + ide buffer saves.

## 15. Risks
Realtime filtering/pagination + k6 load test; webhook idempotency + dead-letter table `webhook_dead_letters`; Google `watch` renewal cron; PTY backpressure + idle timeout + recording cost; CRDT convergence + LSP spawn cost; keep clients thin; RLS policy tests in CI.
