import XCTest
@testable import EchoTrailsShell

final class BetaAccessContractsTests: XCTestCase {
    func testBetaAccessStatesAndPath() {
        XCTAssertEqual(BetaAccessState.allCases.map(\.rawValue), ["invite_required", "city_unavailable", "waitlist", "accepted"])
        XCTAssertEqual(BetaAccessDescriptor(state: .accepted).apiPath, "/beta/access")
        XCTAssertTrue(BetaAccessDescriptor(state: .accepted).allowsEntry)
        XCTAssertTrue(BetaAccessDescriptor(state: .inviteRequired).requiresInviteCode)
    }
}
