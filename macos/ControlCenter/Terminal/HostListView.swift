import SwiftUI
struct HostListView: View {
    @State var hosts: [APIClient.Host] = []
    var body: some View {
        List(hosts, id: \.id) { h in
            VStack(alignment: .leading, spacing: 2) {
                Text(h.label).font(.system(size: 12, weight: .semibold, design: .monospaced))
                Text("\(h.username)@\(h.hostname):\(h.port)  \(h.tags.joined(separator: ","))")
                    .font(.system(size: 11, design: .monospaced)).foregroundStyle(DesignTokens.gray600)
            }.padding(.vertical, 2)
        }.navigationTitle("Hosts").task {
            // fetch via APIClient — placeholder wires to design tokens B&W list
        }
    }
}
extension APIClient {
    struct Host: Decodable { let id:String; let label:String; let hostname:String; let port:Int; let username:String; let tags:[String] }
}
