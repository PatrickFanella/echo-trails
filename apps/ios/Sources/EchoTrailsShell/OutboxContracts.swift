import Foundation

public enum OutboxState: String, CaseIterable, Sendable {
    case draft
    case readyToSubmit
    case uploading
    case waitingForModeration
    case failed
    case synced
}

public enum SyncConflictPolicy: String, Sendable {
    case serverWins
    case preserveLocalDraft
    case requireUserReview
}

public struct RetryPolicy: Equatable, Sendable {
    public let maxAttempts: Int
    public let baseDelaySeconds: Int

    public init(maxAttempts: Int = 5, baseDelaySeconds: Int = 2) {
        self.maxAttempts = max(1, maxAttempts)
        self.baseDelaySeconds = max(1, baseDelaySeconds)
    }

    public func delaySeconds(forAttempt attempt: Int) -> Int {
        baseDelaySeconds * max(1, min(attempt, maxAttempts))
    }
}

public struct OutboxItem: Equatable, Sendable {
    public let localID: String
    public let placeID: String
    public let state: OutboxState
    public let creationToken: CreationTokenDescriptor?
    public let retryCount: Int

    public init(localID: String, placeID: String, state: OutboxState, creationToken: CreationTokenDescriptor? = nil, retryCount: Int = 0) {
        self.localID = localID
        self.placeID = placeID
        self.state = state
        self.creationToken = creationToken
        self.retryCount = max(0, retryCount)
    }

    public func canRetry(at now: Date) -> Bool {
        state == .failed && creationToken?.isValid(at: now) == true
    }
}
