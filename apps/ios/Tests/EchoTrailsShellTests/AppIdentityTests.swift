import XCTest
@testable import EchoTrailsShell

final class AppIdentityTests: XCTestCase {
    func testDefaultIdentityIsStable() {
        let identity = AppIdentity()

        XCTAssertEqual(identity.name, "EchoTrails")
        XCTAssertEqual(identity.bundleIdentifier, "tv.subcult.echotrails")
        XCTAssertEqual(identity.minimumOS, "17.0")
    }

    func testShellTabsDeclarePhaseThreeNavigationShape() {
        XCTAssertEqual(ShellTabs.allCases.map(\.rawValue), ["map", "explore", "activity", "profile"])
        XCTAssertEqual(ShellTabs.allCases.map(\.title), ["Map", "Explore", "Activity", "Profile"])
    }
}
