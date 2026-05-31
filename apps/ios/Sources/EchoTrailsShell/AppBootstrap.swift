import Foundation

public struct AppBootstrap: Equatable, Sendable {
    public let configuration: AppConfiguration
    public let apiClient: APIClientConfiguration
    public let navigationTabs: [ShellTabs]
    public let navigationDescriptors: [NavigationTabDescriptor]
    public let navigationState: NavigationState

    public init(configuration: AppConfiguration = AppConfiguration(), initialRoute: NavigationRoute = .tab(.map)) {
        self.configuration = configuration
        self.apiClient = APIClientConfiguration.from(configuration)
        self.navigationTabs = ShellTabs.allCases
        self.navigationDescriptors = ShellTabs.allCases.map(NavigationTabDescriptor.init(tab:))
        self.navigationState = NavigationState(route: initialRoute)
    }
}

public struct NavigationTabDescriptor: Equatable, Sendable {
    public let tab: ShellTabs
    public let title: String
    public let systemImageName: String

    public init(tab: ShellTabs) {
        self.tab = tab
        self.title = tab.title
        self.systemImageName = tab.systemImageName
    }
}
