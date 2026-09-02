import SwiftUI
struct ContentView: View {
    @StateObject var api = APIClient()
    var body: some View {
        NavigationStack {
            List(api.workspaces) { ws in Label(ws.name, systemImage: "folder") }
            .navigationTitle("Control Center")
            .task { await api.fetch() }
            .overlay { if api.workspaces.isEmpty { ContentUnavailableView("No workspaces", systemImage: "folder", description: Text("Create one via API")) } }
        }
    }
}
