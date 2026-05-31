import XCTest
@testable import EchoTrailsShell

final class AuthContractsTests: XCTestCase {
    func testEmailAuthenticationRequestUsesAuthPathsAndValidation() {
        let request = EmailAuthenticationRequest(email: " User@Example.COM ")

        XCTAssertEqual(request.email, "user@example.com")
        XCTAssertTrue(request.isValid)
        XCTAssertEqual(request.startAPIPath, "/auth/email/start")
        XCTAssertEqual(request.verifyAPIPath, "/auth/email/verify")
        XCTAssertTrue(OpenAPIPaths.all.contains("/auth/email/start"))
        XCTAssertFalse(EmailAuthenticationRequest(email: "not email").isValid)
    }

    func testAppleAuthenticationRequestUsesExchangePathWithoutAuthServicesRuntime() {
        let request = AppleAuthenticationRequest(nonce: "nonce_123")

        XCTAssertTrue(request.isValid)
        XCTAssertEqual(request.exchangeAPIPath, "/auth/apple")
        XCTAssertTrue(OpenAPIPaths.all.contains("/auth/apple"))
        XCTAssertFalse(AppleAuthenticationRequest(nonce: "bad/nonce").isValid)
    }

    func testSessionTokenDescriptorRedactsAndExpires() {
        let now = Date(timeIntervalSince1970: 1_000)
        let session = SessionTokenDescriptor(userID: "user-1", sessionID: "sess-1", accessToken: "access-token-secret", refreshToken: "refresh-token-secret", accessTokenExpiresAt: Date(timeIntervalSince1970: 2_000), refreshTokenExpiresAt: Date(timeIntervalSince1970: 3_000))

        XCTAssertTrue(session.isAuthenticated(at: now))
        XCTAssertFalse(session.accessTokenRedacted.contains("token-secret"))
        XCTAssertFalse(session.refreshTokenRedacted.contains("token-secret"))
        XCTAssertFalse(session.isAuthenticated(at: Date(timeIntervalSince1970: 4_000)))
    }

    func testAuthenticationFlowUsesKeychainDescriptorAndNoTokenLogging() {
        let storage = SessionStorageDescriptor(kind: .keychain)
        let flow = AuthenticationFlowDescriptor(state: .loggedOut, storage: storage)

        XCTAssertEqual(flow.storage.kind, .keychain)
        XCTAssertTrue(flow.storage.storesAccessToken)
        XCTAssertTrue(flow.storage.storesRefreshToken)
        XCTAssertFalse(flow.storage.logsRawTokens)
    }

    func testAuthenticatedFlowResolvesExpiredSession() {
        let now = Date(timeIntervalSince1970: 1_000)
        let expiredSession = SessionTokenDescriptor(userID: "user-1", sessionID: "sess-1", accessToken: "access-token-secret", refreshToken: "refresh-token-secret", accessTokenExpiresAt: Date(timeIntervalSince1970: 900), refreshTokenExpiresAt: Date(timeIntervalSince1970: 3_000))
        let flow = AuthenticationFlowDescriptor(state: .authenticated, method: .emailCode, session: expiredSession)

        XCTAssertEqual(flow.resolvedState(at: now), .expired)
    }
}
