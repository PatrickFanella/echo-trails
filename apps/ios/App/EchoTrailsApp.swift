import AuthenticationServices
import SwiftData
import SwiftUI
import UserNotifications

enum AppCapabilities {
    private static func enabled(_ key: String, default fallback: Bool) -> Bool {
        guard let value = Bundle.main.object(forInfoDictionaryKey: key) else { return fallback }
        if let boolean = value as? Bool { return boolean }
        return (value as? String)?.uppercased() != "NO"
    }

    static var pushEnabled: Bool { enabled("ECHOTRAILS_PUSH_ENABLED", default: true) }
    static var appleSignInEnabled: Bool { enabled("ECHOTRAILS_APPLE_SIGN_IN_ENABLED", default: true) }
}

extension Notification.Name {
    static let didReceivePushToken = Notification.Name("EchoTrails.didReceivePushToken")
    static let didReceiveRemoteNotification = Notification.Name("EchoTrails.didReceiveRemoteNotification")
    static let didReceiveDeepLink = Notification.Name("EchoTrails.didReceiveDeepLink")
}

final class AppDelegate: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool { UNUserNotificationCenter.current().delegate = self; return true }
    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) { NotificationCenter.default.post(name: .didReceivePushToken, object: deviceToken.map { String(format: "%02x", $0) }.joined()) }
    func application(_ application: UIApplication, didReceiveRemoteNotification userInfo: [AnyHashable: Any], fetchCompletionHandler completionHandler: @escaping (UIBackgroundFetchResult) -> Void) { NotificationCenter.default.post(name: .didReceiveRemoteNotification, object: nil); completionHandler(.newData) }
    func application(_ application: UIApplication, handleEventsForBackgroundURLSession identifier: String, completionHandler: @escaping () -> Void) { BackgroundUploadCoordinator.shared.setBackgroundCompletion(completionHandler) }
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification) async -> UNNotificationPresentationOptions {
        await MainActor.run { NotificationCenter.default.post(name: .didReceiveRemoteNotification, object: nil) }
        return [.banner, .sound, .badge]
    }
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
        await MainActor.run { NotificationCenter.default.post(name: .didReceiveRemoteNotification, object: nil) }
        guard let value = response.notification.request.content.userInfo["deep_link"] as? String, let url = URL(string: value) else { return }
        await MainActor.run { NotificationCenter.default.post(name: .didReceiveDeepLink, object: url) }
    }
}

@main struct EchoTrailsApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    private let container: ModelContainer
    @StateObject private var model: AppModel

    init() {
        let container = try! PersistenceStack.container()
        self.container = container
        _model = StateObject(wrappedValue: AppModel(modelContainer: container))
    }

    var body: some Scene {
        WindowGroup { RootView().environmentObject(model).task { await model.bootstrap() }.onOpenURL { url in Task { await model.handleDeepLink(url) } } }
            .modelContainer(container)
    }
}

struct RootView: View {
    @EnvironmentObject private var app: AppModel
    var body: some View {
        Group {
            switch app.gate {
            case .loading: VStack(spacing: 16) { ProgressView("Opening EchoTrails"); Button("Retry") { Task { await app.bootstrap() } } }.padding()
            case .authentication: AuthenticationView()
            case .invite: InviteView()
            case .consent: ConsentView()
            case .profile: ProfileSetupView()
            case .ready: MainTabView()
            }
        }
        .alert("EchoTrails", isPresented: Binding(get: { app.errorMessage != nil }, set: { if !$0 { app.errorMessage = nil } })) { Button("OK") { app.errorMessage = nil } } message: { Text(app.errorMessage ?? "") }
    }
}

struct AuthenticationView: View {
    @EnvironmentObject private var app: AppModel
    @State private var email = ""
    @State private var code = ""
    @State private var requestID: String?
    var body: some View {
        NavigationStack { Form {
            Section { Text("Leave stories at real public Places and discover what happened there.").font(.title3) }
            Section("Email sign in") {
                TextField("Email", text: $email).textContentType(.emailAddress).keyboardType(.emailAddress).textInputAutocapitalization(.never)
                if requestID == nil { Button("Send one-time code") { Task { requestID = await app.startEmail(email) } }.disabled(email.isEmpty || app.isWorking) }
                else { TextField("Six-digit code", text: $code).keyboardType(.numberPad).textContentType(.oneTimeCode); Button("Verify code") { Task { await app.verifyEmail(requestID: requestID!, code: code) } }.disabled(code.count < 6 || app.isWorking) }
            }
            if AppCapabilities.appleSignInEnabled {
                Section("Or") { SignInWithAppleButton(.signIn) { request in request.requestedScopes = [.email] } onCompletion: { result in if case let .success(authorization) = result { Task { await app.completeAppleAuthorization(authorization) } } else if case let .failure(error) = result { app.errorMessage = error.localizedDescription } }.signInWithAppleButtonStyle(.black).frame(height: 48).accessibilityLabel("Sign in with Apple") }
            }
        }.navigationTitle("EchoTrails") }
    }
}

struct InviteView: View {
    @EnvironmentObject private var app: AppModel
    @State private var code = ""
    var body: some View { NavigationStack { Form { Section("Downers Grove beta") { Text("Enter your invite code. The first beta catalog contains walkable public Places in Downers Grove."); TextField("Invite code", text: $code).textInputAutocapitalization(.characters); Button("Join beta") { Task { await app.claimInvite(code: code) } }.disabled(code.isEmpty || app.isWorking) } }.navigationTitle("Invitation") } }
}

struct ConsentView: View {
    @EnvironmentObject private var app: AppModel
    @State private var accepted = false
    var body: some View { NavigationStack { Form { Section("Before you continue") { Text("Echoes are public. Exact creation coordinates are encrypted for validation and are never shown in public responses, search, analytics, logs, or notifications."); Text("Photo, audio, video, captions, and transcripts are checked by conservative self-hosted safety systems. Uncertain content stays private for review."); Toggle("I agree to the current beta terms and privacy disclosure", isOn: $accepted); Button("Continue") { Task { await app.acceptConsent() } }.disabled(!accepted || app.isWorking) } }.navigationTitle("Privacy & safety") } }
}

struct ProfileSetupView: View {
    @EnvironmentObject private var app: AppModel
    @State private var handle = ""
    @State private var displayName = ""
    @State private var bio = ""
    private var valid: Bool { handle.range(of: "^[A-Za-z][A-Za-z0-9_]{2,29}$", options: .regularExpression) != nil && displayName.count <= 80 && bio.count <= 160 }
    var body: some View { NavigationStack { Form { Section("Public profile") { TextField("Handle", text: $handle).textInputAutocapitalization(.never).autocorrectionDisabled(); TextField("Display name", text: $displayName); TextField("Short bio (optional)", text: $bio, axis: .vertical); Text("Your email, subscriptions, notification preferences, and exact creation location are never part of this public profile.").font(.caption).foregroundStyle(.secondary); Button("Create profile") { Task { await app.saveProfile(handle: handle, displayName: displayName, bio: bio) } }.disabled(!valid || app.isWorking) } }.navigationTitle("Choose a handle") } }
}

struct MainTabView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    var body: some View {
        TabView {
            NavigationStack { PlacesMapView() }.tabItem { Label("Map", systemImage: "map") }
            NavigationStack { ExploreView() }.tabItem { Label("Explore", systemImage: "sparkle.magnifyingglass") }
            NavigationStack { ActivityView() }.tabItem { Label("Activity", systemImage: "bell") }
            NavigationStack { ProfileView() }.tabItem { Label("Profile", systemImage: "person.crop.circle") }
        }
        .task { if AppCapabilities.pushEnabled { await requestPushPermission() } }
        .refreshable { await app.refreshAll() }
        .onReceive(NotificationCenter.default.publisher(for: .didReceivePushToken)) { notification in if let token = notification.object as? String { Task { await app.registerPushToken(token) } } }
        .onReceive(NotificationCenter.default.publisher(for: .didReceiveRemoteNotification)) { _ in Task { await app.loadActivity() } }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.willEnterForegroundNotification)) { _ in Task { await app.loadActivity(); await app.processOutbox() } }
        .sheet(item: $app.linkedEcho) { echo in NavigationStack { EchoDetailView(echo: echo) } }
        .sheet(item: $app.linkedPlace) { place in NavigationStack { PlaceDetailView(place: place) } }
        .sheet(item: $app.appealAction) { action in NavigationStack { AppealComposerView(moderationActionID: action.id) } }
        .onChange(of: app.deepLink) { if let url = app.deepLink { Task { await app.handleDeepLink(url) } } }
        .transaction { transaction in if reduceMotion { transaction.animation = nil } }
    }
    private func requestPushPermission() async { if (try? await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound])) == true { await MainActor.run { UIApplication.shared.registerForRemoteNotifications() } } }
}
