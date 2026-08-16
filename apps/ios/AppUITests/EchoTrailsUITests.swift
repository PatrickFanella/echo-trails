import XCTest

final class EchoTrailsUITests: XCTestCase {
    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testAuthenticationEntryPointsAreReachable() {
        let app = XCUIApplication()
        app.launchArguments += ["-AppleLanguages", "(en)", "-AppleLocale", "en_US"]
        app.launch()

        XCTAssertTrue(app.navigationBars["EchoTrails"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.textFields["Email"].exists)
        XCTAssertTrue(app.buttons["Send one-time code"].exists)
        XCTAssertTrue(app.buttons["Sign in with Apple"].exists)
    }

    func testAuthenticatedReleaseSurfacesAreReachable() throws {
        guard ProcessInfo.processInfo.environment["ECHOTRAILS_UI_TEST_AUTHENTICATED"] == "1" else {
            throw XCTSkip("Run after signing the test device into a prepared invite, consent, and profile account.")
        }
        let app = XCUIApplication()
        app.launch()

        for tab in ["Map", "Explore", "Activity", "Profile"] {
            XCTAssertTrue(app.tabBars.buttons[tab].waitForExistence(timeout: 15), "Missing \(tab) tab")
        }
        app.tabBars.buttons["Profile"].tap()
        XCTAssertTrue(app.staticTexts["Offline & uploads"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Safety & privacy"].exists)
        XCTAssertTrue(app.buttons["Upload & publishing status"].exists)
        XCTAssertTrue(app.buttons["Safety & Appeals"].exists)
        XCTAssertTrue(app.buttons["Notification settings"].exists)
        XCTAssertTrue(app.buttons["Signed-in devices"].exists)
        XCTAssertTrue(app.buttons["Export or delete account"].exists)
    }
}
