import SwiftUI

enum Theme {
    static let amber = Color(red: 0.85, green: 0.55, blue: 0.2)
    static let terracotta = Color(red: 0.78, green: 0.38, blue: 0.22)
    static let warmSand = Color(red: 0.96, green: 0.91, blue: 0.82)
    static let deepBrown = Color(red: 0.15, green: 0.1, blue: 0.08)
    static let warmGray = Color(red: 0.4, green: 0.35, blue: 0.3)
    static let softGlow = Color(red: 1.0, green: 0.75, blue: 0.35)

    static let cardBackground = Color(red: 0.18, green: 0.14, blue: 0.12)
    static let surfaceBackground = Color(red: 0.12, green: 0.09, blue: 0.07)

    static let gradient = LinearGradient(
        colors: [amber, terracotta],
        startPoint: .topLeading,
        endPoint: .bottomTrailing
    )

    static let subtleGradient = LinearGradient(
        colors: [amber.opacity(0.15), terracotta.opacity(0.08)],
        startPoint: .topLeading,
        endPoint: .bottomTrailing
    )
}
