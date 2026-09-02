import Foundation
final class APIClient: ObservableObject {
    static let base = URL(string: "http://localhost:8080/v1")!
    @Published var workspaces: [Workspace] = []
    struct Workspace: Codable, Identifiable { let id: String; let name: String }
    struct DataWrap<T: Codable>: Codable { let data: T }
    func fetchWorkspaces() async {
        guard let url = URL(string: "/workspaces", relativeTo: Self.base) else { return }
        if let (data,_) = try? await URLSession.shared.data(from: url),
           let w = try? JSONDecoder().decode(DataWrap<[Workspace]>.self, from: data) { await MainActor.run { self.workspaces = w.data } }
    }
}
