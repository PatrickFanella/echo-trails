import Foundation

public enum NavigationRoute: Equatable, Sendable {
    case tab(ShellTabs)
    case placeDetail(id: String)
    case echoDetail(id: String)
    case userProfile(handle: String)
    case mediaUploadStatus(uploadID: String)

    public var requiresAuthentication: Bool {
        switch self {
        case .tab(.activity), .tab(.profile), .mediaUploadStatus:
            return true
        case .tab(.map), .tab(.explore), .placeDetail, .echoDetail, .userProfile:
            return false
        }
    }

    public var preferredTab: ShellTabs {
        switch self {
        case .tab(let tab):
            return tab
        case .placeDetail:
            return .map
        case .echoDetail:
            return .explore
        case .userProfile:
            return .profile
        case .mediaUploadStatus:
            return .activity
        }
    }

    public static func parse(path: String) -> NavigationRoute? {
        let components = path
            .split(separator: "/")
            .map(String.init)
            .filter { !$0.isEmpty }

        guard let first = components.first else { return .tab(.map) }
        if components.count == 1, let tab = ShellTabs(rawValue: first) {
            return .tab(tab)
        }
        guard components.count == 2 else { return nil }
        let value = components[1]
        guard isSafeRouteValue(value) else { return nil }
        switch first {
        case "places":
            return .placeDetail(id: value)
        case "echoes":
            return .echoDetail(id: value)
        case "users":
            return .userProfile(handle: value)
        case "uploads":
            return .mediaUploadStatus(uploadID: value)
        default:
            return nil
        }
    }
}

private func isSafeRouteValue(_ value: String) -> Bool {
    guard !value.isEmpty, value.count <= 128, !value.contains("..") else { return false }
    return value.allSatisfy { character in
        (character >= "a" && character <= "z") ||
            (character >= "A" && character <= "Z") ||
            (character >= "0" && character <= "9") ||
            character == "-" || character == "_"
    }
}
