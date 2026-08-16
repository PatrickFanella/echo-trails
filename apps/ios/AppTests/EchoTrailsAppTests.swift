import XCTest
import SwiftData
@testable import EchoTrails

final class EchoTrailsAppTests: XCTestCase {
    func testAPNsRegistrationIdempotencyKeyNeverContainsRawToken() {
        let token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
        let first = APNsTokenPrivacy.idempotencyKey(for: token)
        let second = APNsTokenPrivacy.idempotencyKey(for: token)

        XCTAssertEqual(first, second)
        XCTAssertTrue(first.hasPrefix("register-device-"))
        XCTAssertEqual(first.count, "register-device-".count + 64)
        XCTAssertFalse(first.contains(token))
    }

    func testPublicEchoContractHasNoExactCoordinateField() throws {
        let json = #"{"id":"e","author_handle":"a","place_id":"p","content_mode":"text","title":"t","status":"published","public_location":{"lat":1,"lon":2}}"#.data(using: .utf8)!
        let echo = try JSONDecoder.api.decode(EchoDTO.self, from: json)
        XCTAssertEqual(echo.publicLocation.lat, 1)
        XCTAssertFalse(String(reflecting: EchoDTO.self).contains("ExactLocation"))
    }

    @MainActor func testPersistenceStackIncludesDurableOutbox() throws {
        let container = try PersistenceStack.container(inMemory: true)
        container.mainContext.insert(DurableOutboxItem(operation: "upload", endpoint: "/media/uploads", body: Data()))
        try container.mainContext.save()
        XCTAssertEqual(try container.mainContext.fetch(FetchDescriptor<DurableOutboxItem>()).count, 1)
    }

    @MainActor func testComposerIntentIsDurableBeforeDraftMutation() throws {
        let container = try PersistenceStack.container(inMemory: true)
        let app = AppModel(modelContainer: container)
        let file = LocalMedia(
            url: URL(fileURLWithPath: "/private/outbox/captured.jpg"), mediaType: "image",
            contentType: "image/jpeg", byteSize: 42, durationSeconds: nil
        )
        let expiry = Date(timeIntervalSince1970: 1_800_000_000)
        let transaction = ComposerTransactionPayload(
            placeID: "place-public", contentMode: "photo_set", title: "A walk",
            bodyText: "", captionText: "Spring", creationToken: "captured-proof",
            creationTokenExpiresAt: expiry, sourceEchoID: "source-echo",
            quotedReplyID: "reply-1", quoteWarningAcknowledged: true, media: [file],
            draftIdempotencyKey: "draft-key", submitIdempotencyKey: "submit-key"
        )

        try app.enqueueComposer(transaction)

        let item = try XCTUnwrap(container.mainContext.fetch(FetchDescriptor<DurableOutboxItem>()).first)
        XCTAssertEqual(item.operation, "composer_transaction")
        XCTAssertEqual(item.endpoint, "/echoes/source-echo/stitches")
        XCTAssertEqual(item.creationToken, "captured-proof")
        XCTAssertEqual(item.filePaths, ["/private/outbox/captured.jpg"])
        let recovered = try JSONDecoder.api.decode(ComposerTransactionPayload.self, from: item.body)
        XCTAssertEqual(recovered, transaction)
        XCTAssertEqual(recovered.creationTokenExpiresAt, expiry)
        XCTAssertEqual(recovered.draftIdempotencyKey, "draft-key")
        XCTAssertEqual(recovered.attachments.first?.uploadIdempotencyKey, transaction.attachments.first?.uploadIdempotencyKey)
    }

    func testOutboxRetryIsBoundedAndBackoffIsCapped() {
        XCTAssertEqual(DurableOutboxPolicy.maximumAttempts, 8)
        XCTAssertEqual(DurableOutboxPolicy.delay(afterAttempt: 1), 10)
        XCTAssertEqual(DurableOutboxPolicy.delay(afterAttempt: 2), 20)
        XCTAssertEqual(DurableOutboxPolicy.delay(afterAttempt: 20), 3600)
    }

    func testDiscoveryResponsesDecodeRankedEchoesForFeedAndSearch() throws {
        let echo = #"{"id":"e","author_handle":"a","place_id":"p","content_mode":"text","title":"A story","status":"published","public_location":{"lat":41.8,"lon":-88.0}}"#
        let ranked = #"{"echo":\#(echo),"score":0.82,"label":"Popular nearby"}"#

        let feed = try JSONDecoder.api.decode(NotableResponseDTO.self, from: #"{"echoes":[\#(ranked)]}"#.data(using: .utf8)!)
        XCTAssertEqual(feed.echoes.first?.label, "Popular nearby")

        let search = try JSONDecoder.api.decode(SearchResponseDTO.self, from: #"{"places":[],"echoes":[\#(ranked)],"next_cursor":"next"}"#.data(using: .utf8)!)
        XCTAssertEqual(search.echoes.first?.echo.title, "A story")
        XCTAssertEqual(search.echoes.first?.score, 0.82)
        XCTAssertEqual(search.nextCursor, "next")
    }

    func testReplyContractsDecodeNestingEditsAndContinuity() throws {
        let json = #"{"replies":[{"id":"parent","echo_id":"echo","user_id":"user-a","author_handle":"trail_parent","body":"Reply deleted","edited":true,"hidden":true,"posted_at":"2026-07-09T12:00:00Z"},{"id":"child","echo_id":"echo","user_id":"user-b","author_handle":"trail_child","body":"Still visible","reply_to":"parent","posted_at":"2026-07-09T12:01:00Z"}]}"#.data(using: .utf8)!
        let response = try JSONDecoder.api.decode(RepliesDTO.self, from: json)

        XCTAssertTrue(response.replies[0].isContinuityPlaceholder)
        XCTAssertNil(response.replies[0].replyTo)
        XCTAssertEqual(response.replies[1].replyTo, "parent")
        XCTAssertEqual(response.replies[1].authorHandle, "trail_child")
        XCTAssertFalse(response.replies[1].edited)
        XCTAssertFalse(response.replies[1].hidden)
        let placeholder = response.replies[1].continuityPlaceholder()
        XCTAssertEqual(placeholder.replyTo, "parent")
        XCTAssertEqual(placeholder.body, "Reply deleted")
        XCTAssertTrue(placeholder.hidden)
    }

    func testReplyMutationAndHistoryMatchWireContract() throws {
        let body = ReplyMutationBody(body: "Nested", replyTo: "parent")
        let encoded = try JSONSerialization.jsonObject(with: JSONEncoder.api.encode(body)) as? [String: String]
        XCTAssertEqual(encoded?["body"], "Nested")
        XCTAssertEqual(encoded?["reply_to"], "parent")

        let historyJSON = #"{"revisions":[{"revision_number":1,"body":"First","created_at":"2026-07-09T12:00:00Z"},{"revision_number":2,"body":"Second","created_at":"2026-07-09T12:05:00Z"}]}"#.data(using: .utf8)!
        let history = try JSONDecoder.api.decode(ReplyHistoryDTO.self, from: historyJSON)
        XCTAssertEqual(history.revisions.map(\.revisionNumber), [1, 2])
        XCTAssertEqual(history.revisions.last?.body, "Second")
    }

    func testSubscriptionPreferencesAndActivityPageDecodeAuthoritativeState() throws {
        let subscriptionsJSON = #"{"subscriptions":[{"place_id":"place-1","notifications_enabled":false,"created_at":"2026-07-09T12:00:00Z"}]}"#.data(using: .utf8)!
        let subscriptions = try JSONDecoder.api.decode(SubscriptionsDTO.self, from: subscriptionsJSON)
        XCTAssertEqual(subscriptions.subscriptions.first?.placeID, "place-1")
        XCTAssertEqual(subscriptions.subscriptions.first?.notificationsEnabled, false)

        let preferencesJSON = #"{"direct_activity_enabled":true,"place_alerts_enabled":false,"moderation_enabled":true,"upload_status_enabled":false,"updated_at":"2026-07-09T12:00:00Z"}"#.data(using: .utf8)!
        let preferences = try JSONDecoder.api.decode(NotificationPreferencesDTO.self, from: preferencesJSON)
        XCTAssertFalse(preferences.placeAlertsEnabled)
        XCTAssertFalse(preferences.uploadStatusEnabled)

        let activityJSON = #"{"notifications":[{"id":"notification-1","kind":"place_alert","preview":"New Echo nearby","target_id":"place-1","created_at":"2026-07-09T12:00:00Z"}],"next_cursor":"opaque+/="}"#.data(using: .utf8)!
        let page = try JSONDecoder.api.decode(NotificationPageDTO.self, from: activityJSON)
        XCTAssertEqual(page.notifications.first?.targetID, "place-1")
        XCTAssertEqual(page.nextCursor, "opaque+/=")
    }

    func testAppealContractsDecodeAndEncodeWireFields() throws {
        let json = #"{"appeals":[{"id":"appeal-1","moderation_action_id":"action-1","body":"Please review","status":"open","created_at":"2026-07-09T12:00:00Z"}],"next_cursor":"next"}"#.data(using: .utf8)!
        let page = try JSONDecoder.api.decode(AppealPageDTO.self, from: json)
        XCTAssertEqual(page.appeals.first?.moderationActionID, "action-1")
        XCTAssertEqual(page.appeals.first?.statusPresentation, .open)
        XCTAssertEqual(page.nextCursor, "next")

        let encoded = try JSONSerialization.jsonObject(with: JSONEncoder.api.encode(AppealMutationDTO(moderationActionID: "action-1", body: "Context"))) as? [String: String]
        XCTAssertEqual(encoded?["moderation_action_id"], "action-1")
        XCTAssertEqual(encoded?["body"], "Context")
    }
}
