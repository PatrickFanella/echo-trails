import AuthenticationServices
import CoreLocation
import CryptoKit
import EchoTrailsShell
import Foundation
import SwiftData
import UIKit
import UserNotifications

enum APNsTokenPrivacy {
    static func idempotencyKey(for rawToken: String) -> String {
        let digest = SHA256.hash(data: Data(rawToken.utf8))
        return "register-device-" + digest.map { String(format: "%02x", $0) }.joined()
    }
}

struct AuthStartResponse: Decodable { let requestID: String; let expiresAt: Date; enum CodingKeys: String, CodingKey { case requestID = "request_id", expiresAt = "expires_at" } }
struct AuthTokensResponse: Decodable {
    let userID, sessionID, accessToken, refreshToken: String
    let accessExpiresAt, refreshExpiresAt: Date
    enum CodingKeys: String, CodingKey { case userID = "user_id", sessionID = "session_id", accessToken = "access_token", refreshToken = "refresh_token", accessExpiresAt = "access_token_expires_at", refreshExpiresAt = "refresh_token_expires_at" }
}
struct PlaceDTO: Codable, Identifiable, Hashable { let id, name, state: String; let latitude, longitude: Double?; let radiusMeters: Double?; let category: String?; let sensitive: Bool?; enum CodingKeys: String, CodingKey { case id, name, state, latitude, longitude, category, sensitive; case radiusMeters = "radius_meters" } }
struct PlaceSearchResponse: Decodable { let places: [PlaceDTO] }
struct CoordinateDTO: Codable, Hashable { let lat, lon: Double }
struct StitchSourceDTO: Codable, Hashable { let echoID, title, authorHandle: String; let quotedReplyID, quoteSnapshotURL: String?; let quoteUnavailable: Bool?; enum CodingKeys: String, CodingKey { case title; case echoID = "echo_id", authorHandle = "author_handle", quotedReplyID = "quoted_reply_id", quoteSnapshotURL = "quote_snapshot_url", quoteUnavailable = "quote_unavailable" } }
/// Public, processed media derivatives. Originals and private scanner evidence are never exposed here.
struct MediaAssetDTO: Codable, Hashable, Identifiable {
    let id, mediaType, status: String
    let publicURL, thumbnailURL: String?
    let durationSeconds: Double?
    let transcript: String?
    enum CodingKeys: String, CodingKey { case id, mediaType = "media_type", status, transcript; case publicURL = "public_url", thumbnailURL = "thumbnail_url", durationSeconds = "duration_seconds" }
}
struct EchoDTO: Codable, Identifiable, Hashable {
    let id, authorHandle, placeID, contentMode, title, status: String
    let bodyText, captionText: String?
    let publicLocation: CoordinateDTO
    let createdAt, updatedAt, publishedAt: Date?
    let stitchSource: StitchSourceDTO?
    let media: [MediaAssetDTO]?
    enum CodingKeys: String, CodingKey { case id, title, status, media; case authorHandle = "author_handle", placeID = "place_id", contentMode = "content_mode", bodyText = "body_text", captionText = "caption_text", publicLocation = "public_location", createdAt = "created_at", updatedAt = "updated_at", publishedAt = "published_at", stitchSource = "stitch_source" }
}
struct DiscoveryResultDTO: Decodable, Identifiable { let echo: EchoDTO; let score: Double; let label: String; var id: String { echo.id } }
struct SearchResponseDTO: Decodable {
    let places: [PlaceDTO]; let echoes: [DiscoveryResultDTO]; let nextCursor: String?
    enum CodingKeys: String, CodingKey { case places, echoes; case nextCursor = "next_cursor" }
}
struct NotableResponseDTO: Decodable { let echoes: [DiscoveryResultDTO] }
struct NotificationDTO: Decodable, Identifiable { let id, kind, preview: String; let targetID: String; let createdAt: Date; let readAt: Date?; enum CodingKeys: String, CodingKey { case id, kind, preview; case targetID = "target_id", createdAt = "created_at", readAt = "read_at" } }
struct NotificationPageDTO: Decodable { let notifications: [NotificationDTO]; let nextCursor: String?; enum CodingKeys: String, CodingKey { case notifications; case nextCursor = "next_cursor" } }
struct AppealDTO: Codable, Identifiable, Equatable {
    let id, moderationActionID, body, status: String
    let createdAt: Date
    var statusPresentation: AppealStatus { AppealStatus(rawValue: status) ?? .open }
    enum CodingKeys: String, CodingKey { case id, body, status; case moderationActionID = "moderation_action_id", createdAt = "created_at" }
}
struct AppealPageDTO: Decodable { let appeals: [AppealDTO]; let nextCursor: String?; enum CodingKeys: String, CodingKey { case appeals; case nextCursor = "next_cursor" } }
struct AppealMutationDTO: Encodable {
    let moderationActionID: String?
    let body: String
    enum CodingKeys: String, CodingKey { case body; case moderationActionID = "moderation_action_id" }
}
struct ModerationActionReference: Identifiable, Equatable { let id: String }
struct PlaceSubscriptionDTO: Decodable, Equatable {
    let placeID: String
    let notificationsEnabled: Bool
    let createdAt: Date
    enum CodingKeys: String, CodingKey { case placeID = "place_id", notificationsEnabled = "notifications_enabled", createdAt = "created_at" }
}
struct SubscriptionsDTO: Decodable {
    let subscriptions: [PlaceSubscriptionDTO]
    let nextCursor: String?
    enum CodingKeys: String, CodingKey { case subscriptions; case nextCursor = "next_cursor" }
}
struct NotificationPreferencesDTO: Decodable, Equatable {
    let directActivityEnabled, placeAlertsEnabled, moderationEnabled, uploadStatusEnabled: Bool
    let updatedAt: Date
    enum CodingKeys: String, CodingKey { case directActivityEnabled = "direct_activity_enabled", placeAlertsEnabled = "place_alerts_enabled", moderationEnabled = "moderation_enabled", uploadStatusEnabled = "upload_status_enabled", updatedAt = "updated_at" }
}
struct NotificationPreferencesPatchDTO: Encodable {
    let directActivityEnabled, placeAlertsEnabled, moderationEnabled, uploadStatusEnabled: Bool
    enum CodingKeys: String, CodingKey { case directActivityEnabled = "direct_activity_enabled", placeAlertsEnabled = "place_alerts_enabled", moderationEnabled = "moderation_enabled", uploadStatusEnabled = "upload_status_enabled" }
}
struct DeviceRegistrationDTO: Decodable {
    let id: String
}
struct MeDTO: Decodable {
    let userID, handle, displayName: String
    let bio, avatarURL: String?
    enum CodingKeys: String, CodingKey {
        case userID = "user_id", handle, bio
        case displayName = "display_name", avatarURL = "avatar_url"
    }
}
struct PublicProfileDTO: Decodable {
    let handle, displayName: String
    let bio, avatarURL: String?
    enum CodingKeys: String, CodingKey {
        case handle, bio
        case displayName = "display_name", avatarURL = "avatar_url"
    }
}
struct PublicProfileEchoesDTO: Decodable {
    let echoes: [EchoDTO]
    let nextCursor: String?
    enum CodingKeys: String, CodingKey { case echoes; case nextCursor = "next_cursor" }
}

enum AppGate { case loading, authentication, invite, consent, profile, ready }

struct AccountAccessStateDTO: Decodable {
    let betaAccessAccepted, consentAccepted, profileConfigured, quoteWarningAcknowledged: Bool
    enum CodingKeys: String, CodingKey {
        case betaAccessAccepted = "beta_access_accepted"
        case consentAccepted = "consent_accepted"
        case profileConfigured = "profile_configured"
        case quoteWarningAcknowledged = "quote_warning_acknowledged"
    }
}

@MainActor final class AppModel: ObservableObject {
    @Published var gate: AppGate = .loading
    @Published var places: [PlaceDTO] = []
    @Published var discovery: [DiscoveryResultDTO] = []
    @Published var searchResults: SearchResponseDTO?
    @Published var searchError: String?
    @Published var isSearching = false
    @Published var searchNextCursor: String?
    @Published var activity: [NotificationDTO] = []
    @Published private(set) var activityNextCursor: String?
    @Published private(set) var isLoadingActivity = false
    @Published var profile: MeDTO?
    @Published var errorMessage: String?
    @Published var isWorking = false
    @Published var deepLink: URL?
    @Published var linkedEcho: EchoDTO?
    @Published var linkedPlace: PlaceDTO?
    @Published var appealAction: ModerationActionReference?
    @Published private(set) var subscriptionsByPlaceID: [String: PlaceSubscriptionDTO] = [:]
    @Published private(set) var notificationPreferences: NotificationPreferencesDTO?
    @Published private(set) var quoteWarningAcknowledged = false
    var subscribedPlaceIDs: Set<String> { Set(subscriptionsByPlaceID.keys) }

    let vault = SessionVault()
    let client: APIClient
    private let defaults = UserDefaults.standard
    let modelContainer: ModelContainer
    private var uploadObserver: NSObjectProtocol?
    private var activeSearchID: UUID?
    private var isProcessingOutbox = false

    init(modelContainer: ModelContainer) {
        self.modelContainer = modelContainer
        let configured = (Bundle.main.object(forInfoDictionaryKey: "ECHOTRAILS_API_BASE_URL") as? String)?.trimmingCharacters(in: .whitespacesAndNewlines)
        #if DEBUG
        let resolved = URL(string: configured?.isEmpty == false ? configured! : "http://127.0.0.1:8080")!
        #else
        guard let value = configured, let resolved = URL(string: value), resolved.scheme == "https", resolved.host?.hasSuffix(".example") != true else {
            fatalError("Release builds require a real HTTPS ECHOTRAILS_API_BASE_URL build setting")
        }
        #endif
        self.client = APIClient(baseURL: resolved, vault: vault)
        uploadObserver = NotificationCenter.default.addObserver(forName: .backgroundUploadFinished, object: nil, queue: .main) { [weak self] notification in
            Task { @MainActor in await self?.handleUploadCompletion(notification) }
        }
        NotificationCenter.default.addObserver(forName: .didReceiveDeepLink, object: nil, queue: .main) { [weak self] notification in
            if let url = notification.object as? URL { Task { @MainActor in await self?.handleDeepLink(url) } }
        }
    }

    func bootstrap() async {
        guard let tokens = await vault.load(), tokens.refreshExpiresAt > .now else {
            gate = .authentication
            return
        }
        hydrateCachedContent()
        do {
            let state: AccountAccessStateDTO = try await client.get(OpenAPIPaths.getAccountBootstrap)
            defaults.set(state.betaAccessAccepted, forKey: "betaAccessAccepted")
            defaults.set(state.consentAccepted, forKey: "consentAccepted")
            defaults.set(state.profileConfigured, forKey: "profileConfigured")
            defaults.set(state.quoteWarningAcknowledged, forKey: "quoteWarningAcknowledged")
            quoteWarningAcknowledged = state.quoteWarningAcknowledged
            await enterGate(for: state)
        } catch APIError.unauthorized {
            await vault.clear()
            gate = .authentication
        } catch let error as URLError {
            // Preserve a previously server-confirmed ready session for the
            // offline experience. Setup gates never advance while offline.
            errorMessage = error.localizedDescription
            let cached = AccountAccessStateDTO(
                betaAccessAccepted: defaults.bool(forKey: "betaAccessAccepted"),
                consentAccepted: defaults.bool(forKey: "consentAccepted"),
                profileConfigured: defaults.bool(forKey: "profileConfigured"),
                quoteWarningAcknowledged: defaults.bool(forKey: "quoteWarningAcknowledged")
            )
            await enterGate(for: cached)
        } catch {
            errorMessage = error.localizedDescription
            gate = .loading
        }
    }

    private func enterGate(for state: AccountAccessStateDTO) async {
        gate = !state.betaAccessAccepted ? .invite : (!state.consentAccepted ? .consent : (!state.profileConfigured ? .profile : .ready))
        if gate == .ready {
            await refreshAll(); await recoverBackgroundUploads(); await processOutbox(); await retryPendingAvatarSelection()
        }
    }

    func startEmail(_ email: String) async -> String? {
        await perform { let response: AuthStartResponse = try await client.send(OpenAPIPaths.startEmailAuth, method: "POST", body: ["email": email], authenticated: false); return response.requestID }
    }

    func verifyEmail(requestID: String, code: String) async {
        let response: AuthTokensResponse? = await perform { try await client.send(OpenAPIPaths.verifyEmailAuth, method: "POST", body: ["request_id": requestID, "code": code, "device_name": UIDevice.current.name], authenticated: false) }
        if let response { await acceptTokens(response) }
    }

    func completeAppleAuthorization(_ authorization: ASAuthorization) async {
        guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential, let data = credential.identityToken, let token = String(data: data, encoding: .utf8) else { errorMessage = "Apple did not return an identity token."; return }
        let response: AuthTokensResponse? = await perform { try await client.send(OpenAPIPaths.signInWithApple, method: "POST", body: ["identity_token": token], authenticated: false) }
        if let response { await acceptTokens(response) }
    }

    private func acceptTokens(_ response: AuthTokensResponse) async {
        try? await vault.save(.init(accessToken: response.accessToken, refreshToken: response.refreshToken, accessExpiresAt: response.accessExpiresAt, refreshExpiresAt: response.refreshExpiresAt))
        await bootstrap()
    }

    func claimInvite(code: String) async {
        let _: [String: String]? = await perform { try await client.send(OpenAPIPaths.evaluateBetaAccess, method: "POST", body: ["code": code, "city": "Downers Grove"]) }
        if errorMessage == nil { defaults.set(true, forKey: "betaAccessAccepted"); gate = .consent }
    }

    func acceptConsent() async {
        struct Consent: Decodable { let termsVersion, privacyVersion: String; enum CodingKeys: String, CodingKey { case termsVersion = "terms_version", privacyVersion = "privacy_version" } }
        guard let current: Consent = await perform({ try await client.get(OpenAPIPaths.getCurrentConsent, authenticated: false) }) else { return }
        let succeeded = await performNoContent { try await client.sendNoContent(OpenAPIPaths.acceptConsent, method: "POST", body: ["terms_version": current.termsVersion, "privacy_version": current.privacyVersion]) }
        if succeeded { defaults.set(true, forKey: "consentAccepted"); gate = defaults.bool(forKey: "profileConfigured") ? .ready : .profile; if gate == .ready { await refreshAll() } }
    }

    func saveProfile(handle: String, displayName: String, bio: String) async {
        struct ProfileRequest: Encodable { let handle, displayName, bio: String; enum CodingKeys: String, CodingKey { case handle, bio; case displayName = "display_name" } }
        guard let response: MeDTO = await perform({ try await client.send(OpenAPIPaths.updateMe, method: "PATCH", body: ProfileRequest(handle: handle, displayName: displayName, bio: bio), idempotencyKey: UUID().uuidString) }) else { return }
        profile = response; defaults.set(true, forKey: "profileConfigured"); gate = .ready; await refreshAll()
    }

    enum AvatarResult: Equatable { case selected, pendingReview }

    func uploadAvatar(data: Data, contentType: String) async throws -> AvatarResult {
        guard data.count > 0, data.count <= 10 * 1024 * 1024,
              ["image/jpeg", "image/png", "image/heic"].contains(contentType) else {
            throw APIError.invalidResponse
        }
        struct Request: Encodable {
            let contentType: String; let byteSize: Int
            enum CodingKeys: String, CodingKey { case contentType = "content_type", byteSize = "byte_size" }
        }
        struct Selection: Encodable {
            let mediaAssetID: String
            enum CodingKeys: String, CodingKey { case mediaAssetID = "media_asset_id" }
        }
        let key = UUID().uuidString
        let upload: UploadResponseDTO = try await client.send(
            OpenAPIPaths.createAvatarUpload, method: "POST",
            body: Request(contentType: contentType, byteSize: data.count), idempotencyKey: key
        )
        guard let signedURL = URL(string: upload.uploadURL) else { throw APIError.invalidResponse }
        try await client.upload(data, to: signedURL, contentType: contentType)
        let _: UploadResponseDTO = try await client.send(
            OpenAPIPaths.completeMediaUpload(uploadId: upload.uploadID), method: "POST", body: EmptyRequest(),
            idempotencyKey: "avatar-complete-\(upload.uploadID)"
        )
        defaults.set(upload.mediaAssetID, forKey: "pendingAvatarMediaAssetID")
        do {
            let selected: MeDTO = try await client.send(
                OpenAPIPaths.selectAvatar, method: "PUT", body: Selection(mediaAssetID: upload.mediaAssetID),
                idempotencyKey: "avatar-select-\(upload.mediaAssetID)"
            )
            profile = selected
            defaults.removeObject(forKey: "pendingAvatarMediaAssetID")
            return .selected
        } catch APIError.server(409, _) {
            return .pendingReview
        }
    }

    func retryPendingAvatarSelection() async {
        guard let assetID = defaults.string(forKey: "pendingAvatarMediaAssetID"), !assetID.isEmpty else { return }
        struct Selection: Encodable {
            let mediaAssetID: String
            enum CodingKeys: String, CodingKey { case mediaAssetID = "media_asset_id" }
        }
        do {
            let selected: MeDTO = try await client.send(
                OpenAPIPaths.selectAvatar, method: "PUT", body: Selection(mediaAssetID: assetID),
                idempotencyKey: "avatar-select-\(assetID)"
            )
            profile = selected
            defaults.removeObject(forKey: "pendingAvatarMediaAssetID")
        } catch APIError.server(409, _) {
            // Fail closed: moderation has not approved a safe derivative yet.
        } catch { errorMessage = error.localizedDescription }
    }

    func removeAvatar() async -> Bool {
        do {
            let response: MeDTO = try await client.send(
                OpenAPIPaths.clearAvatar, method: "DELETE", body: Optional<EmptyRequest>.none,
                idempotencyKey: UUID().uuidString
            )
            profile = response
            defaults.removeObject(forKey: "pendingAvatarMediaAssetID")
            return true
        } catch { errorMessage = error.localizedDescription; return false }
    }

    func refreshAll() async { async let p: Void = loadPlaces(); async let d: Void = loadDiscovery(); async let a: Void = loadActivity(); async let m: Void = loadProfile(); async let s: Void = loadSubscriptions(); async let n: Void = loadNotificationPreferences(); _ = await (p, d, a, m, s, n) }
    func loadPlaces(query: String = "") async {
        let encoded = query.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? ""
        if let response: PlaceSearchResponse = await perform({ try await client.get("\(OpenAPIPaths.searchPlaces)?q=\(encoded)", authenticated: false) }) {
            places = response.places.filter { $0.state == "active" }; cache(response.places)
        }
    }
    func loadDiscovery() async { if let response: NotableResponseDTO = await perform({ try await client.get("\(OpenAPIPaths.getNotableEchoes)?limit=30") }) { discovery = response.echoes; cache(response.echoes) } }
    func deleteEcho(id: String) async -> Bool {
        guard let safe = ContractPath.safeSegment(id) else { return false }
        let deleted = await performNoContent {
            try await client.sendNoContent(OpenAPIPaths.deleteEcho(echoId: safe), method: "DELETE", body: Optional<EmptyRequest>.none, idempotencyKey: "delete-echo-\(safe)")
        }
        guard deleted else { return false }
        discovery.removeAll { $0.echo.id == id }
        searchResults = searchResults.map { SearchResponseDTO(places: $0.places, echoes: $0.echoes.filter { $0.echo.id != id }, nextCursor: $0.nextCursor) }
        if linkedEcho?.id == id { linkedEcho = nil }
        let context = modelContainer.mainContext
        let cachedEchoes = (try? context.fetch(FetchDescriptor<CachedEcho>())) ?? []
        cachedEchoes.filter { $0.id == id }.forEach(context.delete)
        let uploads = (try? context.fetch(FetchDescriptor<CachedUpload>())) ?? []
        for upload in uploads where upload.echoID == id {
            try? FileManager.default.removeItem(atPath: upload.localFilePath)
            context.delete(upload)
        }
        try? context.save()
        return true
    }
    func clearSearch() { activeSearchID = nil; searchResults = nil; searchNextCursor = nil; searchError = nil; isSearching = false }
    func search(_ request: DiscoverySearchQuery, reset: Bool = true) async {
        guard let path = request.apiPath else { clearSearch(); return }
        let searchID = UUID()
        activeSearchID = searchID
        isSearching = true; searchError = nil
        defer { if activeSearchID == searchID { isSearching = false } }
        do {
            // Search stays authenticated so server-side hides, blocks, and mutes are enforced.
            let response: SearchResponseDTO = try await client.get(path)
            guard !Task.isCancelled, activeSearchID == searchID else { return }
            if reset || searchResults == nil {
                searchResults = response
            } else if let current = searchResults {
                let placeIDs = Set(current.places.map(\.id))
                let echoIDs = Set(current.echoes.map { $0.echo.id })
                searchResults = SearchResponseDTO(
                    places: current.places + response.places.filter { !placeIDs.contains($0.id) },
                    echoes: current.echoes + response.echoes.filter { !echoIDs.contains($0.echo.id) },
                    nextCursor: response.nextCursor
                )
            }
            searchNextCursor = response.nextCursor?.isEmpty == false ? response.nextCursor : nil
        } catch is CancellationError {
            return
        } catch {
            guard !Task.isCancelled, activeSearchID == searchID else { return }
            searchResults = SearchResponseDTO(places: [], echoes: [], nextCursor: nil)
            searchNextCursor = nil
            searchError = error.localizedDescription
        }
    }
    func loadActivity(reset: Bool = true) async {
        guard !isLoadingActivity else { return }
        let cursor = reset ? nil : activityNextCursor
        if !reset && cursor == nil { return }
        guard let path = ActivityPageRequest(cursor: cursor, limit: 50).apiPath else { return }
        isLoadingActivity = true; defer { isLoadingActivity = false }
        do {
            let response: NotificationPageDTO = try await client.get(path)
            if reset { activity = response.notifications }
            else {
                let existing = Set(activity.map(\.id))
                activity.append(contentsOf: response.notifications.filter { !existing.contains($0.id) })
            }
            activityNextCursor = response.nextCursor?.isEmpty == false ? response.nextCursor : nil
        } catch { errorMessage = error.localizedDescription }
    }
    func loadProfile() async { if let response: MeDTO = await perform({ try await client.get(OpenAPIPaths.getMe) }) { profile = response; cache(response) } }
    func loadSubscriptions() async {
        do {
            var values: [PlaceSubscriptionDTO] = []
            var cursor: String?
            var seenCursors = Set<String>()
            repeat {
                var path = PlaceSubscriptionDescriptor.listAPIPath + "?limit=50"
                if let cursor { path += "&cursor=" + (cursor.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? "") }
                let response: SubscriptionsDTO = try await client.get(path)
                values.append(contentsOf: response.subscriptions)
                if let next = response.nextCursor, !next.isEmpty, seenCursors.insert(next).inserted {
                    cursor = next
                } else {
                    cursor = nil
                }
            } while cursor != nil
            subscriptionsByPlaceID = Dictionary(uniqueKeysWithValues: values.map { ($0.placeID, $0) })
        } catch { errorMessage = error.localizedDescription }
    }
    func loadNotificationPreferences() async {
        do {
            let response: NotificationPreferencesDTO = try await client.get(OpenAPIPaths.getNotificationPreferences)
            notificationPreferences = response
        } catch { errorMessage = error.localizedDescription }
    }
    func subscribe(placeID: String) async {
        guard let path = PlaceSubscriptionDescriptor(placeID: placeID).resourceAPIPath else { return }
        do {
            if subscriptionsByPlaceID[placeID] != nil {
                try await client.sendNoContent(path, method: "DELETE", body: Optional<EmptyRequest>.none, idempotencyKey: UUID().uuidString)
                subscriptionsByPlaceID.removeValue(forKey: placeID)
            } else {
                let subscription: PlaceSubscriptionDTO = try await client.send(path, method: "PUT", body: ["notifications_enabled": true], idempotencyKey: UUID().uuidString)
                subscriptionsByPlaceID[placeID] = subscription
            }
        } catch { errorMessage = error.localizedDescription }
    }
    func setPlaceNotifications(placeID: String, enabled: Bool) async {
        guard subscriptionsByPlaceID[placeID] != nil, let path = PlaceSubscriptionDescriptor(placeID: placeID).resourceAPIPath else { return }
        do {
            let subscription: PlaceSubscriptionDTO = try await client.send(path, method: "PUT", body: ["notifications_enabled": enabled], idempotencyKey: UUID().uuidString)
            subscriptionsByPlaceID[placeID] = subscription
        } catch { errorMessage = error.localizedDescription }
    }
    func setMute(target: NotificationMuteTarget, id: String, muted: Bool) async {
        guard let path = NotificationMuteDescriptor(target: target, targetID: id).apiPath else { return }
        do { try await client.sendNoContent(path, method: muted ? "PUT" : "DELETE", body: Optional<EmptyRequest>.none, idempotencyKey: UUID().uuidString) }
        catch { errorMessage = error.localizedDescription }
    }
    func saveNotificationPreferences(direct: Bool, places: Bool, moderation: Bool, uploads: Bool) async -> Bool {
        do {
            let response: NotificationPreferencesDTO = try await client.send(OpenAPIPaths.patchNotificationPreferences, method: "PATCH", body: NotificationPreferencesPatchDTO(directActivityEnabled: direct, placeAlertsEnabled: places, moderationEnabled: moderation, uploadStatusEnabled: uploads), idempotencyKey: UUID().uuidString)
            notificationPreferences = response
            return true
        } catch { errorMessage = error.localizedDescription; return false }
    }
    func markActivityRead(_ id: String) async {
        guard let safe = ContractPath.safeSegment(id) else { return }
        _ = await performNoContent { try await client.sendNoContent(OpenAPIPaths.markNotificationsRead, method: "PATCH", body: ["notification_ids": [safe]], idempotencyKey: "notification-read-\(safe)") }
        if let index = activity.firstIndex(where: { $0.id == id }) { let item = activity[index]; activity[index] = NotificationDTO(id: item.id, kind: item.kind, preview: item.preview, targetID: item.targetID, createdAt: item.createdAt, readAt: .now) }
    }
    func openActivity(_ item: NotificationDTO) async {
        await markActivityRead(item.id)
        guard let targetID = ContractPath.safeSegment(item.targetID) else { return }
        switch ActivityTargetKind.resolve(notificationKind: item.kind) {
        case .echo:
            if let response: EchoDTO = await perform({ try await client.get(OpenAPIPaths.getEcho(echoId: targetID)) }) { linkedEcho = response }
        case .place:
            if let response: PlaceDTO = await perform({ try await client.get(OpenAPIPaths.getPlace(placeId: targetID), authenticated: false) }) { linkedPlace = response }
        case .moderationAction:
            appealAction = ModerationActionReference(id: targetID)
        case .none:
            break
        }
    }
    func registerPushToken(_ token: String) async {
        if let response: DeviceRegistrationDTO = await perform({
            try await client.send(OpenAPIPaths.registerDevice, method: "POST", body: ["token": token, "environment": pushEnvironment(), "app_version": Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "", "locale": Locale.current.identifier, "timezone": TimeZone.current.identifier], idempotencyKey: APNsTokenPrivacy.idempotencyKey(for: token))
        }) {
            defaults.set(response.id, forKey: "registeredPushDeviceID")
        }
    }
    func signOut() async {
        let tokens = await vault.load()
        if let deviceID = defaults.string(forKey: "registeredPushDeviceID"), let safeID = ContractPath.safeSegment(deviceID) {
            try? await client.sendNoContent(OpenAPIPaths.deleteDevice(deviceId: safeID), method: "DELETE", body: Optional<EmptyRequest>.none, idempotencyKey: "disable-device-\(safeID)")
        }
        if let refreshToken = tokens?.refreshToken {
            try? await client.sendNoContent(
                OpenAPIPaths.logoutSession, method: "POST", body: ["refresh_token": refreshToken],
                authenticated: false, idempotencyKey: UUID().uuidString
            )
        }
        await clearLocalSession()
    }

    func completeAccountDeletionLocally() async {
        await clearLocalSession()
    }

    private func clearLocalSession() async {
        defaults.removeObject(forKey: "registeredPushDeviceID")
        await vault.clear(); defaults.removeObject(forKey: "betaAccessAccepted"); defaults.removeObject(forKey: "consentAccepted"); defaults.removeObject(forKey: "profileConfigured"); defaults.removeObject(forKey: "quoteWarningAcknowledged"); quoteWarningAcknowledged = false; subscriptionsByPlaceID = [:]; activity = []; activityNextCursor = nil; profile = nil; gate = .authentication
    }

    func handleDeepLink(_ url: URL) async {
        deepLink = url
        if url.path == OpenAPIPaths.verifyEmailAuth,
           let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
           let requestID = components.queryItems?.first(where: { $0.name == "request_id" })?.value,
           let code = components.queryItems?.first(where: { $0.name == "code" })?.value,
           !requestID.isEmpty, !code.isEmpty {
            await verifyEmail(requestID: requestID, code: code)
            return
        }
        let parts = url.pathComponents.filter { $0 != "/" }
        guard parts.count >= 2 else { return }
        if parts[0] == "echoes", let response: EchoDTO = await perform({ try await client.get(OpenAPIPaths.getEcho(echoId: parts[1])) }) { linkedEcho = response }
        if parts[0] == "places", let response: PlaceDTO = await perform({ try await client.get(OpenAPIPaths.getPlace(placeId: parts[1]), authenticated: false) }) { linkedPlace = response }
        if parts[0] == "moderation", let actionID = ContractPath.safeSegment(parts[1]) { appealAction = ModerationActionReference(id: actionID) }
    }

    func registerUpload(_ upload: CachedUpload) {
        modelContainer.mainContext.insert(upload); try? modelContainer.mainContext.save()
    }

    /// Commits the complete pre-draft intent before any server mutation. A
    /// creation token is deliberately duplicated in the indexed outbox column
    /// and the encoded transaction: this makes authorization capture auditable
    /// without ever reacquiring location after the user has left the Place.
    func enqueueComposer(_ transaction: ComposerTransactionPayload) throws {
        let body = try JSONEncoder.api.encode(transaction)
        let endpoint = transaction.sourceEchoID.map { OpenAPIPaths.createEchoStitch(echoId: $0) } ?? OpenAPIPaths.createEchoDraft
        let item = DurableOutboxItem(
            operation: "composer_transaction", endpoint: endpoint, body: body,
            creationToken: transaction.creationToken,
            filePaths: transaction.attachments.map(\.localFilePath)
        )
        modelContainer.mainContext.insert(item)
        try modelContainer.mainContext.save()
    }

    private func handleUploadCompletion(_ notification: Notification) async {
        guard let uploadID = notification.userInfo?["upload_id"] as? String, !uploadID.isEmpty else { return }
        let success = notification.userInfo?["success"] as? Bool == true
        let descriptor = FetchDescriptor<CachedUpload>(predicate: #Predicate { $0.uploadID == uploadID })
        guard let upload = try? modelContainer.mainContext.fetch(descriptor).first else { return }
        if !success {
            upload.status = "retry_required"; upload.updatedAt = .now
            let payload = (try? JSONEncoder.api.encode(["upload_id": uploadID])) ?? Data()
            enqueueOutbox(operation: "retry_media_upload", endpoint: OpenAPIPaths.renewMediaUpload(uploadId: uploadID), body: payload, filePaths: [upload.localFilePath])
            try? modelContainer.mainContext.save(); return
        }
        _ = await completeUploadedAsset(upload, queueOnFailure: true)
    }

    @discardableResult private func completeUploadedAsset(_ upload: CachedUpload, queueOnFailure: Bool) async -> Bool {
        do {
            let _: UploadResponseDTO = try await client.send(OpenAPIPaths.completeMediaUpload(uploadId: upload.uploadID), method: "POST", body: EmptyRequest(), idempotencyKey: "upload-complete-\(upload.uploadID)")
        } catch {
            if queueOnFailure { enqueueOutbox(operation: "complete_media_upload", endpoint: OpenAPIPaths.completeMediaUpload(uploadId: upload.uploadID), body: Data(), filePaths: [upload.localFilePath]) }
            try? modelContainer.mainContext.save()
            return false
        }
        upload.status = "uploaded"; upload.updatedAt = .now; try? modelContainer.mainContext.save()
        let echoID = upload.echoID
        let siblings = (try? modelContainer.mainContext.fetch(FetchDescriptor<CachedUpload>(predicate: #Predicate { $0.echoID == echoID }))) ?? []
        if !siblings.isEmpty && siblings.allSatisfy({ $0.status == "uploaded" }) {
            for sibling in siblings {
                try? FileManager.default.removeItem(atPath: sibling.localFilePath)
                sibling.status = "processing"; sibling.updatedAt = .now
            }
            try? modelContainer.mainContext.save()
            do {
                let _: EchoDTO = try await client.send(OpenAPIPaths.submitEcho(echoId: echoID), method: "POST", body: EmptyRequest(), idempotencyKey: "echo-submit-\(echoID)")
            } catch {
                enqueueOutbox(operation: "submit_echo", endpoint: OpenAPIPaths.submitEcho(echoId: echoID), body: Data())
                try? modelContainer.mainContext.save()
            }
        }
        return true
    }

    func retryOutboxItem(_ id: UUID) async {
        let descriptor = FetchDescriptor<DurableOutboxItem>(predicate: #Predicate { $0.id == id })
        guard let item = try? modelContainer.mainContext.fetch(descriptor).first else { return }
        item.state = "queued"; item.nextAttemptAt = .now; item.lastError = nil
        try? modelContainer.mainContext.save()
        await processOutbox()
    }

    private func enqueueOutbox(operation: String, endpoint: String, body: Data, filePaths: [String] = []) {
        let existing = (try? modelContainer.mainContext.fetch(FetchDescriptor<DurableOutboxItem>())) ?? []
        guard !existing.contains(where: { $0.operation == operation && $0.endpoint == endpoint && $0.state != "completed" }) else { return }
        modelContainer.mainContext.insert(DurableOutboxItem(operation: operation, endpoint: endpoint, body: body, filePaths: filePaths))
    }

    func processOutbox() async {
        guard !isProcessingOutbox else { return }
        isProcessingOutbox = true
        defer { isProcessingOutbox = false }
        let all = (try? modelContainer.mainContext.fetch(FetchDescriptor<DurableOutboxItem>(sortBy: [SortDescriptor(\.createdAt)]))) ?? []
        for item in all where (item.state == "queued" || item.state == "processing") && item.nextAttemptAt <= .now {
            item.state = "processing"; item.attemptCount += 1; try? modelContainer.mainContext.save()
            do {
                switch item.operation {
                case "composer_transaction":
                    try await processComposerTransaction(item)
                case "retry_media_upload":
                    guard let uploadID = item.endpoint.split(separator: "/").dropLast().last.map(String.init),
                          let upload = try modelContainer.mainContext.fetch(FetchDescriptor<CachedUpload>(predicate: #Predicate { $0.uploadID == uploadID })).first,
                          let path = item.filePaths.first, FileManager.default.fileExists(atPath: path) else { throw APIError.invalidResponse }
                    let renewed: UploadResponseDTO = try await client.send(item.endpoint, method: "POST", body: EmptyRequest(), idempotencyKey: "\(item.id.uuidString)-\(item.attemptCount)")
                    guard let signedURL = URL(string: renewed.uploadURL) else { throw APIError.invalidResponse }
                    upload.status = "uploading"; upload.updatedAt = .now
                    BackgroundUploadCoordinator.shared.upload(fileURL: URL(fileURLWithPath: path), to: signedURL, contentType: contentType(for: path), uploadID: uploadID)
                case "complete_media_upload":
                    guard let uploadID = item.endpoint.split(separator: "/").dropLast().last.map(String.init),
                          let upload = try modelContainer.mainContext.fetch(FetchDescriptor<CachedUpload>(predicate: #Predicate { $0.uploadID == uploadID })).first else { throw APIError.invalidResponse }
                    guard await completeUploadedAsset(upload, queueOnFailure: false) else { throw APIError.invalidResponse }
                case "submit_echo":
                    let _: EchoDTO = try await client.send(item.endpoint, method: "POST", body: EmptyRequest(), idempotencyKey: item.id.uuidString)
                default:
                    throw APIError.invalidResponse
                }
                item.state = "completed"; item.lastError = nil
            } catch {
                item.state = item.attemptCount >= DurableOutboxPolicy.maximumAttempts ? "failed" : "queued"
                item.lastError = String(error.localizedDescription.prefix(300))
                item.nextAttemptAt = .now.addingTimeInterval(DurableOutboxPolicy.delay(afterAttempt: item.attemptCount))
            }
            try? modelContainer.mainContext.save()
        }
    }

    /// Reconciles SwiftData upload checkpoints with the background URLSession.
    /// iOS normally restores those tasks itself, but it may purge a task after
    /// reboot or prolonged suspension. In that case we renew the presigned URL
    /// and reschedule from the durable file without reacquiring location.
    func recoverBackgroundUploads() async {
        let activeTasks = await BackgroundUploadCoordinator.shared.restoreTasks()
        let activeIDs = Set(activeTasks.compactMap(\.taskDescription))
        let uploads = (try? modelContainer.mainContext.fetch(FetchDescriptor<CachedUpload>())) ?? []
        for upload in uploads where ["ready_to_upload", "uploading", "retry_required"].contains(upload.status) {
            guard !activeIDs.contains(upload.uploadID) else { continue }
            guard FileManager.default.fileExists(atPath: upload.localFilePath) else {
                upload.status = "failed"; upload.updatedAt = .now; continue
            }
            do {
                var response: UploadResponseDTO = try await client.get(OpenAPIPaths.getMediaUpload(uploadId: upload.uploadID))
                if response.uploadURL.isEmpty {
                    response = try await client.send(
                        OpenAPIPaths.renewMediaUpload(uploadId: upload.uploadID), method: "POST", body: EmptyRequest(),
                        idempotencyKey: "recover-upload-\(upload.uploadID)"
                    )
                }
                guard let signedURL = URL(string: response.uploadURL) else { throw APIError.invalidResponse }
                upload.status = "uploading"; upload.updatedAt = .now
                BackgroundUploadCoordinator.shared.upload(
                    fileURL: URL(fileURLWithPath: upload.localFilePath), to: signedURL,
                    contentType: contentType(for: upload.localFilePath), uploadID: upload.uploadID
                )
            } catch {
                upload.status = "retry_required"; upload.updatedAt = .now
            }
        }
        try? modelContainer.mainContext.save()
    }

    private func processComposerTransaction(_ item: DurableOutboxItem) async throws {
        var transaction = try JSONDecoder.api.decode(ComposerTransactionPayload.self, from: item.body)

        if transaction.echoID == nil {
            // Always replay the same token and idempotency key, even if the
            // local expiry has passed. The original request may have committed
            // while its response was lost; the server's idempotency record is
            // authoritative and can return that already-authorized draft.
            if let sourceEchoID = transaction.sourceEchoID {
                let request = StitchCreateBody(
                    placeID: transaction.placeID, contentMode: transaction.contentMode,
                    title: transaction.title, body: transaction.bodyText,
                    captionText: transaction.captionText, creationToken: transaction.creationToken,
                    quotedReplyID: transaction.quotedReplyID,
                    quoteWarningAcknowledged: transaction.quoteWarningAcknowledged
                )
                let response: StitchResponseDTO = try await client.send(
                    OpenAPIPaths.createEchoStitch(echoId: sourceEchoID), method: "POST", body: request,
                    idempotencyKey: transaction.draftIdempotencyKey
                )
                transaction.echoID = response.echoID
                if transaction.quotedReplyID != nil {
                    quoteWarningAcknowledged = true
                    defaults.set(true, forKey: "quoteWarningAcknowledged")
                }
            } else {
                let request = CreateEchoBody(
                    placeID: transaction.placeID, contentMode: transaction.contentMode,
                    title: transaction.title, bodyText: transaction.bodyText,
                    captionText: transaction.captionText, creationToken: transaction.creationToken
                )
                let response: EchoDTO = try await client.send(
                    OpenAPIPaths.createEchoDraft, method: "POST", body: request,
                    idempotencyKey: transaction.draftIdempotencyKey
                )
                transaction.echoID = response.id
            }
            try checkpoint(transaction, on: item)
        }

        guard let echoID = transaction.echoID else { throw APIError.invalidResponse }
        if transaction.attachments.isEmpty {
            let _: EchoDTO = try await client.send(
                OpenAPIPaths.submitEcho(echoId: echoID), method: "POST", body: EmptyRequest(),
                idempotencyKey: transaction.submitIdempotencyKey
            )
            return
        }

        for index in transaction.attachments.indices {
            var attachment = transaction.attachments[index]
            guard FileManager.default.fileExists(atPath: attachment.localFilePath) else {
                throw ComposerRecoveryError.missingFile
            }
            if attachment.uploadID == nil {
                let request = CreateUploadBody(
                    mediaType: attachment.mediaType, contentType: attachment.contentType,
                    byteSize: attachment.byteSize, sortOrder: attachment.sortOrder
                )
                let response: UploadResponseDTO = try await client.send(
                    OpenAPIPaths.createMediaUpload(echoId: echoID), method: "POST", body: request,
                    idempotencyKey: attachment.uploadIdempotencyKey
                )
                guard URL(string: response.uploadURL) != nil else { throw APIError.invalidResponse }
                attachment.uploadID = response.uploadID
                attachment.mediaAssetID = response.mediaAssetID
                attachment.uploadURL = response.uploadURL
                transaction.attachments[index] = attachment
                upsertUpload(attachment, echoID: echoID, status: "ready_to_upload")
                try checkpoint(transaction, on: item)
            }

            guard let uploadID = attachment.uploadID,
                  let signedURLString = attachment.uploadURL,
                  let signedURL = URL(string: signedURLString) else { throw APIError.invalidResponse }
            upsertUpload(attachment, echoID: echoID, status: "uploading")
            BackgroundUploadCoordinator.shared.upload(
                fileURL: URL(fileURLWithPath: attachment.localFilePath), to: signedURL,
                contentType: attachment.contentType, uploadID: uploadID
            )
        }
    }

    private func checkpoint(_ transaction: ComposerTransactionPayload, on item: DurableOutboxItem) throws {
        item.body = try JSONEncoder.api.encode(transaction)
        try modelContainer.mainContext.save()
    }

    private func upsertUpload(_ attachment: ComposerTransactionPayload.Attachment, echoID: String, status: String) {
        guard let uploadID = attachment.uploadID, let mediaAssetID = attachment.mediaAssetID else { return }
        let descriptor = FetchDescriptor<CachedUpload>(predicate: #Predicate { $0.uploadID == uploadID })
        if let existing = try? modelContainer.mainContext.fetch(descriptor).first {
            existing.status = status; existing.updatedAt = .now
        } else {
            modelContainer.mainContext.insert(CachedUpload(
                uploadID: uploadID, echoID: echoID, mediaAssetID: mediaAssetID,
                localFilePath: attachment.localFilePath, status: status,
                sortOrder: attachment.sortOrder
            ))
        }
        try? modelContainer.mainContext.save()
    }

    private func contentType(for path: String) -> String {
        switch URL(fileURLWithPath: path).pathExtension.lowercased() {
        case "jpg", "jpeg": return "image/jpeg"
        case "heic", "heif": return "image/heic"
        case "m4a": return "audio/mp4"
        case "mp4": return "video/mp4"
        default: return "video/quicktime"
        }
    }

    private func cache(_ values: [PlaceDTO]) {
        let context = modelContainer.mainContext
        let existing = (try? context.fetch(FetchDescriptor<CachedPlace>())) ?? []
        let byID = Dictionary(uniqueKeysWithValues: existing.map { ($0.id, $0) })
        for value in values {
            if let cached = byID[value.id] {
                cached.name = value.name; cached.state = value.state; cached.latitude = value.latitude; cached.longitude = value.longitude
                cached.radiusMeters = value.radiusMeters; cached.category = value.category; cached.sensitive = value.sensitive; cached.cachedAt = .now
            } else {
                context.insert(CachedPlace(id: value.id, name: value.name, state: value.state, latitude: value.latitude, longitude: value.longitude, radiusMeters: value.radiusMeters, category: value.category, sensitive: value.sensitive))
            }
        }
        try? context.save()
    }

    private func cache(_ values: [DiscoveryResultDTO]) {
        let context = modelContainer.mainContext
        let existing = (try? context.fetch(FetchDescriptor<CachedEcho>())) ?? []
        let byID = Dictionary(uniqueKeysWithValues: existing.map { ($0.id, $0) })
        for value in values {
            let encoded = try? JSONEncoder.api.encode(value.echo)
            if let cached = byID[value.echo.id] {
                cached.placeID = value.echo.placeID; cached.authorHandle = value.echo.authorHandle; cached.title = value.echo.title
                cached.body = value.echo.bodyText; cached.caption = value.echo.captionText; cached.contentMode = value.echo.contentMode
                cached.status = value.echo.status; cached.publishedAt = value.echo.publishedAt; cached.encodedEcho = encoded
                cached.rankingScore = value.score; cached.rankingLabel = value.label; cached.cachedAt = .now
            } else {
                context.insert(CachedEcho(id: value.echo.id, placeID: value.echo.placeID, authorHandle: value.echo.authorHandle,
                    title: value.echo.title, body: value.echo.bodyText, caption: value.echo.captionText,
                    contentMode: value.echo.contentMode, status: value.echo.status, publishedAt: value.echo.publishedAt,
                    encodedEcho: encoded, rankingScore: value.score, rankingLabel: value.label))
            }
        }
        try? context.save()
    }

    private func cache(_ value: MeDTO) {
        let context = modelContainer.mainContext
        let userID = value.userID
        let descriptor = FetchDescriptor<CachedProfile>(predicate: #Predicate { $0.userID == userID })
        if let cached = try? context.fetch(descriptor).first {
            cached.handle = value.handle; cached.displayName = value.displayName; cached.bio = value.bio ?? ""; cached.avatarURL = value.avatarURL; cached.cachedAt = .now
        } else {
            context.insert(CachedProfile(userID: value.userID, handle: value.handle, displayName: value.displayName, bio: value.bio ?? "", avatarURL: value.avatarURL))
        }
        try? context.save()
    }

    private func hydrateCachedContent() {
        let context = modelContainer.mainContext
        let cachedPlaces = (try? context.fetch(FetchDescriptor<CachedPlace>(sortBy: [SortDescriptor(\.name)]))) ?? []
        places = cachedPlaces.filter { $0.state == "active" }.map {
            PlaceDTO(id: $0.id, name: $0.name, state: $0.state, latitude: $0.latitude, longitude: $0.longitude,
                radiusMeters: $0.radiusMeters, category: $0.category, sensitive: $0.sensitive)
        }
        let cachedEchoes = (try? context.fetch(FetchDescriptor<CachedEcho>(sortBy: [SortDescriptor(\.cachedAt, order: .reverse)]))) ?? []
        discovery = cachedEchoes.compactMap { cached in
            guard let data = cached.encodedEcho, let echo = try? JSONDecoder.api.decode(EchoDTO.self, from: data) else { return nil }
            return DiscoveryResultDTO(echo: echo, score: cached.rankingScore, label: cached.rankingLabel)
        }
        if let cached = try? context.fetch(FetchDescriptor<CachedProfile>(sortBy: [SortDescriptor(\.cachedAt, order: .reverse)])).first {
            profile = MeDTO(userID: cached.userID, handle: cached.handle, displayName: cached.displayName, bio: cached.bio, avatarURL: cached.avatarURL)
        }
    }
    private func pushEnvironment() -> String { #if DEBUG
        return "sandbox"
        #else
        return "production"
        #endif
    }
    private func perform<T>(_ work: () async throws -> T) async -> T? { isWorking = true; errorMessage = nil; defer { isWorking = false }; do { return try await work() } catch { errorMessage = error.localizedDescription; return nil } }
    private func performNoContent(_ work: () async throws -> Void) async -> Bool { isWorking = true; errorMessage = nil; defer { isWorking = false }; do { try await work(); return true } catch { errorMessage = error.localizedDescription; return false } }
}

private enum ComposerRecoveryError: LocalizedError {
    case missingFile
    var errorDescription: String? { "A captured media file is no longer available. Keep this failed draft so it can be reviewed or discarded." }
}
