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
