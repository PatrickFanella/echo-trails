import Foundation

struct UserProfile: Identifiable, Hashable, Sendable {
    let id: String
    var displayName: String
    var bio: String
    var avatarURL: String?
    let joinedAt: Date
    var echoCount: Int
    var totalReactionsReceived: Int

    var initials: String {
        let parts = displayName.split(separator: " ")
        if parts.count >= 2 {
            return "\(parts[0].prefix(1))\(parts[1].prefix(1))".uppercased()
        }
        return String(displayName.prefix(2)).uppercased()
    }
}
