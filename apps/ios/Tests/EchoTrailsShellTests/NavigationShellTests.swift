import XCTest
@testable import EchoTrailsShell

final class NavigationShellTests: XCTestCase {
    func testDefaultNavigationStartsOnMapTab() {
        let state = NavigationState()

        XCTAssertEqual(state.selectedTab, .map)
        XCTAssertEqual(state.route, .tab(.map))
        XCTAssertEqual(state.screen.title, "Map")
        XCTAssertFalse(state.screen.requiresAuthentication)
    }

    func testTabSelectionKeepsStableTabOrder() {
        let state = NavigationState().selecting(.activity)

        XCTAssertEqual(ShellTabs.allCases, [.map, .explore, .activity, .profile])
        XCTAssertEqual(state.selectedTab, .activity)
        XCTAssertEqual(state.screen.accessibilityLabel, "Activity tab")
        XCTAssertTrue(state.screen.requiresAuthentication)
    }

    func testDeepLinkParsingForCoreRoutes() {
        XCTAssertEqual(NavigationRoute.parse(path: "/places/place-active"), .placeDetail(id: "place-active"))
        XCTAssertEqual(NavigationRoute.parse(path: "/echoes/echo-active-1"), .echoDetail(id: "echo-active-1"))
        XCTAssertEqual(NavigationRoute.parse(path: "/users/archivist"), .userProfile(handle: "archivist"))
        XCTAssertEqual(NavigationRoute.parse(path: "/uploads/upl_abc123"), .mediaUploadStatus(uploadID: "upl_abc123"))
    }

    func testRouteMetadataSelectsPreferredTabsAndAPIPaths() {
        let place = NavigationState(route: .placeDetail(id: "place-active"))
        let echo = NavigationState(route: .echoDetail(id: "echo-active-1"))
        let upload = NavigationState(route: .mediaUploadStatus(uploadID: "upl_abc123"))

        XCTAssertEqual(place.selectedTab, .map)
        XCTAssertEqual(place.screen.requiredAPIPath, "/places/place-active")
        XCTAssertEqual(echo.selectedTab, .explore)
        XCTAssertEqual(echo.screen.requiredAPIPath, "/echoes/echo-active-1")
        XCTAssertEqual(upload.selectedTab, .activity)
        XCTAssertEqual(upload.screen.requiredAPIPath, "/media/uploads/upl_abc123")
        XCTAssertTrue(upload.screen.requiresAuthentication)
    }

    func testPublicUserProfileRouteIsNotAuthGated() {
        let profile = NavigationState(route: .userProfile(handle: "archivist"))

        XCTAssertEqual(profile.selectedTab, .profile)
        XCTAssertEqual(profile.screen.requiredAPIPath, "/users/archivist")
        XCTAssertFalse(profile.screen.requiresAuthentication)
    }

    func testRejectsUnsafeOrUnknownRoutes() {
        XCTAssertNil(NavigationRoute.parse(path: "/places/../secret"))
        XCTAssertNil(NavigationRoute.parse(path: "/places/café"))
        XCTAssertNil(NavigationRoute.parse(path: "/places/place/extra"))
        XCTAssertNil(NavigationRoute.parse(path: "/unknown/value"))
    }
}
