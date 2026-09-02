import Foundation
final class APIClient: ObservableObject {
    static let base = URL(string: "http://localhost:8080/v1")!
    @Published var workspaces: [Workspace] = []
    struct Workspace: Codable, Identifiable { let id:String; let name:String }
    struct Wrap<T:Codable>:Codable{let data:T}
    func fetch() async { guard let u = URL(string: "/workspaces", relativeTo: Self.base) else {return}; if let (d,_) = try? await URLSession.shared.data(from: u), let w = try? JSONDecoder().decode(Wrap<[Workspace]>.self, from: d) { await MainActor.run{ self.workspaces = w.data } } }
}
