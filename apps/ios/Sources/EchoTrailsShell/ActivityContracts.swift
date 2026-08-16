import Foundation

public enum ActivityItemKind: String, CaseIterable, Sendable {
    case notification
    case mention
    case reply
    case stitch
    case reaction
    case moderation
    case upload
    case outbox
    case placeAlert
}

public enum NotificationPreviewPolicy: String, Sendable {
    case publicSummary
    case genericSensitive
    case hidden
}

public struct ActivityItemDescriptor: Equatable, Sendable {
    public let id: String
    public let kind: ActivityItemKind
    public let title: String
    public let preview: String
    public let previewPolicy: NotificationPreviewPolicy

    public init(id: String, kind: ActivityItemKind, title: String, preview: String, previewPolicy: NotificationPreviewPolicy = .publicSummary) {
        self.id = id
        self.kind = kind
        self.title = title
        self.preview = previewPolicy == .genericSensitive ? "Open EchoTrails to view this update." : preview
        self.previewPolicy = previewPolicy
    }
}

public struct ModerationActivityDescriptor: Equatable, Sendable {
    public let uploadID: String
    public let status: String
    public let reason: String
}

public struct OutboxActivityDescriptor: Equatable, Sendable {
    public let localID: String
    public let state: OutboxState
    public let canRetry: Bool
}

public struct ActivityPageRequest: Equatable, Sendable {
    public let cursor: String?
    public let limit: Int

    public init(cursor: String? = nil, limit: Int = 50) {
        self.cursor = cursor?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.limit = limit
    }

    public var apiPath: String? {
        guard (1...100).contains(limit) else { return nil }
        guard let cursor, !cursor.isEmpty else { return "\(OpenAPIPaths.listNotifications)?limit=\(limit)" }
        var unreserved = CharacterSet.alphanumerics
        unreserved.insert(charactersIn: "-._~")
        guard let encoded = cursor.addingPercentEncoding(withAllowedCharacters: unreserved) else { return nil }
        return "\(OpenAPIPaths.listNotifications)?limit=\(limit)&cursor=\(encoded)"
    }
}

public enum ActivityTargetKind: Equatable, Sendable {
    case echo
    case place
    case moderationAction
    case none

    public static func resolve(notificationKind: String) -> ActivityTargetKind {
        switch notificationKind {
        case "reply", "reaction", "mention", "stitch", "upload", "upload_status": .echo
        case "place_alert": .place
        case "moderation", "moderation_update", "moderation_action": .moderationAction
        default: .none
        }
    }
}

public enum NotificationMuteTarget: String, Sendable {
    case place = "places"
    case echo = "echoes"
}

public struct NotificationMuteDescriptor: Equatable, Sendable {
    public let target: NotificationMuteTarget
    public let targetID: String

    public init(target: NotificationMuteTarget, targetID: String) {
        self.target = target
        self.targetID = targetID
    }

    public var apiPath: String? {
        ContractPath.safeSegment(targetID).map { OpenAPIPaths.putNotificationMute(targetType: target.rawValue, targetId: $0) }
    }
}

public struct PlaceSubscriptionDescriptor: Equatable, Sendable {
    public let placeID: String

    public init(placeID: String) { self.placeID = placeID }

    public var resourceAPIPath: String? {
        ContractPath.safeSegment(placeID).map { OpenAPIPaths.putPlaceSubscription(placeId: $0) }
    }

    public static let listAPIPath = OpenAPIPaths.listPlaceSubscriptions
}
