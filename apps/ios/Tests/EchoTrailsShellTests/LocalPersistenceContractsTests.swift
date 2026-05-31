import XCTest
@testable import EchoTrailsShell

final class LocalPersistenceContractsTests: XCTestCase {
    func testSwiftDataReadyDescriptorsDoNotClaimRuntimePersistence() {
        let entities = LocalCacheEntityDescriptor.swiftDataReadyEntities

        XCTAssertEqual(entities.map(\.kind), [.place, .echo, .profile, .mediaUpload, .outboxItem])
        XCTAssertEqual(entities.first(where: { $0.kind == .outboxItem })?.serverSourced, false)
    }

    func testOutboxRetryRequiresCapturedValidToken() {
        let now = Date(timeIntervalSince1970: 1_000)
        let token = CreationTokenDescriptor(tokenID: "tok_123", placeID: "place-active", expiresAt: Date(timeIntervalSince1970: 2_000))
        let item = OutboxItem(localID: "local-1", placeID: "place-active", state: .failed, creationToken: token, retryCount: 1)

        XCTAssertTrue(item.canRetry(at: now))
        XCTAssertFalse(OutboxItem(localID: "local-2", placeID: "place-active", state: .failed).canRetry(at: now))
    }

    func testRetryPolicyIsBounded() {
        let policy = RetryPolicy(maxAttempts: 3, baseDelaySeconds: 2)

        XCTAssertEqual(policy.delaySeconds(forAttempt: 1), 2)
        XCTAssertEqual(policy.delaySeconds(forAttempt: 9), 6)
    }
}
