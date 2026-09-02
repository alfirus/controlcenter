import SwiftUI
enum DesignTokens {
    static let black = Color(hex: "#000000")
    static let white = Color(hex: "#FFFFFF")
    static let gray900 = Color(hex: "#1A1A1A")
    static let gray600 = Color(hex: "#666666")
    static let gray400 = Color(hex: "#888888")
    static let border = Color(hex: "#E5E5E5")
    static let radius: CGFloat = 6
}
extension Color { init(hex: String) { let h = hex.trimmingCharacters(in: .whitespacesAndNewlines).replacingOccurrences(of: "#", with: ""); var v: UInt64 = 0; Scanner(string: h).scanHexInt64(&v); let r = Double((v >> 16) & 0xFF)/255, g = Double((v >> 8) & 0xFF)/255, b = Double(v & 0xFF)/255; self.init(red: r, green: g, blue: b) } }
