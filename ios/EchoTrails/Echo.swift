import Foundation
import CoreLocation

nonisolated enum EchoType: String, Codable, Sendable, CaseIterable, Identifiable {
    case audio
    case text
    case photo
    case video

    var id: String { rawValue }

    var icon: String {
        switch self {
        case .audio: "waveform"
        case .text: "text.bubble.fill"
        case .photo: "camera.fill"
        case .video: "video.fill"
        }
    }

    var label: String {
        switch self {
        case .audio: "Audio"
        case .text: "Text"
        case .photo: "Photo"
        case .video: "Video"
        }
    }
}

nonisolated enum PrivacyLevel: String, Codable, Sendable, CaseIterable, Identifiable {
    case fullProfile
    case displayNameOnly
    case anonymous

    var id: String { rawValue }

    var label: String {
        switch self {
        case .fullProfile: "Full Profile"
        case .displayNameOnly: "Display Name Only"
        case .anonymous: "Anonymous"
        }
    }

    var icon: String {
        switch self {
        case .fullProfile: "person.crop.circle.fill"
        case .displayNameOnly: "person.fill"
        case .anonymous: "theatermasks.fill"
        }
    }

    var description: String {
        switch self {
        case .fullProfile: "Your avatar, name, and bio are visible"
        case .displayNameOnly: "Only your display name is shown"
        case .anonymous: "No identity attached"
        }
    }
}

nonisolated enum ReactionType: String, Codable, Sendable, CaseIterable, Identifiable {
    case heart
    case fire
    case sad
    case laugh
    case clap
    case mindBlown

    var id: String { rawValue }

    var emoji: String {
        switch self {
        case .heart: "❤️"
        case .fire: "🔥"
        case .sad: "😢"
        case .laugh: "😂"
        case .clap: "👏"
        case .mindBlown: "🤯"
        }
    }
}

struct Echo: Identifiable, Hashable, Sendable {
    let id: String
    let type: EchoType
    let authorId: String
    let privacyLevel: PrivacyLevel
    let latitude: Double
    let longitude: Double
    let locationName: String
    let textContent: String?
    let mediaURL: String?
    let audioDuration: TimeInterval?
    let createdAt: Date
    var reactions: [ReactionType: Int]
    var replyCount: Int
    var stitchCount: Int
    let parentEchoId: String?

    var coordinate: CLLocationCoordinate2D {
        CLLocationCoordinate2D(latitude: latitude, longitude: longitude)
    }

    var totalReactions: Int {
        reactions.values.reduce(0, +)
    }

    var snippet: String {
        if let text = textContent, !text.isEmpty {
            return String(text.prefix(80))
        }
        switch type {
        case .audio:
            if let duration = audioDuration {
                return "\(Int(duration))s audio echo"
            }
            return "Audio echo"
        case .photo: return "Photo echo"
        case .video: return "Video echo"
        case .text: return ""
        }
    }

    var timeAgo: String {
        let interval = Date().timeIntervalSince(createdAt)
        if interval < 60 { return "just now" }
        if interval < 3600 { return "\(Int(interval / 60))m ago" }
        if interval < 86400 { return "\(Int(interval / 3600))h ago" }
        if interval < 604800 { return "\(Int(interval / 86400))d ago" }
        return "\(Int(interval / 604800))w ago"
    }

    func distance(from location: CLLocation) -> CLLocationDistance {
        let echoLocation = CLLocation(latitude: latitude, longitude: longitude)
        return location.distance(from: echoLocation)
    }

    func formattedDistance(from location: CLLocation) -> String {
        let meters = distance(from: location)
        if meters < 100 {
            return "\(Int(meters))m"
        } else if meters < 1000 {
            return "\(Int(meters / 10) * 10)m"
        } else {
            return String(format: "%.1fkm", meters / 1000)
        }
    }

    nonisolated func hash(into hasher: inout Hasher) {
        hasher.combine(id)
    }

    nonisolated static func == (lhs: Echo, rhs: Echo) -> Bool {
        lhs.id == rhs.id
    }
}
