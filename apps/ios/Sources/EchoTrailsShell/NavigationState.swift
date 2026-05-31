import Foundation

public struct ScreenDescriptor: Equatable, Sendable {
    public let route: NavigationRoute
    public let title: String
    public let accessibilityLabel: String
    public let requiredAPIPath: String?
    public let requiresAuthentication: Bool

    public init(route: NavigationRoute, title: String, accessibilityLabel: String, requiredAPIPath: String? = nil) {
        self.route = route
        self.title = title
        self.accessibilityLabel = accessibilityLabel
        self.requiredAPIPath = requiredAPIPath
        self.requiresAuthentication = route.requiresAuthentication
    }
}

public struct NavigationState: Equatable, Sendable {
    public let selectedTab: ShellTabs
    public let route: NavigationRoute
    public let screen: ScreenDescriptor

    public init(route: NavigationRoute = .tab(.map)) {
        self.route = route
        self.selectedTab = route.preferredTab
        self.screen = ScreenDescriptor.describe(route)
    }

    public func selecting(_ tab: ShellTabs) -> NavigationState {
        NavigationState(route: .tab(tab))
    }

    public func navigating(to route: NavigationRoute) -> NavigationState {
        NavigationState(route: route)
    }
}

public extension ScreenDescriptor {
    static func describe(_ route: NavigationRoute) -> ScreenDescriptor {
        switch route {
        case .tab(let tab):
            return ScreenDescriptor(route: route, title: tab.title, accessibilityLabel: "\(tab.title) tab")
        case .placeDetail(let id):
            return ScreenDescriptor(route: route, title: "Place", accessibilityLabel: "Place detail", requiredAPIPath: "/places/\(id)")
        case .echoDetail(let id):
            return ScreenDescriptor(route: route, title: "Echo", accessibilityLabel: "Echo detail", requiredAPIPath: "/echoes/\(id)")
        case .userProfile(let handle):
            return ScreenDescriptor(route: route, title: "Profile", accessibilityLabel: "Profile for \(handle)", requiredAPIPath: "/users/\(handle)")
        case .mediaUploadStatus(let uploadID):
            return ScreenDescriptor(route: route, title: "Upload Status", accessibilityLabel: "Upload status", requiredAPIPath: "/media/uploads/\(uploadID)")
        }
    }
}
