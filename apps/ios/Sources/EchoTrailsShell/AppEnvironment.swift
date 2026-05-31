import Foundation

public enum AppEnvironment: String, CaseIterable, Sendable {
    case local
    case staging
    case production

    public var apiBaseURL: URL {
        switch self {
        case .local:
            return URL(string: "http://127.0.0.1:8080")!
        case .staging:
            return URL(string: "https://staging-api.echotrails.example")!
        case .production:
            return URL(string: "https://api.echotrails.example")!
        }
    }

    public var allowsInsecureLocalhost: Bool {
        self == .local
    }
}

public struct AppConfiguration: Equatable, Sendable {
    public let identity: AppIdentity
    public let environment: AppEnvironment
    public let apiBaseURL: URL

    public init(identity: AppIdentity = AppIdentity(), environment: AppEnvironment = .local, apiBaseURL: URL? = nil) {
        self.identity = identity
        self.environment = environment
        self.apiBaseURL = apiBaseURL ?? environment.apiBaseURL
    }
}
