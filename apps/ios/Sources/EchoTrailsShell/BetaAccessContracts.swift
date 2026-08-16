import Foundation

public enum BetaAccessState: String, CaseIterable, Sendable {
    case inviteRequired = "invite_required"
    case cityUnavailable = "city_unavailable"
    case waitlist
    case accepted
}

public struct BetaAccessDescriptor: Equatable, Sendable {
    public let state: BetaAccessState
    public let citySlug: String
    public let inviteCode: String

    public init(state: BetaAccessState, citySlug: String = "", inviteCode: String = "") {
        self.state = state
        self.citySlug = citySlug
        self.inviteCode = inviteCode
    }

    public var apiPath: String { OpenAPIPaths.evaluateBetaAccess }

    public var allowsEntry: Bool { state == .accepted }

    public var requiresInviteCode: Bool { state == .inviteRequired }
}
