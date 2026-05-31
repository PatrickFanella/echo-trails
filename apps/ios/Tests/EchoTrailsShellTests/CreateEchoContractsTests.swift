import XCTest
@testable import EchoTrailsShell

final class CreateEchoContractsTests: XCTestCase {
    func testTextDraftRequiresPlaceBodyAndCreationToken() {
        let now = Date(timeIntervalSince1970: 1_000)
        let invalid = EchoDraft(placeID: "", mode: .text, body: "")

        XCTAssertFalse(EchoDraftValidation.validate(invalid, now: now).isValid)

        let token = CreationTokenDescriptor(tokenID: "tok_123", placeID: "place-active", expiresAt: Date(timeIntervalSince1970: 2_000))
        let valid = EchoDraft(placeID: "place-active", mode: .text, body: "A story", creationToken: token)

        XCTAssertTrue(EchoDraftValidation.validate(valid, now: now).isValid)
    }

    func testDraftLimitsMatchPhaseThreeContract() {
        let now = Date(timeIntervalSince1970: 1_000)
        let token = CreationTokenDescriptor(tokenID: "tok_123", placeID: "place-active", expiresAt: Date(timeIntervalSince1970: 2_000))
        let draft = EchoDraft(placeID: "place-active", mode: .photoSet, title: String(repeating: "t", count: 81), caption: String(repeating: "c", count: 501), creationToken: token)

        let validation = EchoDraftValidation.validate(draft, now: now)

        XCTAssertFalse(validation.isValid)
        XCTAssertTrue(validation.reasons.contains("title_too_long"))
        XCTAssertTrue(validation.reasons.contains("caption_too_long"))
    }

    func testCreateFlowStepsStayPlaceFirst() {
        XCTAssertEqual(CreateFlowStep.allCases, [.choosePlace, .captureProof, .composeEcho, .review, .submit])
    }
}
