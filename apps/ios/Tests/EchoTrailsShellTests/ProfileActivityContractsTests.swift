import XCTest
@testable import EchoTrailsShell

final class ProfileActivityContractsTests: XCTestCase {
    func testPublicProfileExcludesPrivateAccountFields() {
        let profile = PublicProfileDescriptor(handle: "archivist", displayName: "Archivist", bio: "Places first", publicEchoCount: 4)

        XCTAssertEqual(profile.profileAPIPath, "/users/archivist")
        XCTAssertEqual(profile.publicEchoCount, 4)
    }

    func testPublicProfileRejectsUnsafeHandlePath() {
        let profile = PublicProfileDescriptor(handle: "bad/handle", displayName: "Bad")

        XCTAssertEqual(profile.profileAPIPath, "")
    }

    func testPrivateProfileSummaryOnlyExposesCounts() {
        let summary = PrivateProfileSummary(handle: "archivist", pendingUploadCount: 2, moderationReviewCount: 1, outboxCount: 3)

        XCTAssertEqual(summary.pendingUploadCount + summary.moderationReviewCount + summary.outboxCount, 6)
    }

    func testActivityCoversPhaseThreeAndSafetyKinds() {
        XCTAssertEqual(ActivityItemKind.allCases, [.notification, .mention, .reply, .stitch, .reaction, .moderation, .upload, .outbox, .placeAlert])
    }

    func testSensitiveActivityPreviewIsGeneric() {
        let item = ActivityItemDescriptor(id: "act-1", kind: .moderation, title: "Moderation update", preview: "specific reason", previewPolicy: .genericSensitive)

        XCTAssertEqual(item.preview, "Open EchoTrails to view this update.")
    }
}
