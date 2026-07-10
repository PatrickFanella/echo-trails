import XCTest
@testable import EchoTrailsShell

final class SocialContractsTests: XCTestCase {
    func testReactionTypesAndPathMatchOpenAPI() {
        XCTAssertEqual(SocialReactionType.allCases.map(\.rawValue), ["heart", "fire", "sad", "laugh", "clap", "mind_blown"])
        let descriptor = ReactionBarDescriptor(echoID: "echo-active-1", selectedReaction: .heart, totals: [.heart: 1])
        XCTAssertEqual(descriptor.apiPath, "/echoes/echo-active-1/reactions")
        XCTAssertTrue(OpenAPIPaths.all.contains("/echoes/{echoId}/reactions"))
        XCTAssertFalse(ReactionBarDescriptor(echoID: "bad/id").isActionable)
    }

    func testRepliesAreOneLevelAndBounded() {
        let reply = ReplyComposerDescriptor(echoID: "echo-active-1", body: String(repeating: "a", count: 500), parentReplyID: "reply-1")
        XCTAssertTrue(reply.isValid)
        XCTAssertEqual(reply.apiPath, "/echoes/echo-active-1/replies")
        XCTAssertTrue(OpenAPIPaths.all.contains("/echoes/{echoId}/replies"))
        XCTAssertFalse(ReplyComposerDescriptor(echoID: "echo-active-1", body: String(repeating: "a", count: 501)).isValid)
        XCTAssertFalse(ReplyComposerDescriptor(echoID: "echo-active-1", body: "ok", parentReplyID: "nested/reply").isValid)
        XCTAssertEqual(ReplyThreadDescriptor(echoID: "echo-active-1", replyIDsOldestFirst: ["reply-1", "bad/id", "reply-2"]).replyIDsOldestFirst, ["reply-1", "reply-2"])

        let layout = ReplyThreadLayout(nodes: [
            ReplyThreadNode(id: "child-late", parentID: "parent", postedAt: Date(timeIntervalSince1970: 4)),
            ReplyThreadNode(id: "parent", postedAt: Date(timeIntervalSince1970: 1)),
            ReplyThreadNode(id: "child-early", parentID: "parent", postedAt: Date(timeIntervalSince1970: 2)),
            ReplyThreadNode(id: "orphan", parentID: "unavailable", postedAt: Date(timeIntervalSince1970: 3)),
        ])
        XCTAssertEqual(layout.rootIDs, ["parent", "orphan"])
        XCTAssertEqual(layout.childIDsByParent["parent"], ["child-early", "child-late"])
    }

    func testStitchRequiresCreationTokenAndSourcePath() {
        let now = Date(timeIntervalSince1970: 100)
        let token = CreationTokenDescriptor(tokenID: "tok-1", placeID: "place-active-1", expiresAt: Date(timeIntervalSince1970: 200))
        let stitch = StitchComposerDescriptor(sourceEchoID: "echo-active-1", title: "Source card", body: "A linked Echo", creationToken: token)
        XCTAssertTrue(stitch.isValid(at: now))
        XCTAssertEqual(stitch.apiPath, "/echoes/echo-active-1/stitches")
        XCTAssertTrue(OpenAPIPaths.all.contains("/echoes/{echoId}/stitches"))
        XCTAssertFalse(StitchComposerDescriptor(sourceEchoID: "echo-active-1", title: "Source card", body: "A linked Echo").isValid(at: now))
        XCTAssertFalse(StitchComposerDescriptor(sourceEchoID: "echo-active-1", title: "Source card", body: "A linked Echo", creationToken: token, quotedReplyID: "reply-1").isValid(at: now))
        XCTAssertTrue(StitchComposerDescriptor(sourceEchoID: "echo-active-1", title: "Source card", body: "A linked Echo", creationToken: token, quotedReplyID: "reply-1", quoteWarningAcknowledged: true).isValid(at: now))
        let revision = ReplyRevisionDescriptor(replyID: "reply-1")
        XCTAssertEqual(revision.editAPIPath, "/replies/reply-1")
        XCTAssertEqual(revision.deleteAPIPath, "/replies/reply-1")
        XCTAssertEqual(revision.historyAPIPath, "/replies/reply-1/history")
    }

    func testReportsAreAnonymousAndHideImmediately() {
        let report = ReportFlowDescriptor(reporterUserID: "user-1", targetType: .echo, targetID: "echo-active-1", category: .harassment, details: "unsafe")
        XCTAssertTrue(report.isValid)
        XCTAssertEqual(report.targetType.rawValue, "echo")
        XCTAssertEqual(report.apiPath, "/reports")
        XCTAssertTrue(report.hidesTargetImmediately)
        XCTAssertTrue(report.anonymousToReportedUser)
        XCTAssertTrue(OpenAPIPaths.all.contains("/reports"))
        XCTAssertFalse(ReportFlowDescriptor(reporterUserID: "bad/user", targetID: "echo-active-1", category: .other, details: "unsafe").isValid)
        XCTAssertFalse(ReportFlowDescriptor(reporterUserID: "user-1", targetID: "echo-active-1", category: .other, details: "").isValid)
    }

    func testBlocksAndHidesUseSafetyPathsAndFilteringSemantics() {
        let hide = SafetyActionDescriptor(kind: .hideContent, actorUserID: "user-1", targetID: "echo-active-1")
        let block = SafetyActionDescriptor(kind: .blockUser, actorUserID: "user-1", targetID: "user-2")
        XCTAssertEqual(hide.apiPath, "/hides")
        XCTAssertEqual(block.apiPath, "/blocks")
        XCTAssertTrue(hide.filtersFutureContent)
        XCTAssertFalse(hide.blocksInteraction)
        XCTAssertTrue(block.blocksInteraction)
        XCTAssertTrue(OpenAPIPaths.all.contains("/hides"))
        XCTAssertTrue(OpenAPIPaths.all.contains("/blocks"))
    }

    func testSocialActionMetadataDefaultsToAuthenticated() {
        let descriptor = SocialActionRequestDescriptor(apiPath: "/reports", idempotencyKey: "report-echo-active-1")
        XCTAssertTrue(descriptor.requiresAuthentication)
        XCTAssertEqual(descriptor.idempotencyKey, "report-echo-active-1")
    }
}
