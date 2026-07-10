import Foundation

public enum AccessibilitySurface: String, CaseIterable, Sendable {
    case tab
    case mapMarker
    case exploreRow
    case createStep
    case mediaAttachment
    case profileRow
    case activityRow
    case moderationStatus
    case appealRow
    case transcript
}

public struct AccessibilityDescriptor: Equatable, Sendable {
    public let surface: AccessibilitySurface
    public let label: String
    public let hint: String
    public let supportsDynamicType: Bool

    public init(surface: AccessibilitySurface, label: String, hint: String = "", supportsDynamicType: Bool = true) {
        self.surface = surface
        self.label = label
        self.hint = hint
        self.supportsDynamicType = supportsDynamicType
    }
}

public enum AccessibilityAuditRule: String, CaseIterable, Sendable {
    case labelRequired
    case dynamicTypeRequired
    case sensitivePreviewGeneric
    case noColorOnlyState
}

public struct AccessibilityAuditResult: Equatable, Sendable {
    public let passed: Bool
    public let failedRules: [AccessibilityAuditRule]

    public static func audit(_ descriptors: [AccessibilityDescriptor]) -> AccessibilityAuditResult {
        var failures: [AccessibilityAuditRule] = []
        if descriptors.contains(where: { $0.label.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }) {
            failures.append(.labelRequired)
        }
        if descriptors.contains(where: { !$0.supportsDynamicType }) {
            failures.append(.dynamicTypeRequired)
        }
        return AccessibilityAuditResult(passed: failures.isEmpty, failedRules: failures)
    }
}

public enum PhaseThreeAccessibilityCatalog {
    public static let requiredDescriptors: [AccessibilityDescriptor] = [
        AccessibilityDescriptor(surface: .tab, label: "Map tab"),
        AccessibilityDescriptor(surface: .mapMarker, label: "Place marker"),
        AccessibilityDescriptor(surface: .exploreRow, label: "Explore result"),
        AccessibilityDescriptor(surface: .createStep, label: "Create Echo step"),
        AccessibilityDescriptor(surface: .mediaAttachment, label: "Media attachment"),
        AccessibilityDescriptor(surface: .profileRow, label: "Profile row"),
        AccessibilityDescriptor(surface: .activityRow, label: "Activity row"),
        AccessibilityDescriptor(surface: .moderationStatus, label: "Moderation status"),
        AccessibilityDescriptor(surface: .appealRow, label: "Appeal status"),
        AccessibilityDescriptor(surface: .transcript, label: "Media transcript")
    ]
}
