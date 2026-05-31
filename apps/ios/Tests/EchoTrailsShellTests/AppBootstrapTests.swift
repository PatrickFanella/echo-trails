import XCTest
@testable import EchoTrailsShell

final class AppBootstrapTests: XCTestCase {
    func testDefaultBootstrapUsesLocalEnvironmentAndAllTabs() {
        let bootstrap = AppBootstrap()

        XCTAssertEqual(bootstrap.configuration.environment, .local)
        XCTAssertEqual(bootstrap.apiClient.baseURL.absoluteString, "http://127.0.0.1:8080")
        XCTAssertEqual(bootstrap.navigationTabs, [.map, .explore, .activity, .profile])
    }

    func testAPIClientBuildsURLsFromGeneratedPaths() {
        let configuration = AppConfiguration(environment: .production)
        let client = APIClientConfiguration.from(configuration)

        XCTAssertTrue(OpenAPIPaths.all.contains("/places/search"))
        XCTAssertEqual(client.url(for: "/places/search").absoluteString, "https://api.echotrails.example/places/search")
        XCTAssertEqual(client.defaultHeaders["Accept"], "application/json")
        XCTAssertEqual(client.defaultHeaders["User-Agent"], "EchoTrails/0.1")
    }

    func testEnvironmentPolicyIsExplicit() {
        XCTAssertTrue(AppEnvironment.local.allowsInsecureLocalhost)
        XCTAssertFalse(AppEnvironment.staging.allowsInsecureLocalhost)
        XCTAssertFalse(AppEnvironment.production.allowsInsecureLocalhost)
    }

    func testNavigationDescriptorsAreStable() {
        let descriptors = ShellTabs.allCases.map(NavigationTabDescriptor.init(tab:))
        let bootstrap = AppBootstrap()

        XCTAssertEqual(descriptors.map(\.title), ["Map", "Explore", "Activity", "Profile"])
        XCTAssertEqual(descriptors.map(\.systemImageName), ["map", "safari", "bell", "person.crop.circle"])
        XCTAssertEqual(bootstrap.navigationDescriptors, descriptors)
    }
}
