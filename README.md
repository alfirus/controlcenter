# Control Center

> Programmer-focused control center — Mattermost-grade communication, GitHub-synced Projects/Tasks, Google Calendar (team + personal), Termius-grade Terminal (SSH/host management + PTY), Zed-grade IDE (collab CRDT + LSP), inviteable AI personas (Hermes/Sofia), and RBAC user management. Pure black & white, minimalist, native everywhere. AGPL-3.0.

## Structure

```
controlcenter/
  backend/            Go + Supabase (Docker) — API, Realtime, workers
  macos/              Swift/SwiftUI — native macOS
  windows/            WinUI 3 (C#) — native Windows
  ubuntu/             GTK4/libadwaita — native Ubuntu (.deb + Flatpak)
  android/            Java — native Android
  ios/                Swift/SwiftUI — native iOS
  docs/
    BLUEPRINT.md      Architecture, schema, services, phases
    UI-UX.md          B&W design system, tokens, keyboard, states
  design-tokens.json  Single source of truth for theme
  docker-compose.yml  Supabase local + backend
  Makefile
```

Omarchy is covered by the Ubuntu Flatpak — no separate folder. See `docs/BLUEPRINT.md:2`.

## Quick Start (Backend + Supabase local)

```bash
# prereqs: Docker, Go 1.22+, golang-migrate, psql (brew install postgresql@15 golang-migrate)
cp backend/.env.example backend/.env  # DATABASE_URL already points to docker db

# 1. DB (docker postgres with supabase extensions) + migrations + seed
make db:up
DATABASE_URL=postgres://postgres:postgres@localhost:54322/postgres?sslmode=disable make migrate:up
DATABASE_URL=postgres://postgres:postgres@localhost:54322/postgres?sslmode=disable make migrate:seed

# 2. Backend (dev mode: SUPABASE_JWT_SECRET empty → dev user 000...001)
DATABASE_URL=postgres://postgres:postgres@localhost:54322/postgres?sslmode=disable go run ./backend/cmd/server
# health: curl http://localhost:8080/healthz
# api:    curl http://localhost:8080/v1/workspaces | jq
# openapi: backend/api/openapi.yaml

# 3. Verify seeded data
# curl "http://localhost:8080/v1/channels?workspace_id=11111111-1111-1111-1111-111111111111" | jq
# curl "http://localhost:8080/v1/hosts?workspace_id=11111111-1111-1111-1111-111111111111" | jq
```

Full backend detail: `docs/BLUEPRINT.md:3`.

## Clients

Each native app is thin — API + Supabase Realtime (`postgres_changes`). Shared IA: sidebar (Workspaces→Channels/DMs/GMs + Projects + Terminal + IDE) | center (Messages/Tasks/Calendar/Terminal/IDE) | right (Thread/Agent panel). Theme via `design-tokens.json` (see `docs/UI-UX.md`). Terminal (Termius parity) via native PTY (ConPTY/libvte/swift-pty); IDE (Zed parity) via native buffer + LSP. See `docs/BLUEPRINT.md:3.5-3.7`.

```bash
make gen              # regenerate OpenAPI server/client stubs
# macos:  open macos/ControlCenter.xcodeproj
# ios:    open ios/ControlCenter.xcodeproj
# windows: open windows/ControlCenter.sln
# ubuntu: cd ubuntu && meson setup build && ninja -C build
# android: cd android && ./gradlew assembleDebug
```

## AI Personas (Hermes)

Hermes owns persona lifecycle (e.g. Sofia). Register `hermes_endpoint + persona` via `POST /v1/agents`, invite to workspace/channel/project/calendar via `POST /v1/agents/:id/invite`. Agents participate with `agent` RBAC role. See `docs/BLUEPRINT.md:5`.

## API Conventions

- Base `/v1`, envelope `{ data, error }`, cursor pagination `?cursor=&limit=50`, idempotency via `Idempotency-Key`. See `docs/BLUEPRINT.md:7`.

## Design System

B&W only — hierarchy via weight/size/whitespace/borders. Tokens in `design-tokens.json`. No accent color, no shadows, mono icons 1.5px stroke. Keyboard-first (`Cmd+K`, `J/K`, `/`, `@`). See `docs/UI-UX.md`.

## Contributing

AGPL-3.0. `main` is protected — PR required. CI checks `make gen` drift, lint, tests, builds.

## Docs

- [Blueprint](docs/BLUEPRINT.md) · [UI/UX](docs/UI-UX.md) · [OpenAPI](backend/api/openapi.yaml)
