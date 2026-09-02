import SwiftUI
struct ContentView: View {
    @StateObject var api = APIClient()
    @State var selectedChannel: String?
    var body: some View {
        NavigationSplitView {
            List(selection: $selectedChannel) {
                Section { Label("General", systemImage: "bubble.left").tag("general") }
                    header: { Text("Channels").font(.system(size: 11, weight: .semibold)).foregroundStyle(DesignTokens.gray600) }
                Section {
                    ForEach(api.workspaces) { ws in Label(ws.name, systemImage: "folder") }
                } header: { Text("Workspaces").font(.system(size: 11, weight: .semibold)).foregroundStyle(DesignTokens.gray600) }
            }
            .navigationTitle("Control Center")
            .task { await api.fetchWorkspaces() }
        } content: {
            VStack(spacing: 8) {
                Text("Select a channel").foregroundStyle(.secondary)
                if let ch = selectedChannel { Text(ch).font(.system(size: 12, design: .monospaced)) }
            }
        } detail: {
            VStack(spacing: 16) {
                Text("Control Center").font(.system(size: 26, weight: .bold)).foregroundStyle(DesignTokens.gray900)
                Text("B&W • Minimal • Programmer UX").font(.system(size: 11, design: .monospaced)).foregroundStyle(DesignTokens.gray400)
                Text("API: \(APIClient.base.absoluteString)").font(.system(size: 12, design: .monospaced)).foregroundStyle(DesignTokens.gray600)
                Divider().overlay(DesignTokens.border)
                Text("Endpoints: /workspaces /channels /messages /projects /tasks /calendars /agents").font(.system(size: 11, design: .monospaced)).foregroundStyle(.secondary)
            }.padding(24)
        }
    }
}
