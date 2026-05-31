import Foundation

public enum LocalCacheEntityKind: String, CaseIterable, Sendable {
    case place
    case echo
    case profile
    case mediaUpload
    case outboxItem
}

public struct SchemaVersion: Equatable, Comparable, Sendable {
    public let major: Int
    public let minor: Int

    public init(major: Int, minor: Int = 0) {
        self.major = major
        self.minor = minor
    }
}

public struct LocalCacheEntityDescriptor: Equatable, Sendable {
    public let kind: LocalCacheEntityKind
    public let serverSourced: Bool
    public let expiresAfterSeconds: Int?

    public init(kind: LocalCacheEntityKind, serverSourced: Bool = true, expiresAfterSeconds: Int? = nil) {
        self.kind = kind
        self.serverSourced = serverSourced
        self.expiresAfterSeconds = expiresAfterSeconds
    }

    public static let swiftDataReadyEntities: [LocalCacheEntityDescriptor] = [
        LocalCacheEntityDescriptor(kind: .place, expiresAfterSeconds: 3_600),
        LocalCacheEntityDescriptor(kind: .echo, expiresAfterSeconds: 3_600),
        LocalCacheEntityDescriptor(kind: .profile, expiresAfterSeconds: 3_600),
        LocalCacheEntityDescriptor(kind: .mediaUpload, expiresAfterSeconds: 86_400),
        LocalCacheEntityDescriptor(kind: .outboxItem, serverSourced: false)
    ]
}
