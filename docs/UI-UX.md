# Control Center — UI/UX Design Guideline (B&W Minimalist)

## 1. Principles
Minimalist, programmer-first. No accent color — hierarchy only via weight, size, whitespace, and borders. Monochrome (#000/#FFF) forces clarity; gray is only for muted secondary text. Every screen must feel like a code editor: dense where needed, generous whitespace elsewhere.

## 2. Tokens (`design-tokens.json` — single source of truth)
```json
{
  "colors": { "black": "#000000", "white": "#FFFFFF", "gray900": "#1A1A1A", "gray600": "#666666", "gray400": "#888888", "gray100": "#F2F2F2", "border": "#E5E5E5" },
  "spacing": [4, 8, 16, 24, 32, 48],
  "radius": { "sm": 6, "md": 10, "lg": 14 },
  "typography": {
    "sans": "Inter",
    "mono": "JetBrains Mono",
    "sizes": { "xs": 11, "sm": 12, "base": 13, "lg": 15, "xl": 20, "2xl": 26 },
    "weights": { "regular": 400, "medium": 500, "semibold": 600, "bold": 700 }
  }
}
```
- Sans for UI, mono for code/messages/timestamps/tasks. Never mix.
- Enforce via token loader in each native project; CI fails if a hard-coded color is introduced.

## 3. Layout (shared IA across all platforms)
```
┌─ Sidebar (240px) ─┬────── Center ──────┬─ Right Panel (320px, collapsible) ─┐
│ Workspaces        │ Channel / Tasks /  │ Thread / Agent / Terminal / IDE    │
│ Channels / DMs/GMs│ Calendar / Terminal│ Details                            │
│ Projects          │ IDE                │                                      │
│ Terminal │ IDE    │                    │                                      │
└───────────────────┴────────────────────┴──────────────────────────────────────┘
```
- Sidebar: workspace switcher (top), channels grouped (open/private/DM/GM), projects below, **Terminal** (host groups + sessions) and **IDE** (workspaces) as top-level nav (Termius + Zed parity), collapse to icons on narrow widths.
- Center: message list (virtualized), task board (list/kanban toggle), calendar month/week, **terminal PTY** (multi-tab, split panes client-side, B&W xterm), **IDE editor** (file tree left, buffer center, minimap off, command palette).
- Right: thread, task detail, agent panel, **terminal inspector** (SFTP/recording) or **IDE outline/LSP diagnostics** — one at a time.

## 4. Type Scale
- Workspace/channel title: 13 semibold #1A1A1A
- Message author: 13 semibold #1A1A1A, timestamp 11 mono #888888
- Message body: 13 regular #1A1A1A, line-height 1.5
- Section header (e.g. "Threads", "Tasks"): 11 semibold uppercase tracking 0.06em #666666
- Input placeholder: 13 regular #888888

## 5. Components
- **Buttons**: primary = black bg white text, secondary = white bg black border, ghost = text only. Radius 6, height 32, padding 0 14. No shadows.
- **Inputs**: 1px border #E5E5E5, focus border #000, radius 6, height 36.
- **Channel row**: 32h, 6 radius, selected = black bg white text, unread = 600 weight + dot.
- **Message**: avatar 28 circle (initials, B&W), no bubble — flat list with 1px divider #F2F2F2 between days.
- **Task row**: mono status pill (TODO/DOING/DONE) with border only, no fill, 10 mono.
- **Calendar**: grid with 1px borders, today = inverted (black bg white text), event = left border 2px black.
- **Agent badge**: `◉ Sofia` mono 11 #666666, persona tag bordered.
- **Terminal**: host row 32h mono 12, group header 11 semibold uppercase #666666; PTY view black bg (light: #0A0A0A) white mono 13, cursor block white, selection #333; tab bar border-bottom 1px #E5E5E5, active tab black bg white text; SFTP file row 28h mono 12 with 1px divider.
- **IDE**: file tree row 24h mono 12, selected = black bg white text; buffer line numbers 11 mono #888888 gutter 40px; editor mono 13 #1A1A1A; diagnostic underline 1px dotted #000 (no color), LSP completion popup bordered 1px #000, no shadow.

## 6. Motion
- No color transitions. Only 120ms ease for panel open/close and message-in. No bounce.

## 7. Platform Notes
- macOS: respect vibrancy off (pure white/black), unified toolbar.
- Windows: WinUI monochrome theme, respect system title bar.
- Ubuntu: libadwaita with `prefer-dark` off by default; B&W overrides.
- Android/iOS: edge-to-edge, gesture nav, dynamic type maps to token sizes.

## 8. States (Empty / Loading / Error)
- Empty channel: centered mono 13 #888888 "No messages yet — send the first one." + primary button "New message".
- Empty tasks: dashed border card, "No tasks — create one or link GitHub."
- Empty terminal: dashed card "No hosts — add one" + "Create session" when host selected; PTY disconnected shows `● disconnected` mono 11.
- Empty IDE: dashed card "No workspaces — create one" + "Open buffer".
- Loading: skeleton lines (border #F2F2F2, no shimmer color) — 3 rows, 16h each.
- Error: inline bordered alert black border, mono 12, retry link underlined. Toast for transient errors (top-right, black bg white text, auto-dismiss 4s).
- Unsent/offline: message row shows `◌` mono 10 #888888 + "Offline — queued." Terminal requires live WS — shows `offline` banner; IDE buffers show `● unsaved` + queued sync.

## 9. Accessibility & Keyboard (Programmer UX)
- Contrast: text #1A1A1A on #FFFFFF = 17:1. All interactive targets ≥ 32px, focus ring 2px solid #000 (offset 2px).
- Screen reader: semantic roles per platform, avatar `alt` = display name, channel `aria-label` includes unread count.
- Keyboard (all platforms):
  - `Cmd/Ctrl+K` quick switcher (channels/tasks/people/hosts/buffers)
  - `Cmd/Ctrl+B` toggle sidebar, `Cmd/Ctrl+.` toggle right panel
  - `R` reply in thread, `E` edit last message, `⌘+Enter` send
  - `G then C` calendar, `G then P` projects, `G then T` terminal, `G then I` IDE, `J/K` next/prev channel/host/buffer
  - `/` slash command palette, `@` mention autocomplete, `:` emoji picker (mono search)
  - Terminal: `Cmd+T` new tab, `Cmd+W` close, `Cmd+D` split, `Cmd+Shift+C` copy
  - IDE: `Cmd+P` file palette, `Cmd+Shift+F` search, `F12` go to definition, `Esc` close palette
  - Full tab order, Esc closes panels/modals.

## 10. Notifications & Presence
- Presence dot: 8px circle, online = solid black, offline = white with black border 1.5px, away = half-filled.
- Unread: sidebar dot 6px black + bold channel name; mention = black pill with white text count.
- Toast: black bg white text 13, 6 radius, 8 spacing, appears top-right (desktop) / bottom (mobile).

## 11. Iconography & Illustration
- Icons: mono stroke 1.5px, 16px, no fill (lucide/feather style). No colored icons.
- No illustrations; empty states use bordered wireframe boxes only.

## 12. Responsive Breakpoints
- Desktop ≥1024px: three panels. 768–1023px: collapsible sidebar (overlay). <768px (mobile): single panel with bottom tab bar (Chat / Tasks / Calendar / You). Right panel becomes sheet.

## 13. Theme Modes
- Light (default): white bg black text as above.
- Dark (system `prefers-color-scheme`): invert — #000 bg, #FFF text, border #2A2A2A, muted #777. No gray fills; borders only. Toggle in Settings, respects OS.

## 14. Do / Don't
- Do: use borders and whitespace to separate, weight to emphasize.
- Don't: introduce gray fills, colored status, shadows, gradients, or emoji beyond message content.
- Don't: hard-code colors — always via tokens.
- Don't: add color-coded status pills — use mono bordered labels (TODO / DOING / DONE) only.
