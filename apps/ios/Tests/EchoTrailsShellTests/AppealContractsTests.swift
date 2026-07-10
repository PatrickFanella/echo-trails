import XCTest
@testable import EchoTrailsShell

final class AppealContractsTests: XCTestCase {
    func testAppealPaginationEncodesOpaqueCursor() {
        XCTAssertEqual(AppealListRequest(limit: 30).apiPath, "/appeals?limit=30")
        XCTAssertEqual(AppealListRequest(cursor: "opaque+/=", limit: 20).apiPath, "/appeals?limit=20&cursor=opaque%2B%2F%3D")
        XCTAssertNil(AppealListRequest(limit: 101).apiPath)
    }

    func testAppealDraftRequiresSafeActionAndBoundedBody() {
        XCTAssertTrue(AppealDraft(moderationActionID: "018f-action", body: "Please review this context.").isValid)
        XCTAssertFalse(AppealDraft(moderationActionID: "../private", body: "Please review").isValid)
        XCTAssertFalse(AppealDraft(moderationActionID: "action", body: String(repeating: "a", count: 2_001)).isValid)
    }

    func testModerationAndAppealStatusesHaveAccessiblePresentation() {
        XCTAssertEqual(ModerationStatusPresentation(rawValue: "pending_review").title, "Awaiting moderator review")
        XCTAssertTrue(ModerationStatusPresentation(rawValue: "removed").isAdverse)
        XCTAssertEqual(AppealStatus.accepted.title, "Appeal accepted")
        XCTAssertFalse(AppealStatus.rejected.isEditable)
    }

    func testMediaPlaybackPolicyHonorsDataMotionAndNetworkPreferences() {
        XCTAssertTrue(MediaPlaybackPolicy(dataSaverEnabled: true, wifiOnlyAutoplay: false, isOnWiFi: true, reduceMotionEnabled: false).requiresExplicitLoad)
        XCTAssertFalse(MediaPlaybackPolicy(dataSaverEnabled: false, wifiOnlyAutoplay: true, isOnWiFi: false, reduceMotionEnabled: false).allowsAutoplay)
        XCTAssertFalse(MediaPlaybackPolicy(dataSaverEnabled: false, wifiOnlyAutoplay: false, isOnWiFi: true, reduceMotionEnabled: true).allowsAutoplay)
        XCTAssertTrue(MediaPlaybackPolicy(dataSaverEnabled: false, wifiOnlyAutoplay: true, isOnWiFi: true, reduceMotionEnabled: false).allowsAutoplay)
    }

    func testModerationNotificationsRouteToAppealFlow() {
        XCTAssertEqual(ActivityTargetKind.resolve(notificationKind: "moderation"), .moderationAction)
        XCTAssertEqual(ActivityTargetKind.resolve(notificationKind: "moderation_update"), .moderationAction)
    }
}
