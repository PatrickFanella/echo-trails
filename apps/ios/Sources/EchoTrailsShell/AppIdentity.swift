import Foundation

public struct AppIdentity: Equatable, Sendable {
    public let name: String
    public let bundleIdentifier: String
    public let minimumOS: String

    public init(
        name: String = "EchoTrails",
        bundleIdentifier: String = "tv.subcult.echotrails",
        minimumOS: String = "17.0"
    ) {
        self.name = name
        self.bundleIdentifier = bundleIdentifier
        self.minimumOS = minimumOS
    }
}

public enum ShellTabs: String, CaseIterable, Sendable {
    case map
    case explore
    case activity
    case profile

    public var title: String {
        switch self {
        case .map:
            return "Map"
        case .explore:
            return "Explore"
        case .activity:
            return "Activity"
        case .profile:
            return "Profile"
        }
    }

    public var systemImageName: String {
        switch self {
        case .map:
            return "map"
        case .explore:
            return "safari"
        case .activity:
            return "bell"
        case .profile:
            return "person.crop.circle"
        }
    }
}
