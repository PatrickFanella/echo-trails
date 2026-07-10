import Foundation

public enum AppealStatus: String, CaseIterable, Sendable {
    case open
    case accepted
    case rejected
    case withdrawn

    public var title: String {
        switch self {
        case .open: "Under review"
        case .accepted: "Appeal accepted"
        case .rejected: "Appeal not accepted"
        case .withdrawn: "Appeal withdrawn"
        }
    }

    public var isEditable: Bool { self == .open }
}

public struct AppealListRequest: Equatable, Sendable {
    public let cursor: String?
    public let limit: Int

    public init(cursor: String? = nil, limit: Int = 30) {
        self.cursor = cursor?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.limit = limit
    }

    public var apiPath: String? {
        guard (1...100).contains(limit) else { return nil }
        guard let cursor, !cursor.isEmpty else { return "\(OpenAPIPaths.listAppeals)?limit=\(limit)" }
        var unreserved = CharacterSet.alphanumerics
        unreserved.insert(charactersIn: "-._~")
        guard let encoded = cursor.addingPercentEncoding(withAllowedCharacters: unreserved) else { return nil }
        return "\(OpenAPIPaths.listAppeals)?limit=\(limit)&cursor=\(encoded)"
    }
}

public struct AppealDraft: Equatable, Sendable {
    public let moderationActionID: String
    public let body: String

    public init(moderationActionID: String, body: String) {
        self.moderationActionID = moderationActionID.trimmingCharacters(in: .whitespacesAndNewlines)
        self.body = body.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public var isValid: Bool {
        ContractPath.safeSegment(moderationActionID) != nil && !body.isEmpty && body.count <= 2_000
    }
}

public enum ModerationStatusPresentation: Equatable, Sendable {
    case privateDraft
    case processing
    case awaitingReview
    case published
    case limited
    case removed
    case deleted
    case failed
    case unknown(String)

    public init(rawValue: String) {
        switch rawValue {
        case "draft", "uploading": self = .privateDraft
        case "processing": self = .processing
        case "pending_review": self = .awaitingReview
        case "published": self = .published
        case "limited": self = .limited
        case "removed": self = .removed
        case "deleted": self = .deleted
        case "failed": self = .failed
        default: self = .unknown(rawValue)
        }
    }

    public var title: String {
        switch self {
        case .privateDraft: "Private draft"
        case .processing: "Safety checks in progress"
        case .awaitingReview: "Awaiting moderator review"
        case .published: "Published"
        case .limited: "Visibility limited"
        case .removed: "Removed"
        case .deleted: "Deleted"
        case .failed: "Processing failed"
        case .unknown: "Status unavailable"
        }
    }

    public var isAdverse: Bool {
        switch self {
        case .limited, .removed: true
        default: false
        }
    }
}

public struct MediaPlaybackPolicy: Equatable, Sendable {
    public let dataSaverEnabled: Bool
    public let wifiOnlyAutoplay: Bool
    public let isOnWiFi: Bool
    public let reduceMotionEnabled: Bool

    public init(dataSaverEnabled: Bool, wifiOnlyAutoplay: Bool, isOnWiFi: Bool, reduceMotionEnabled: Bool) {
        self.dataSaverEnabled = dataSaverEnabled
        self.wifiOnlyAutoplay = wifiOnlyAutoplay
        self.isOnWiFi = isOnWiFi
        self.reduceMotionEnabled = reduceMotionEnabled
    }

    public var requiresExplicitLoad: Bool {
        dataSaverEnabled || (wifiOnlyAutoplay && !isOnWiFi)
    }

    public var allowsAutoplay: Bool {
        !requiresExplicitLoad && !reduceMotionEnabled
    }
}
