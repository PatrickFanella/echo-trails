import EchoTrailsShell
import Foundation
import Security

enum APIError: LocalizedError {
    case invalidConfiguration, unauthorized, invalidResponse, server(Int, String)
    var errorDescription: String? {
        switch self { case .invalidConfiguration: "The API URL is not configured."; case .unauthorized: "Please sign in again."; case .invalidResponse: "The server response was invalid."; case let .server(code, message): "Server error \(code): \(message)" }
    }
}

struct EmptyRequest: Encodable {}

actor SessionVault {
    struct Tokens: Codable { let accessToken: String; let refreshToken: String; let accessExpiresAt: Date; let refreshExpiresAt: Date; var refreshAttemptKey: String? = nil }
    private let service = "tv.subcult.echotrails.session"
    private let account = "current"

    func load() -> Tokens? {
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account, kSecReturnData as String: true]
        var result: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess, let data = result as? Data else { return nil }
        return try? JSONDecoder.api.decode(Tokens.self, from: data)
    }

    func save(_ tokens: Tokens) throws {
        let data = try JSONEncoder.api.encode(tokens)
        let query: [String: Any] = [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account]
        let attributes: [String: Any] = [kSecValueData as String: data, kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly]
        let status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            var insert = query; attributes.forEach { insert[$0.key] = $0.value }
            guard SecItemAdd(insert as CFDictionary, nil) == errSecSuccess else { throw APIError.invalidResponse }
        } else if status != errSecSuccess { throw APIError.invalidResponse }
    }

    func clear() { SecItemDelete([kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: account] as CFDictionary) }
}

extension JSONDecoder {
    static var api: JSONDecoder { let decoder = JSONDecoder(); decoder.dateDecodingStrategy = .iso8601; return decoder }
}
extension JSONEncoder {
    static var api: JSONEncoder { let encoder = JSONEncoder(); encoder.dateEncodingStrategy = .iso8601; encoder.outputFormatting = [.sortedKeys]; return encoder }
}

actor APIClient {
    nonisolated let baseURL: URL
    let vault: SessionVault
    private let session: URLSession

    init(baseURL: URL, vault: SessionVault, session: URLSession = .shared) { self.baseURL = baseURL; self.vault = vault; self.session = session }

    func get<Response: Decodable>(_ path: String, authenticated: Bool = true) async throws -> Response {
        try await send(path, method: "GET", body: Optional<EmptyRequest>.none, authenticated: authenticated)
    }

    func send<Response: Decodable, Body: Encodable>(_ path: String, method: String = "GET", body: Body? = nil, authenticated: Bool = true, idempotencyKey: String? = nil) async throws -> Response {
        var request = URLRequest(url: try url(path)); request.httpMethod = method; request.timeoutInterval = 30
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body { request.httpBody = try JSONEncoder.api.encode(body); request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        if authenticated {
            guard let tokens = await vault.load() else { throw APIError.unauthorized }
            request.setValue("Bearer \(tokens.accessToken)", forHTTPHeaderField: "Authorization")
        }
        let mutationKey = idempotencyKey ?? (method == "GET" || method == "HEAD" ? nil : UUID().uuidString)
        if let mutationKey { request.setValue(mutationKey, forHTTPHeaderField: "Idempotency-Key") }
        var (data, response) = try await session.data(for: request)
        guard response is HTTPURLResponse else { throw APIError.invalidResponse }
        if (response as? HTTPURLResponse)?.statusCode == 401 && authenticated {
            guard try await refreshSession(), let tokens = await vault.load() else { throw APIError.unauthorized }
            request.setValue("Bearer \(tokens.accessToken)", forHTTPHeaderField: "Authorization")
            (data, response) = try await session.data(for: request)
        }
        guard let http = response as? HTTPURLResponse else { throw APIError.invalidResponse }
        if http.statusCode == 401 { throw APIError.unauthorized }
        guard (200..<300).contains(http.statusCode) else { throw APIError.server(http.statusCode, String(data: data.prefix(500), encoding: .utf8) ?? "") }
        return try JSONDecoder.api.decode(Response.self, from: data)
    }

    func sendNoContent<Body: Encodable>(_ path: String, method: String, body: Body? = nil, authenticated: Bool = true, idempotencyKey: String? = nil) async throws {
        var request = URLRequest(url: try url(path)); request.httpMethod = method; request.timeoutInterval = 30
        if let body { request.httpBody = try JSONEncoder.api.encode(body); request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        if authenticated {
            guard let tokens = await vault.load() else { throw APIError.unauthorized }
            request.setValue("Bearer \(tokens.accessToken)", forHTTPHeaderField: "Authorization")
        }
        let mutationKey = idempotencyKey ?? UUID().uuidString
        request.setValue(mutationKey, forHTTPHeaderField: "Idempotency-Key")
        var (data, response) = try await session.data(for: request)
        if (response as? HTTPURLResponse)?.statusCode == 401 && authenticated {
            guard try await refreshSession(), let refreshed = await vault.load() else { throw APIError.unauthorized }
            request.setValue("Bearer \(refreshed.accessToken)", forHTTPHeaderField: "Authorization")
            (data, response) = try await session.data(for: request)
        }
        guard let http = response as? HTTPURLResponse else { throw APIError.invalidResponse }
        if http.statusCode == 401 { throw APIError.unauthorized }
        guard (200..<300).contains(http.statusCode) else { throw APIError.server(http.statusCode, String(data: data.prefix(500), encoding: .utf8) ?? "") }
    }

    func downloadAuthenticated(_ path: String, filename: String) async throws -> URL {
        var request = URLRequest(url: try url(path)); request.httpMethod = "GET"; request.timeoutInterval = 120
        guard let tokens = await vault.load() else { throw APIError.unauthorized }
        request.setValue("Bearer \(tokens.accessToken)", forHTTPHeaderField: "Authorization")
        var (temporary, response) = try await session.download(for: request)
        if (response as? HTTPURLResponse)?.statusCode == 401 {
            guard try await refreshSession(), let refreshed = await vault.load() else { throw APIError.unauthorized }
            request.setValue("Bearer \(refreshed.accessToken)", forHTTPHeaderField: "Authorization")
            (temporary, response) = try await session.download(for: request)
        }
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else { throw APIError.invalidResponse }
        let safeName = filename.replacingOccurrences(of: "/", with: "-")
        let destination = FileManager.default.temporaryDirectory.appendingPathComponent(safeName)
        try? FileManager.default.removeItem(at: destination)
        try FileManager.default.moveItem(at: temporary, to: destination)
        try? FileManager.default.setAttributes([.protectionKey: FileProtectionType.completeUntilFirstUserAuthentication], ofItemAtPath: destination.path)
        return destination
    }

    /// Uploads bytes only to a short-lived object-store URL returned by the API.
    /// Authentication headers are deliberately omitted because the signature is
    /// the authorization and forwarding the bearer token would leak it to the
    /// storage origin.
    func upload(_ data: Data, to signedURL: URL, contentType: String) async throws {
        var request = URLRequest(url: signedURL)
        request.httpMethod = "PUT"
        request.timeoutInterval = 120
        request.setValue(contentType, forHTTPHeaderField: "Content-Type")
        let (_, response) = try await session.upload(for: request, from: data)
        guard let http = response as? HTTPURLResponse,
              (200..<300).contains(http.statusCode) else { throw APIError.invalidResponse }
    }

    func url(_ path: String) throws -> URL {
        guard !path.contains(".."), let url = URL(string: path, relativeTo: baseURL)?.absoluteURL else { throw APIError.invalidConfiguration }
        return url
    }

    nonisolated func publicURL(_ path: String?) -> URL? {
        guard let path, !path.contains("..") else { return nil }
        return URL(string: path, relativeTo: baseURL)?.absoluteURL
    }

    private func refreshSession() async throws -> Bool {
        guard let current = await vault.load(), current.refreshExpiresAt > .now else { await vault.clear(); return false }
        let attemptKey = current.refreshAttemptKey ?? UUID().uuidString
        if current.refreshAttemptKey == nil {
            var pending = current; pending.refreshAttemptKey = attemptKey; try await vault.save(pending)
        }
        var request = URLRequest(url: try url(OpenAPIPaths.refreshSession)); request.httpMethod = "POST"; request.setValue("application/json", forHTTPHeaderField: "Content-Type"); request.setValue(attemptKey, forHTTPHeaderField: "Idempotency-Key"); request.httpBody = try JSONEncoder.api.encode(["refresh_token": current.refreshToken])
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else { await vault.clear(); return false }
        struct Refreshed: Decodable {
            let accessToken, refreshToken: String; let accessExpiresAt, refreshExpiresAt: Date
            enum CodingKeys: String, CodingKey { case accessToken = "access_token", refreshToken = "refresh_token", accessExpiresAt = "access_token_expires_at", refreshExpiresAt = "refresh_token_expires_at" }
        }
        let refreshed = try JSONDecoder.api.decode(Refreshed.self, from: data)
        try await vault.save(.init(accessToken: refreshed.accessToken, refreshToken: refreshed.refreshToken, accessExpiresAt: refreshed.accessExpiresAt, refreshExpiresAt: refreshed.refreshExpiresAt))
        return true
    }
}
