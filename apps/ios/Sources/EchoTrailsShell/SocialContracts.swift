import Foundation

public enum SocialReactionType: String, CaseIterable, Sendable {
    case heart
    case fire
    case sad
    case laugh
    case clap
    case mindBlown = "mind_blown"
}

public struct ReactionBarDescriptor: Equatable, Sendable {
    public let echoID: String
    public let selectedReaction: SocialReactionType?
    public let totals: [SocialReactionType: Int]

    public init(echoID: String, selectedReaction: SocialReactionType? = nil, totals: [SocialReactionType: Int] = [:]) {
        self.echoID = echoID
        self.selectedReaction = selectedReaction
        self.totals = totals
    }

    public var apiPath: String? {
        ContractPath.safeSegment(echoID).map { "/echoes/\($0)/reactions" }
    }

    public var isActionable: Bool {
        apiPath != nil
    }
}

public struct ReplyComposerDescriptor: Equatable, Sendable {
    public static let maxBodyLength = 500

    public let echoID: String
    public let body: String
    public let parentReplyID: String?

    public init(echoID: String, body: String, parentReplyID: String? = nil) {
        self.echoID = echoID
        self.body = body.trimmingCharacters(in: .whitespacesAndNewlines)
        self.parentReplyID = parentReplyID
    }

    public var apiPath: String? {
        ContractPath.safeSegment(echoID).map { "/echoes/\($0)/replies" }
    }

    public var isValid: Bool {
        apiPath != nil && !body.isEmpty && body.count <= Self.maxBodyLength && parentIsSafe
    }

    public var parentIsSafe: Bool {
        parentReplyID == nil || ContractPath.safeSegment(parentReplyID ?? "") != nil
    }
}

public struct ReplyThreadDescriptor: Equatable, Sendable {
    public let echoID: String
    public let replyIDsOldestFirst: [String]

    public init(echoID: String, replyIDsOldestFirst: [String]) {
        self.echoID = echoID
        self.replyIDsOldestFirst = replyIDsOldestFirst.filter { ContractPath.safeSegment($0) != nil }
    }

    public var apiPath: String? {
        ContractPath.safeSegment(echoID).map { "/echoes/\($0)/replies" }
    }
}

public struct StitchComposerDescriptor: Equatable, Sendable {
    public let sourceEchoID: String
    public let title: String
    public let body: String
    public let creationToken: CreationTokenDescriptor?

    public init(sourceEchoID: String, title: String, body: String, creationToken: CreationTokenDescriptor? = nil) {
        self.sourceEchoID = sourceEchoID
        self.title = title.trimmingCharacters(in: .whitespacesAndNewlines)
        self.body = body.trimmingCharacters(in: .whitespacesAndNewlines)
        self.creationToken = creationToken
    }

    public var apiPath: String? {
        ContractPath.safeSegment(sourceEchoID).map { "/echoes/\($0)/stitches" }
    }

    public func isValid(at now: Date) -> Bool {
        apiPath != nil && !title.isEmpty && title.count <= 80 && !body.isEmpty && body.count <= 2_000 && creationToken?.isValid(at: now) == true
    }
}

public struct SourceCardDescriptor: Equatable, Sendable {
    public let sourceEchoID: String
    public let sourceTitle: String
    public let sourceAuthorHandle: String
    public let genericPreview: String

    public init(sourceEchoID: String, sourceTitle: String, sourceAuthorHandle: String, genericPreview: String = "Open EchoTrails to view source context.") {
        self.sourceEchoID = sourceEchoID
        self.sourceTitle = sourceTitle
        self.sourceAuthorHandle = sourceAuthorHandle
        self.genericPreview = genericPreview
    }
}

public enum ReportCategory: String, CaseIterable, Sendable {
    case safety
    case harassment
    case privateInformation
    case sensitivePlace
    case spam
    case other
}

public struct ReportFlowDescriptor: Equatable, Sendable {
    public enum TargetType: String, Sendable {
        case echo
        case reply
        case user
    }

    public let reporterUserID: String
    public let targetType: TargetType
    public let targetID: String
    public let category: ReportCategory
    public let details: String
    public let hidesTargetImmediately: Bool
    public let anonymousToReportedUser: Bool

    public init(reporterUserID: String, targetType: TargetType = .echo, targetID: String, category: ReportCategory, details: String) {
        self.reporterUserID = reporterUserID
        self.targetType = targetType
        self.targetID = targetID
        self.category = category
        self.details = details.trimmingCharacters(in: .whitespacesAndNewlines)
        self.hidesTargetImmediately = true
        self.anonymousToReportedUser = true
    }

    public var apiPath: String { "/reports" }

    public var isValid: Bool {
        ContractPath.safeSegment(reporterUserID) != nil && ContractPath.safeSegment(targetID) != nil && !details.isEmpty && details.count <= 500
    }
}

public enum SafetyActionKind: String, Sendable {
    case hideContent
    case blockUser
}

public struct SafetyActionDescriptor: Equatable, Sendable {
    public let kind: SafetyActionKind
    public let actorUserID: String
    public let targetID: String

    public init(kind: SafetyActionKind, actorUserID: String, targetID: String) {
        self.kind = kind
        self.actorUserID = actorUserID
        self.targetID = targetID
    }

    public var apiPath: String {
        kind == .blockUser ? "/blocks" : "/hides"
    }

    public var filtersFutureContent: Bool {
        true
    }

    public var blocksInteraction: Bool {
        kind == .blockUser
    }

    public var isValid: Bool {
        ContractPath.safeSegment(actorUserID) != nil && ContractPath.safeSegment(targetID) != nil
    }
}

public struct SocialActionRequestDescriptor: Equatable, Sendable {
    public let apiPath: String
    public let requiresAuthentication: Bool
    public let idempotencyKey: String?

    public init(apiPath: String, requiresAuthentication: Bool = true, idempotencyKey: String? = nil) {
        self.apiPath = apiPath
        self.requiresAuthentication = requiresAuthentication
        self.idempotencyKey = idempotencyKey
    }
}
