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
        ContractPath.safeSegment(echoID).map { OpenAPIPaths.putEchoReaction(echoId: $0) }
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
        ContractPath.safeSegment(echoID).map { OpenAPIPaths.createEchoReply(echoId: $0) }
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
        ContractPath.safeSegment(echoID).map { OpenAPIPaths.listEchoReplies(echoId: $0) }
    }
}

public struct ReplyThreadNode: Equatable, Sendable {
    public let id: String
    public let parentID: String?
    public let postedAt: Date

    public init(id: String, parentID: String? = nil, postedAt: Date) {
        self.id = id
        self.parentID = parentID
        self.postedAt = postedAt
    }
}

/// Produces a stable, oldest-first, one-level thread without dropping an
/// otherwise visible child when its parent is unavailable to the viewer.
public struct ReplyThreadLayout: Equatable, Sendable {
    public let rootIDs: [String]
    public let childIDsByParent: [String: [String]]

    public init(nodes: [ReplyThreadNode]) {
        let safeNodes = nodes
            .filter { ContractPath.safeSegment($0.id) != nil }
            .sorted { lhs, rhs in
                lhs.postedAt == rhs.postedAt ? lhs.id < rhs.id : lhs.postedAt < rhs.postedAt
            }
        let IDs = Set(safeNodes.map(\.id))
        let rootSet = Set(safeNodes.filter { node in
            guard let parentID = node.parentID else { return true }
            return !IDs.contains(parentID)
        }.map(\.id))
        self.rootIDs = safeNodes.filter { rootSet.contains($0.id) }.map(\.id)

        var children: [String: [String]] = [:]
        for node in safeNodes {
            guard let parentID = node.parentID, IDs.contains(parentID), !rootSet.contains(node.id) else { continue }
            children[parentID, default: []].append(node.id)
        }
        self.childIDsByParent = children
    }
}

public struct StitchComposerDescriptor: Equatable, Sendable {
    public let sourceEchoID: String
    public let title: String
    public let body: String
    public let creationToken: CreationTokenDescriptor?
    public let quotedReplyID: String?
    public let quoteWarningAcknowledged: Bool

    public init(sourceEchoID: String, title: String, body: String, creationToken: CreationTokenDescriptor? = nil, quotedReplyID: String? = nil, quoteWarningAcknowledged: Bool = false) {
        self.sourceEchoID = sourceEchoID
        self.title = title.trimmingCharacters(in: .whitespacesAndNewlines)
        self.body = body.trimmingCharacters(in: .whitespacesAndNewlines)
        self.creationToken = creationToken
        self.quotedReplyID = quotedReplyID
        self.quoteWarningAcknowledged = quoteWarningAcknowledged
    }

    public var apiPath: String? {
        ContractPath.safeSegment(sourceEchoID).map { OpenAPIPaths.createEchoStitch(echoId: $0) }
    }

    public func isValid(at now: Date) -> Bool {
        apiPath != nil && !title.isEmpty && title.count <= 80 && !body.isEmpty && body.count <= 500 && creationToken?.isValid(at: now) == true && quoteIsValid
    }

    private var quoteIsValid: Bool {
        quotedReplyID == nil || (ContractPath.safeSegment(quotedReplyID ?? "") != nil && quoteWarningAcknowledged)
    }
}

public struct ReplyRevisionDescriptor: Equatable, Sendable {
    public let replyID: String

    public init(replyID: String) { self.replyID = replyID }

    public var resourceAPIPath: String? { ContractPath.safeSegment(replyID).map { OpenAPIPaths.updateReply(replyId: $0) } }
    public var editAPIPath: String? { resourceAPIPath }
    public var deleteAPIPath: String? { resourceAPIPath }
    public var historyAPIPath: String? { ContractPath.safeSegment(replyID).map { OpenAPIPaths.getReplyHistory(replyId: $0) } }
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

    public var apiPath: String { OpenAPIPaths.createReport }

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
        kind == .blockUser ? OpenAPIPaths.blockUser : OpenAPIPaths.hideTarget
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
