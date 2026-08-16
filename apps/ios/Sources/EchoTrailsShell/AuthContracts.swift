import Foundation

public enum AuthenticationMethod: String, CaseIterable, Sendable {
    case emailCode
    case appleID
}

public enum AuthenticationState: String, Sendable {
    case loggedOut
    case emailCodeRequested
    case authenticating
    case authenticated
    case expired
}

public enum SecureSessionStorageKind: String, Sendable {
    case keychain
    case inMemoryMock
}

public struct EmailAuthenticationRequest: Equatable, Sendable {
    public let email: String
    public let startAPIPath: String
    public let verifyAPIPath: String

    public init(email: String) {
        self.email = email.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        self.startAPIPath = OpenAPIPaths.startEmailAuth
        self.verifyAPIPath = OpenAPIPaths.verifyEmailAuth
    }

    public var isValid: Bool {
        email.contains("@") && email.contains(".") && !email.contains(" ")
    }
}

public struct AppleAuthenticationRequest: Equatable, Sendable {
    public let nonce: String
    public let exchangeAPIPath: String

    public init(nonce: String) {
        self.nonce = nonce.trimmingCharacters(in: .whitespacesAndNewlines)
        self.exchangeAPIPath = OpenAPIPaths.signInWithApple
    }

    public var isValid: Bool {
        ContractPath.safeSegment(nonce) != nil
    }
}

public struct SessionTokenDescriptor: Equatable, Sendable {
    public let userID: String
    public let sessionID: String
    public let accessTokenRedacted: String
    public let refreshTokenRedacted: String
    public let accessTokenExpiresAt: Date
    public let refreshTokenExpiresAt: Date

    public init(userID: String, sessionID: String, accessToken: String, refreshToken: String, accessTokenExpiresAt: Date, refreshTokenExpiresAt: Date) {
        self.userID = userID
        self.sessionID = sessionID
        self.accessTokenRedacted = SessionTokenDescriptor.redact(accessToken)
        self.refreshTokenRedacted = SessionTokenDescriptor.redact(refreshToken)
        self.accessTokenExpiresAt = accessTokenExpiresAt
        self.refreshTokenExpiresAt = refreshTokenExpiresAt
    }

    public func isAuthenticated(at now: Date) -> Bool {
        !userID.isEmpty && !sessionID.isEmpty && accessTokenExpiresAt > now && refreshTokenExpiresAt > now
    }

    public static func redact(_ token: String) -> String {
        guard token.count > 8 else { return "[redacted]" }
        return "\(token.prefix(4))…\(token.suffix(4))"
    }
}

public struct SessionStorageDescriptor: Equatable, Sendable {
    public let kind: SecureSessionStorageKind
    public let storesAccessToken: Bool
    public let storesRefreshToken: Bool
    public let logsRawTokens: Bool

    public init(kind: SecureSessionStorageKind = .keychain, storesAccessToken: Bool = true, storesRefreshToken: Bool = true, logsRawTokens: Bool = false) {
        self.kind = kind
        self.storesAccessToken = storesAccessToken
        self.storesRefreshToken = storesRefreshToken
        self.logsRawTokens = logsRawTokens
    }
}

public struct AuthenticationFlowDescriptor: Equatable, Sendable {
    public let state: AuthenticationState
    public let method: AuthenticationMethod?
    public let session: SessionTokenDescriptor?
    public let storage: SessionStorageDescriptor

    public init(state: AuthenticationState = .loggedOut, method: AuthenticationMethod? = nil, session: SessionTokenDescriptor? = nil, storage: SessionStorageDescriptor = SessionStorageDescriptor()) {
        self.state = state
        self.method = method
        self.session = session
        self.storage = storage
    }

    public func resolvedState(at now: Date) -> AuthenticationState {
        if state == .authenticated, session?.isAuthenticated(at: now) != true {
            return .expired
        }
        return state
    }
}
