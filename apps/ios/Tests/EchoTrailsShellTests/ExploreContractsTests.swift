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

    func testWeightedDiscoverySearchBuildsAllSupportedScopes() throws {
        let city = DiscoverySearchQuery(text: "  murals & music  ", scope: .city)
        XCTAssertTrue(city.isValid)
        XCTAssertEqual(city.text, "murals & music")
        XCTAssertTrue(try XCTUnwrap(city.apiPath).contains("q=murals%20%26%20music"))
        XCTAssertTrue(try XCTUnwrap(city.apiPath).contains("scope=city"))

        let place = DiscoverySearchQuery(text: "history", scope: .place, placeID: "place/with spaces")
        XCTAssertTrue(place.isValid)
        XCTAssertTrue(try XCTUnwrap(place.apiPath).contains("place_id=place/with%20spaces"))

        let nearby = DiscoverySearchQuery(text: "concert", scope: .nearby, latitude: 41.8, longitude: -88.01, radiusMeters: 2_000)
        let nearbyPath = try XCTUnwrap(nearby.apiPath)
        XCTAssertTrue(nearbyPath.contains("scope=nearby"))
        XCTAssertTrue(nearbyPath.contains("lat=41.8"))
        XCTAssertTrue(nearbyPath.contains("radius_meters=2000"))

        let next = city.withCursor("opaque-_")
        XCTAssertTrue(try XCTUnwrap(next.apiPath).contains("cursor=opaque-_"))
    }

    func testWeightedDiscoverySearchRejectsMissingScopeInputs() {
        XCTAssertNil(DiscoverySearchQuery(text: "", scope: .city).apiPath)
        XCTAssertNil(DiscoverySearchQuery(text: "echo", scope: .place).apiPath)
        XCTAssertNil(DiscoverySearchQuery(text: "echo", scope: .nearby).apiPath)
        XCTAssertNil(DiscoverySearchQuery(text: "echo", scope: .nearby, latitude: 100, longitude: 0).apiPath)
    }
}
