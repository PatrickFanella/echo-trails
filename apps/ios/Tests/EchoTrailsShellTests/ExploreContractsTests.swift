import XCTest
@testable import EchoTrailsShell

final class ExploreContractsTests: XCTestCase {
    func testPlaceSearchQueryIsDefaultAndValidated() {
        let query = ExploreSearchQuery(text: " civic steps ")

        XCTAssertEqual(query.text, "civic steps")
        XCTAssertEqual(query.scope, .places)
        XCTAssertTrue(query.isValid)
        XCTAssertEqual(query.apiPath, "/places/search")
        XCTAssertFalse(ExploreSearchQuery(text: "").isValid)
    }

    func testNearbyExploreUsesDiscoveryPath() {
        let query = ExploreSearchQuery(text: "parks", scope: .nearby)

        XCTAssertEqual(query.apiPath, "/discovery/nearby")
        XCTAssertTrue(OpenAPIPaths.all.contains("/discovery/nearby"))
    }

    func testExploreSectionsKeepPlaceFirstEmptyStates() {
        let sections = ExploreSectionDescriptor.defaultSections

        XCTAssertEqual(sections.map(\.scope), [.nearby, .subscribedPlaces, .echoText])
        XCTAssertTrue(sections.allSatisfy { !$0.emptyState.isEmpty })
    }

    func testExploreResultHasRankingAndAccessibilityLabel() {
        let result = ExploreResultDescriptor(placeID: "place-active", title: "Civic Steps", subtitle: "3 recent Echoes", rankingReason: .popularNearby)

        XCTAssertEqual(result.rankingReason, .popularNearby)
        XCTAssertEqual(result.accessibilityLabel, "Explore result: Civic Steps")
    }
}
