# Ubuntu — Control Center (GTK4/libadwaita)
Build: `meson setup build && ninja -C build`
Flatpak: `flatpak-builder build flatpak/manifest.json`
Theme: pure B&W from `design-tokens.json`.

## Terminal (libvte B&W)
`meson setup build && ninja -C build && ./build/controlcenter-terminal`
WS: `ws://localhost:8080/v1/terminal/sessions/:id/ws` (JSON {type:"input",data} / binary PTY)
