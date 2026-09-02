import SwiftUI
struct ContentView: View {
    var body: some View {
        NavigationSplitView {
            List { Label("General", systemImage: "bubble.left") }
                .navigationTitle("Control Center")
        } content: {
            Text("Select a channel").foregroundStyle(.secondary)
        } detail: {
            VStack(spacing: 16) {
                Text("Control Center").font(.system(size: 26, weight: .bold))
                Text("B&W • Minimal • Programmer UX").font(.system(size: 11, design: .monospaced)).foregroundStyle(.secondary)
                Text("Wiring to http://localhost:8080/v1 in Phase 2").font(.system(size: 12, design: .monospaced))
            }
        }
    }
}
