import SwiftUI
enum DesignTokens { static let black = Color(hex: "#000000"); static let white = Color(hex: "#FFFFFF"); static let gray900 = Color(hex: "#1A1A1A"); static let gray600 = Color(hex: "#666666"); static let border = Color(hex: "#E5E5E5") }
extension Color { init(hex: String) { let h = hex.replacingOccurrences(of: "#", with: ""); var v: UInt64 = 0; Scanner(string: h).scanHexInt64(&v); self.init(red: Double((v>>16)&0xFF)/255, green: Double((v>>8)&0xFF)/255, blue: Double(v&0xFF)/255) } }
