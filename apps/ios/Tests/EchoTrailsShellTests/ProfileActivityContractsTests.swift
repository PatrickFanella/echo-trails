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

    func testActivityPaginationEscapesOpaqueCursor() throws {
        let first = try XCTUnwrap(ActivityPageRequest(limit: 50).apiPath)
        XCTAssertEqual(first, "/notifications?limit=50")
        let next = try XCTUnwrap(ActivityPageRequest(cursor: "opaque+/= cursor", limit: 25).apiPath)
        XCTAssertTrue(next.contains("cursor=opaque%2B%2F%3D%20cursor"))
        XCTAssertNil(ActivityPageRequest(limit: 101).apiPath)
    }

    func testActivityTargetsUseSafeProductRoutes() {
        XCTAssertEqual(ActivityTargetKind.resolve(notificationKind: "reply"), .echo)
        XCTAssertEqual(ActivityTargetKind.resolve(notificationKind: "upload_status"), .echo)
        XCTAssertEqual(ActivityTargetKind.resolve(notificationKind: "place_alert"), .place)
        XCTAssertEqual(ActivityTargetKind.resolve(notificationKind: "moderation"), .moderationAction)
        XCTAssertEqual(NotificationMuteDescriptor(target: .echo, targetID: "echo-1").apiPath, "/mutes/echoes/echo-1")
        XCTAssertNil(NotificationMuteDescriptor(target: .place, targetID: "bad/id").apiPath)
        XCTAssertEqual(PlaceSubscriptionDescriptor(placeID: "place-1").resourceAPIPath, "/places/place-1/subscription")
        XCTAssertEqual(PlaceSubscriptionDescriptor.listAPIPath, "/subscriptions")
    }
}
