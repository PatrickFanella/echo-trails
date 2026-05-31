import Foundation

public struct PublicProfileDescriptor: Equatable, Sendable {
    public let handle: String
    public let displayName: String
    public let bio: String
    public let publicEchoCount: Int
    public let profileAPIPath: String

    public init(handle: String, displayName: String, bio: String = "", publicEchoCount: Int = 0) {
        self.handle = handle
        self.displayName = displayName
        self.bio = bio
        self.publicEchoCount = max(0, publicEchoCount)
        self.profileAPIPath = ContractPath.userProfile(handle) ?? ""
    }
}

public struct PrivateProfileSummary: Equatable, Sendable {
    public let handle: String
    public let pendingUploadCount: Int
    public let moderationReviewCount: Int
    public let outboxCount: Int

    public init(handle: String, pendingUploadCount: Int = 0, moderationReviewCount: Int = 0, outboxCount: Int = 0) {
        self.handle = handle
        self.pendingUploadCount = max(0, pendingUploadCount)
        self.moderationReviewCount = max(0, moderationReviewCount)
        self.outboxCount = max(0, outboxCount)
    }
}
