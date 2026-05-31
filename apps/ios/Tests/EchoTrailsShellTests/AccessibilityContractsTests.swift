import XCTest
@testable import EchoTrailsShell

final class AccessibilityContractsTests: XCTestCase {
    func testPhaseThreeCatalogCoversRequiredSurfaces() {
        XCTAssertEqual(PhaseThreeAccessibilityCatalog.requiredDescriptors.map(\.surface), [.tab, .mapMarker, .exploreRow, .createStep, .mediaAttachment, .profileRow, .activityRow])
    }

    func testAccessibilityAuditRequiresLabelsAndDynamicType() {
        let passing = AccessibilityAuditResult.audit(PhaseThreeAccessibilityCatalog.requiredDescriptors)
        let failing = AccessibilityAuditResult.audit([AccessibilityDescriptor(surface: .tab, label: "", supportsDynamicType: false)])

        XCTAssertTrue(passing.passed)
        XCTAssertEqual(failing.failedRules, [.labelRequired, .dynamicTypeRequired])
    }
}
