import Foundation

public struct APIClientConfiguration: Equatable, Sendable {
    public let baseURL: URL
    public let timeoutSeconds: TimeInterval
    public let defaultHeaders: [String: String]

    public init(baseURL: URL, timeoutSeconds: TimeInterval = 30, defaultHeaders: [String: String] = [:]) {
        self.baseURL = baseURL
        self.timeoutSeconds = timeoutSeconds
        self.defaultHeaders = defaultHeaders
    }

    public static func from(_ configuration: AppConfiguration) -> APIClientConfiguration {
        APIClientConfiguration(
            baseURL: configuration.apiBaseURL,
            timeoutSeconds: 30,
            defaultHeaders: [
                "Accept": "application/json",
                "User-Agent": "\(configuration.identity.name)/0.1"
            ]
        )
    }

    public func url(for path: String) -> URL {
        let normalizedPath = path.hasPrefix("/") ? String(path.dropFirst()) : path
        return baseURL.appending(path: normalizedPath)
    }
}
