import XCTest
@testable import EchoTrailsShell

final class MapContractsTests: XCTestCase {
    func testMapViewportValidatesPublicCoordinateAndRadius() {
        XCTAssertTrue(MapViewport(center: CoordinateValue(latitude: 40.0, longitude: -73.0), radiusMeters: 1_000).isValid)
        XCTAssertFalse(MapViewport(center: CoordinateValue(latitude: 91.0, longitude: -73.0), radiusMeters: 1_000).isValid)
        XCTAssertFalse(MapViewport(center: CoordinateValue(latitude: 40.0, longitude: -73.0), radiusMeters: 1).isValid)
    }

    func testPlaceMarkerIsPlaceFirstAndAccessibilityReady() {
        let marker = PlaceMarkerDescriptor(placeID: "place-active", title: "Civic Steps", publicCoordinate: CoordinateValue(latitude: 40.0, longitude: -73.0), precision: .neighborhood, activityPulse: PlaceActivityPulse(recentEchoCount: 3, hasNewActivity: true))

        XCTAssertTrue(marker.isRenderable)
        XCTAssertEqual(marker.detailAPIPath, "/places/place-active")
        XCTAssertEqual(marker.accessibilityLabel, "Place marker: Civic Steps")
        XCTAssertEqual(marker.precision, .neighborhood)
        XCTAssertEqual(marker.activityPulse.recentEchoCount, 3)
    }

    func testPlaceMarkerRejectsUnsafePathIDs() {
        let marker = PlaceMarkerDescriptor(placeID: "bad/id", title: "Bad", publicCoordinate: CoordinateValue(latitude: 40.0, longitude: -73.0))

        XCTAssertFalse(marker.isRenderable)
        XCTAssertEqual(marker.detailAPIPath, "")
    }

    func testPlaceDetailDescriptorUsesGeneratedBackendPaths() {
        let detail = PlaceDetailDescriptor(placeID: "place-active")

        XCTAssertEqual(detail.echoesAPIPath, "/places/place-active/echoes")
        XCTAssertEqual(detail.nearbyAPIPath, "/discovery/nearby")
        XCTAssertTrue(OpenAPIPaths.all.contains("/discovery/nearby"))
        XCTAssertTrue(OpenAPIPaths.all.contains("/places/{placeId}/echoes"))
    }
}
