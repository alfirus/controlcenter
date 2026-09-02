import SwiftUI
import Foundation

// Minimal B&W terminal view — wires to WS /terminal/sessions/:id/ws
// Real app swaps TextEditor for swift-pty + xterm.js NSView; here we keep pure SwiftUI for build-free preview.
struct TerminalView: View {
    let sessionId: String
    @State private var output = ""
    @State private var input = ""
    @State private var task: URLSessionWebSocketTask?
    @State private var connected = false

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text(connected ? "● \(sessionId.prefix(8))" : "○ disconnected")
                    .font(.system(size: 11, design: .monospaced)).foregroundStyle(DesignTokens.gray600)
                Spacer()
                Button("Clear") { output = "" }
                    .font(.system(size: 11, design: .monospaced))
            }.padding(8).background(DesignTokens.border.opacity(0.3))
            ScrollViewReader { proxy in
                ScrollView {
                    Text(output)
                        .font(.system(size: 12, design: .monospaced))
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(8)
                        .id("bottom")
                }.background(Color(hex: "#0A0A0A")).foregroundStyle(.white)
                 .onChange(of: output) { proxy.scrollTo("bottom", anchor: .bottom) }
            }
            HStack(spacing: 8) {
                TextField("input", text: $input, onCommit: { sendInput() })
                    .textFieldStyle(.roundedBorder).font(.system(size: 12, design: .monospaced))
                Button("Send") { sendInput() }
                    .buttonStyle(.borderedProminent).tint(.black).foregroundStyle(.white)
            }.padding(8).background(DesignTokens.border.opacity(0.2))
        }
        .task { connect() }
        .onDisappear { task?.cancel() }
    }

    func connect() {
        guard let url = URL(string: "ws://localhost:8080/v1/terminal/sessions/\(sessionId)/ws") else { return }
        let t = URLSession.shared.webSocketTask(with: url)
        task = t; t.resume(); connected = true
        recvLoop(t)
    }
    func recvLoop(_ t: URLSessionWebSocketTask) {
        t.receive { result in
            switch result {
            case .success(let msg):
                switch msg {
                case .data(let data): output += String(data: data, encoding: .utf8) ?? ""
                case .string(let s):
                    // try JSON wrapper else raw
                    if let d = s.data(using: .utf8), let obj = try? JSONSerialization.jsonObject(with: d) as? [String:Any], obj["type"] != nil {
                        output += s + "\n"
                    } else { output += s }
                @unknown default: break
                }
                recvLoop(t)
            case .failure: connected = false
            }
        }
    }
    func sendInput() {
        guard !input.isEmpty else { return }
        let payload = ["type":"input","data": input + "\n"] as [String:Any]
        if let data = try? JSONSerialization.data(withJSONObject: payload), let s = String(data: data, encoding: .utf8) {
            task?.send(.string(s)) { _ in }
        }
        input = ""
    }
}
