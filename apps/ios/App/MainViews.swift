import MapKit
import AVKit
import EchoTrailsShell
import Network
import PhotosUI
import SwiftData
import SwiftUI

struct PlacesMapView: View {
    @EnvironmentObject private var app: AppModel
    @State private var camera: MapCameraPosition = .automatic
    var body: some View {
        Map(position: $camera) {
            ForEach(app.places.filter { $0.latitude != nil && $0.longitude != nil }) { place in
                Annotation(place.name, coordinate: CLLocationCoordinate2D(latitude: place.latitude!, longitude: place.longitude!)) { NavigationLink(value: place) { Image(systemName: "wave.3.right.circle.fill").font(.title).foregroundStyle(.indigo).background(.white, in: Circle()).accessibilityLabel("\(place.name), public Place") } }
            }
        }
        .mapControls { MapUserLocationButton(); MapCompass(); MapScaleView() }
        .navigationTitle("Nearby Places")
        .navigationDestination(for: PlaceDTO.self) { PlaceDetailView(place: $0) }
        .overlay(alignment: .bottom) { if app.places.isEmpty { ContentUnavailableView("No Places cached", systemImage: "map", description: Text("Connect to refresh the Downers Grove catalog.")); } }
    }
}

struct ExploreView: View {
    @EnvironmentObject private var app: AppModel
    @State private var query = ""
    @State private var scope: DiscoverySearchScope = .city
    @State private var selectedPlaceID: String?
    @State private var searchTask: Task<Void, Never>?
    @State private var lastSearchQuery: DiscoverySearchQuery?
    var body: some View {
        List {
            Section("Search scope") {
                Picker("Search scope", selection: $scope) {
                    Text("City").tag(DiscoverySearchScope.city)
                    Text("Nearby").tag(DiscoverySearchScope.nearby)
                    Text("Place").tag(DiscoverySearchScope.place)
                }
                .pickerStyle(.segmented)
                if scope == .place {
                    Picker("Place", selection: $selectedPlaceID) {
                        Text("Choose a Place").tag(Optional<String>.none)
                        ForEach(app.places) { place in Text(place.name).tag(Optional(place.id)) }
                    }
                } else if scope == .nearby {
                    Label("Uses your current location for this search only", systemImage: "location")
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
            if query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                Section("Notable in the city") { discoveryRows(app.discovery) }
            } else if query.count > 200 {
                ContentUnavailableView("Search is too long", systemImage: "text.badge.xmark", description: Text("Use 200 characters or fewer."))
            } else if scope == .place && selectedPlaceID == nil {
                ContentUnavailableView("Choose a Place", systemImage: "mappin.and.ellipse", description: Text("Place search only returns Echoes from the selected public Place."))
            } else if app.isSearching {
                Section { HStack { Spacer(); ProgressView("Searching"); Spacer() } }
            } else if let results = app.searchResults {
                if !results.places.isEmpty {
                    Section("Places") { ForEach(results.places) { NavigationLink($0.name, value: $0) } }
                }
                if !results.echoes.isEmpty {
                    Section("Echoes") { discoveryRows(results.echoes) }
                }
                if results.places.isEmpty && results.echoes.isEmpty && app.searchError == nil {
                    ContentUnavailableView.search(text: query)
                }
                if let cursor = app.searchNextCursor, let lastSearchQuery {
                    Button { Task { await app.search(lastSearchQuery.withCursor(cursor), reset: false) } } label: {
                        HStack { Spacer(); if app.isSearching { ProgressView() } else { Text("Load more results") }; Spacer() }
                    }.disabled(app.isSearching)
                }
            }
            if let searchError = app.searchError {
                Section { Label(searchError, systemImage: "wifi.exclamationmark").foregroundStyle(.secondary) }
            }
        }
        .navigationTitle("Explore").searchable(text: $query, prompt: "Search Places and Echoes")
        .onChange(of: query) { scheduleSearch() }
        .onChange(of: scope) { scheduleSearch() }
        .onChange(of: selectedPlaceID) { scheduleSearch() }
        .onDisappear { searchTask?.cancel() }
        .navigationDestination(for: PlaceDTO.self) { PlaceDetailView(place: $0) }
        .overlay { if app.discovery.isEmpty && query.isEmpty { ContentUnavailableView("Stories will appear here", systemImage: "sparkles", description: Text("Explore safely published Echoes from nearby public Places.")) } }
    }

    @ViewBuilder private func discoveryRows(_ results: [DiscoveryResultDTO]) -> some View {
        ForEach(results) { result in
            NavigationLink { EchoDetailView(echo: result.echo) } label: {
                VStack(alignment: .leading, spacing: 3) {
                    Text(result.echo.title).font(.headline)
                    Text(result.label).font(.caption).foregroundStyle(.secondary)
                    Text("@\(result.echo.authorHandle)").font(.caption)
                }
                .accessibilityElement(children: .combine)
                .accessibilityLabel("\(result.echo.title), \(result.label), by \(result.echo.authorHandle)")
            }
        }
    }

    private func scheduleSearch() {
        searchTask?.cancel()
        let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { app.clearSearch(); return }
        guard trimmed.count <= 200, scope != .place || selectedPlaceID != nil else { app.clearSearch(); return }
        searchTask = Task {
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            var latitude: Double?
            var longitude: Double?
            if scope == .nearby {
                do {
                    let location = try await LocationProofProvider().currentLocation()
                    latitude = location.coordinate.latitude
                    longitude = location.coordinate.longitude
                } catch {
                    guard !Task.isCancelled else { return }
                    app.searchResults = SearchResponseDTO(places: [], echoes: [], nextCursor: nil)
                    app.searchError = error.localizedDescription
                    return
                }
            }
            let request = DiscoverySearchQuery(
                text: trimmed,
                scope: scope,
                placeID: selectedPlaceID,
                latitude: latitude,
                longitude: longitude,
                radiusMeters: 2_000,
                limit: 30
            )
            lastSearchQuery = request
            await app.search(request)
        }
    }
}

struct PlaceDetailView: View {
    @EnvironmentObject private var app: AppModel
    let place: PlaceDTO
    @State private var creating = false
    @State private var echoes: [EchoDTO] = []
    @State private var nextCursor: String?
    @State private var isLoadingEchoes = false
    @State private var echoError: String?
    @State private var echoSort: PlaceEchoSort = .notable
    var body: some View {
        List { Section { Label(place.category?.capitalized ?? "Public Place", systemImage: "mappin.and.ellipse"); if place.sensitive == true { Label("Creation restricted for safety", systemImage: "exclamationmark.shield") } }
            Section("Echoes") {
                Picker("Sort Echoes", selection: $echoSort) { ForEach(PlaceEchoSort.allCases) { Text($0.title).tag($0) } }.pickerStyle(.segmented)
                if echoes.isEmpty && !isLoadingEchoes { Text("No published Echoes yet.").foregroundStyle(.secondary) }
                ForEach(echoes) { echo in NavigationLink { EchoDetailView(echo: echo) } label: { Text(echo.title) } }
                if nextCursor != nil {
                    Button { Task { await loadEchoes(reset: false) } } label: { HStack { Spacer(); if isLoadingEchoes { ProgressView() } else { Text("Load more Echoes") }; Spacer() } }.disabled(isLoadingEchoes)
                }
                if let echoError { Label(echoError, systemImage: "wifi.exclamationmark").foregroundStyle(.secondary) }
            }
            Section("Subscription & alerts") {
                Button(app.subscriptionsByPlaceID[place.id] == nil ? "Subscribe to this Place" : "Unsubscribe from this Place") { Task { await app.subscribe(placeID: place.id) } }
                if let subscription = app.subscriptionsByPlaceID[place.id] {
                    Toggle("Place alert notifications", isOn: Binding(
                        get: { app.subscriptionsByPlaceID[place.id]?.notificationsEnabled ?? subscription.notificationsEnabled },
                        set: { enabled in Task { await app.setPlaceNotifications(placeID: place.id, enabled: enabled) } }
                    ))
                    Text("Subscriptions are private. Alerts are sent only for eligible notable Echoes and remain available in Activity.").font(.caption).foregroundStyle(.secondary)
                }
                Menu("Place mute options") {
                    Button("Mute this Place") { Task { await app.setMute(target: .place, id: place.id, muted: true) } }
                    Button("Unmute this Place") { Task { await app.setMute(target: .place, id: place.id, muted: false) } }
                }
            }
        }
        .navigationTitle(place.name).toolbar { ToolbarItem(placement: .primaryAction) { Button { creating = true } label: { Label("Create Echo", systemImage: "plus") }.disabled(place.state != "active") } }
        .sheet(isPresented: $creating) { NavigationStack { ComposerView(place: place) } }
        .task(id: echoSort) { await loadEchoes(reset: true) }
    }
    private func loadEchoes(reset: Bool) async {
        guard !isLoadingEchoes, let placeID = ContractPath.safeSegment(place.id) else { return }
        let cursor = reset ? nil : nextCursor
        if !reset && cursor == nil { return }
        var path = "\(OpenAPIPaths.listPlaceEchoes(placeId: placeID))?limit=20&sort=\(echoSort.rawValue)"
        if let cursor { path += "&cursor=" + (cursor.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? "") }
        isLoadingEchoes = true; echoError = nil; defer { isLoadingEchoes = false }
        do {
            let page: PublicProfileEchoesDTO = try await app.client.get(path)
            if reset { echoes = page.echoes }
            else {
                let existing = Set(echoes.map(\.id)); echoes.append(contentsOf: page.echoes.filter { !existing.contains($0.id) })
            }
            nextCursor = page.nextCursor?.isEmpty == false ? page.nextCursor : nil
        } catch { echoError = error.localizedDescription }
    }
}

private enum PlaceEchoSort: String, CaseIterable, Identifiable {
    case recent, notable, oldest
    var id: String { rawValue }
    var title: String { switch self { case .recent: "Recent"; case .notable: "Notable"; case .oldest: "Oldest" } }
}

struct EchoDetailView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.dismiss) private var dismiss
    let echo: EchoDTO
    @State private var reply = ""
    @State private var replies: [ReplyDTO] = []
    @State private var replyTo: ReplyDTO?
    @State private var editingReply: ReplyDTO?
    @State private var historyReply: ReplyDTO?
    @State private var deletingReply: ReplyDTO?
    @State private var replyError: String?
    @State private var isPostingReply = false
    @State private var showingStitch = false
    @State private var quotedReplyID: String?
    @State private var showingBlockConfirmation = false
    @State private var reportTarget: SafetyReportTarget?
    @State private var showingDeleteEchoConfirmation = false
    private var isOwnEcho: Bool { echo.authorHandle == app.profile?.handle }
    var body: some View {
        List { Section { Text(echo.title).font(.title2.bold()); if let source = echo.stitchSource { VStack(alignment: .leading) { Text("Stitched from @\(source.authorHandle)").font(.caption); Text(source.title); if source.quoteUnavailable == true { Text("Quoted reply unavailable").foregroundStyle(.secondary) } }.padding().background(.quaternary, in: RoundedRectangle(cornerRadius: 10)) }; if let body = echo.bodyText { Text(body) }; if let caption = echo.captionText { Text(caption) }; MediaGalleryView(assets: echo.media ?? [], echoTitle: echo.title, caption: echo.captionText); NavigationLink("@\(echo.authorHandle)") { PublicProfileView(handle: echo.authorHandle) }.foregroundStyle(.secondary); Button("Stitch this Echo") { quotedReplyID = nil; showingStitch = true } }
            moderationStatusSection
            Section("React") { HStack { ForEach(["heart", "fire", "sad", "laugh", "clap", "mind_blown"], id: \.self) { kind in Button(reactionSymbol(kind)) { Task { await react(kind) } }.buttonStyle(.borderless).accessibilityLabel("React \(kind.replacingOccurrences(of: "_", with: " "))") } } }
            Section("Replies") {
                if replies.isEmpty { Text("No Replies yet. Start the conversation.").foregroundStyle(.secondary) }
                ForEach(threadLayout.rootIDs, id: \.self) { rootID in
                    if let root = replyByID[rootID] {
                        replyRow(root, nested: false)
                        ForEach(threadLayout.childIDsByParent[rootID] ?? [], id: \.self) { childID in
                            if let child = replyByID[childID] { replyRow(child, nested: true) }
                        }
                    }
                }
                if let replyTo {
                    HStack {
                        Label("Replying to @\(replyTo.authorHandle)", systemImage: "arrowshape.turn.up.left")
                            .font(.caption).foregroundStyle(.secondary)
                        Spacer()
                        Button("Cancel") { self.replyTo = nil }.font(.caption)
                    }
                }
                TextField(replyTo == nil ? "Write a reply" : "Write a nested reply", text: $reply, axis: .vertical)
                    .lineLimit(2...6)
                    .accessibilityHint("Replies are limited to 500 characters")
                HStack {
                    Text("\(reply.count)/500").font(.caption).foregroundStyle(reply.count > 500 ? .red : .secondary)
                    Spacer()
                    Button { Task { await postReply() } } label: {
                        if isPostingReply { ProgressView() } else { Text(replyTo == nil ? "Reply" : "Reply in thread") }
                    }
                    .disabled(reply.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || reply.count > 500 || isPostingReply)
                }
                if let replyError { Label(replyError, systemImage: "exclamationmark.circle").font(.caption).foregroundStyle(.red) }
            }
            Section("Notifications & safety") {
                Menu("Thread notifications") {
                    Button("Mute this Echo thread") { Task { await app.setMute(target: .echo, id: echo.id, muted: true) } }
                    Button("Unmute this Echo thread") { Task { await app.setMute(target: .echo, id: echo.id, muted: false) } }
                }
                if isOwnEcho {
                    Button("Delete this Echo", role: .destructive) { showingDeleteEchoConfirmation = true }
                } else {
                    Button("Hide this Echo") { Task { try? await app.client.sendNoContent(OpenAPIPaths.hideTarget, method: "POST", body: ["target_id": echo.id], idempotencyKey: UUID().uuidString) } }
                    Button("Block @\(echo.authorHandle)", role: .destructive) { showingBlockConfirmation = true }
                    Button("Report a safety concern", role: .destructive) { reportTarget = SafetyReportTarget(type: "echo", id: echo.id, label: echo.title) }
                }
            }
        }.navigationTitle("Echo").task {
            async let repliesTask: Void = loadReplies()
            async let viewTask: Void = recordDetailView()
            _ = await (repliesTask, viewTask)
        }
        .sheet(isPresented: $showingStitch) { StitchPlacePicker(sourceEchoID: echo.id, quotedReplyID: quotedReplyID) }
        .sheet(item: $editingReply) { target in ReplyEditorView(reply: target) { updated in updateReplyLocally(updated); Task { await loadReplies() } } }
        .sheet(item: $historyReply) { target in ReplyHistoryView(reply: target) }
        .sheet(item: $reportTarget) { target in SafetyReportView(target: target) }
        .confirmationDialog("Delete this Reply?", isPresented: Binding(get: { deletingReply != nil }, set: { if !$0 { deletingReply = nil } }), presenting: deletingReply) { target in
            Button("Delete Reply", role: .destructive) { Task { await deleteReply(target) } }
            Button("Cancel", role: .cancel) { deletingReply = nil }
        } message: { _ in Text("The text will be replaced with “Reply deleted” so nested Replies keep their context.") }
        .confirmationDialog("Block @\(echo.authorHandle)?", isPresented: $showingBlockConfirmation) {
            Button("Block user", role: .destructive) { Task { await blockAuthor() } }
            Button("Cancel", role: .cancel) {}
        } message: { Text("Their Echoes, Replies, and notifications will be hidden from you.") }
        .confirmationDialog("Delete this Echo?", isPresented: $showingDeleteEchoConfirmation) {
            Button("Delete Echo", role: .destructive) { Task { if await app.deleteEcho(id: echo.id) { dismiss() } } }
            Button("Cancel", role: .cancel) {}
        } message: { Text("This removes the Echo from discovery and cannot be undone.") }
    }
    @ViewBuilder private var moderationStatusSection: some View {
        let status = ModerationStatusPresentation(rawValue: echo.status)
        Section("Publishing status") {
            Label(status.title, systemImage: status.isAdverse ? "exclamationmark.shield.fill" : "checkmark.shield")
                .accessibilityLabel("Moderation status: \(status.title)")
            if status == .awaitingReview { Text("This Echo stays private until a moderator completes review.").font(.caption).foregroundStyle(.secondary) }
            if status.isAdverse {
                Text("Open the moderation notice in Activity to see its action reference and submit an appeal.")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
    }
    private var replyByID: [String: ReplyDTO] { Dictionary(uniqueKeysWithValues: replies.map { ($0.id, $0) }) }
    private var threadLayout: ReplyThreadLayout { ReplyThreadLayout(nodes: replies.map { ReplyThreadNode(id: $0.id, parentID: $0.replyTo, postedAt: $0.postedAt) }) }
    private func replyRow(_ item: ReplyDTO, nested: Bool) -> some View {
        ReplyRow(
            reply: item,
            nested: nested,
            isOwn: item.userID == app.profile?.userID,
            onReply: nested ? nil : { replyTo = item },
            onQuote: { quotedReplyID = item.id; showingStitch = true },
            onEdit: { editingReply = item },
            onHistory: { historyReply = item },
            onDelete: { deletingReply = item },
            onReport: { reportTarget = SafetyReportTarget(type: "reply", id: item.id, label: "Reply") }
        )
    }
    private func reactionSymbol(_ kind: String) -> String { ["heart":"♥︎", "fire":"🔥", "sad":"😢", "laugh":"😂", "clap":"👏", "mind_blown":"🤯"][kind]! }
    private func react(_ kind: String) async { let _: ReactionDTO? = try? await app.client.send(OpenAPIPaths.putEchoReaction(echoId: echo.id), method: "PUT", body: ["kind": kind], idempotencyKey: UUID().uuidString) }
    private func blockAuthor() async {
        do {
            try await app.client.sendNoContent(OpenAPIPaths.blockUser, method: "POST", body: ["target_handle": echo.authorHandle], idempotencyKey: UUID().uuidString)
            app.linkedEcho = nil
            await app.loadDiscovery()
        } catch { replyError = error.localizedDescription }
    }
    private func loadReplies() async {
        do { let response: RepliesDTO = try await app.client.get(OpenAPIPaths.listEchoReplies(echoId: echo.id)); replies = response.replies; replyError = nil }
        catch { replyError = error.localizedDescription }
    }
    private func recordDetailView() async { try? await app.client.sendNoContent(OpenAPIPaths.recordEchoDetailView(echoId: echo.id), method: "POST", body: EmptyRequest(), idempotencyKey: "detail-view-\(echo.id)") }
    private func postReply() async {
        let body = reply.trimmingCharacters(in: .whitespacesAndNewlines)
        guard ReplyComposerDescriptor(echoID: echo.id, body: body, parentReplyID: replyTo?.id).isValid else { return }
        isPostingReply = true; replyError = nil; defer { isPostingReply = false }
        do {
            let created: ReplyDTO = try await app.client.send(OpenAPIPaths.createEchoReply(echoId: echo.id), method: "POST", body: ReplyMutationBody(body: body, replyTo: replyTo?.id), idempotencyKey: UUID().uuidString)
            replies.append(created)
            reply = ""; replyTo = nil; await loadReplies()
        } catch { replyError = error.localizedDescription }
    }
    private func deleteReply(_ target: ReplyDTO) async {
        deletingReply = nil
        guard let path = ReplyRevisionDescriptor(replyID: target.id).deleteAPIPath else { return }
        do {
            try await app.client.sendNoContent(path, method: "DELETE", body: Optional<EmptyRequest>.none, idempotencyKey: UUID().uuidString)
            updateReplyLocally(target.continuityPlaceholder())
            await loadReplies()
        } catch { replyError = error.localizedDescription }
    }
    private func updateReplyLocally(_ updated: ReplyDTO) {
        if let index = replies.firstIndex(where: { $0.id == updated.id }) { replies[index] = updated }
    }
}

struct StitchPlacePicker: View {
    @EnvironmentObject private var app: AppModel
    let sourceEchoID: String
    let quotedReplyID: String?
    var body: some View { NavigationStack { List(app.places) { place in NavigationLink(place.name) { ComposerView(place: place, sourceEchoID: sourceEchoID, quotedReplyID: quotedReplyID) } }.navigationTitle("Choose Stitch Place") } }
}

struct ReplyMutationBody: Encodable {
    let body: String
    let replyTo: String?
    enum CodingKeys: String, CodingKey { case body; case replyTo = "reply_to" }
}
struct ReplyDTO: Decodable, Identifiable, Equatable {
    let id, echoID, userID, authorHandle, body: String
    let replyTo: String?
    let edited, hidden: Bool
    let postedAt: Date
    var isContinuityPlaceholder: Bool { hidden || body == "Reply deleted" }
    enum CodingKeys: String, CodingKey { case id, body, edited, hidden; case echoID = "echo_id", userID = "user_id", authorHandle = "author_handle", replyTo = "reply_to", postedAt = "posted_at" }
    init(id: String, echoID: String, userID: String, authorHandle: String? = nil, body: String, replyTo: String?, edited: Bool, hidden: Bool, postedAt: Date) {
        self.id = id; self.echoID = echoID; self.userID = userID; self.authorHandle = authorHandle ?? userID; self.body = body; self.replyTo = replyTo
        self.edited = edited; self.hidden = hidden; self.postedAt = postedAt
    }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        id = try values.decode(String.self, forKey: .id)
        echoID = try values.decode(String.self, forKey: .echoID)
        userID = try values.decode(String.self, forKey: .userID)
        authorHandle = try values.decode(String.self, forKey: .authorHandle)
        body = try values.decode(String.self, forKey: .body)
        replyTo = try values.decodeIfPresent(String.self, forKey: .replyTo).flatMap { $0.isEmpty ? nil : $0 }
        edited = try values.decodeIfPresent(Bool.self, forKey: .edited) ?? false
        hidden = try values.decodeIfPresent(Bool.self, forKey: .hidden) ?? false
        postedAt = try values.decode(Date.self, forKey: .postedAt)
    }
    func continuityPlaceholder() -> ReplyDTO {
        ReplyDTO(id: id, echoID: echoID, userID: userID, authorHandle: authorHandle, body: "Reply deleted", replyTo: replyTo, edited: edited, hidden: true, postedAt: postedAt)
    }
}
struct RepliesDTO: Decodable { let replies: [ReplyDTO] }
struct ReplyRevisionDTO: Decodable, Identifiable {
    let revisionNumber: Int
    let body: String
    let createdAt: Date
    var id: Int { revisionNumber }
    enum CodingKeys: String, CodingKey { case body; case revisionNumber = "revision_number", createdAt = "created_at" }
}
struct ReplyHistoryDTO: Decodable { let revisions: [ReplyRevisionDTO] }
struct ReactionDTO: Decodable { let echoID: String; enum CodingKeys: String, CodingKey { case echoID = "echo_id" } }
struct ReportDTO: Decodable { let id: String }

private struct ReplyRow: View {
    let reply: ReplyDTO
    let nested: Bool
    let isOwn: Bool
    let onReply: (() -> Void)?
    let onQuote: () -> Void
    let onEdit: () -> Void
    let onHistory: () -> Void
    let onDelete: () -> Void
    let onReport: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            if nested { Image(systemName: "arrow.turn.down.right").foregroundStyle(.secondary).accessibilityHidden(true) }
            VStack(alignment: .leading, spacing: 5) {
                if reply.isContinuityPlaceholder {
                    Label("Reply deleted", systemImage: "text.badge.xmark").foregroundStyle(.secondary)
                } else {
                    Text(reply.body)
                    HStack(spacing: 6) {
                        Text("@\(reply.authorHandle)")
                        Text(reply.postedAt, style: .relative)
                        if reply.edited { Text("Edited") }
                    }
                    .font(.caption).foregroundStyle(.secondary)
                    HStack(spacing: 16) {
                        if let onReply { Button("Reply", action: onReply) }
                        Button("Quote", action: onQuote)
                        Menu("More") {
                            if isOwn {
                                Button("Edit Reply", action: onEdit)
                                Button("Revision history", action: onHistory)
                                Button("Delete Reply", role: .destructive, action: onDelete)
                            } else {
                                Button("Report Reply", role: .destructive, action: onReport)
                            }
                        }
                    }
                    .font(.caption).buttonStyle(.borderless)
                }
            }
        }
        .padding(.leading, nested ? 18 : 0)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(reply.isContinuityPlaceholder ? "Reply deleted" : "Reply by \(reply.authorHandle)\(reply.edited ? ", edited" : "")")
    }
}

struct SafetyReportTarget: Identifiable {
    let type, id, label: String
    let handle: String?
    init(type: String, id: String = "", label: String, handle: String? = nil) {
        self.type = type; self.id = id; self.label = label; self.handle = handle
    }
}

private struct SafetyReportMutationBody: Encodable {
    let targetType, targetID, targetHandle, category, details: String?
    enum CodingKeys: String, CodingKey {
        case category, details
        case targetType = "target_type", targetID = "target_id", targetHandle = "target_handle"
    }
}

private struct SafetyReportView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.dismiss) private var dismiss
    let target: SafetyReportTarget
    @State private var category = "harassment"
    @State private var details = ""
    @State private var isSubmitting = false
    @State private var errorMessage: String?
    private let categories = [
        ("harassment", "Harassment or threat"),
        ("hate", "Hateful content"),
        ("sexual_content", "Sexual or exploitative content"),
        ("violence_threat", "Violence or dangerous instructions"),
        ("privacy_location_safety", "Private information or location safety"),
        ("spam_scam", "Spam, scam, or manipulation"),
        ("other", "Other safety concern"),
    ]
    var body: some View {
        NavigationStack {
            Form {
                Section("Reporting") { Text(target.label).foregroundStyle(.secondary) }
                Section("Reason") {
                    Picker("Category", selection: $category) {
                        ForEach(categories, id: \.0) { value, title in Text(title).tag(value) }
                    }
                    TextField("Optional context", text: $details, axis: .vertical).lineLimit(3...8)
                    Text("\(details.count)/400").font(.caption).foregroundStyle(details.count > 400 ? .red : .secondary)
                }
                Section { Text("Reports go to the moderation queue. Use Hide separately if you also want this content removed from your view immediately.") }
                if let errorMessage { Label(errorMessage, systemImage: "exclamationmark.circle").foregroundStyle(.red) }
            }
            .navigationTitle("Safety Report")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Submit") { Task { await submit() } }.disabled(details.count > 400 || isSubmitting)
                }
            }
        }
    }
    private func submit() async {
        isSubmitting = true; errorMessage = nil; defer { isSubmitting = false }
        let context = details.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            let body = SafetyReportMutationBody(targetType: target.type, targetID: target.id.isEmpty ? nil : target.id, targetHandle: target.handle, category: category, details: context)
            let _: ReportDTO = try await app.client.send(OpenAPIPaths.createReport, method: "POST", body: body, idempotencyKey: UUID().uuidString)
            dismiss()
        } catch { errorMessage = error.localizedDescription }
    }
}

private struct ReplyEditorView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.dismiss) private var dismiss
    let reply: ReplyDTO
    let onSaved: (ReplyDTO) -> Void
    @State private var bodyText: String
    @State private var errorMessage: String?
    @State private var isSaving = false

    init(reply: ReplyDTO, onSaved: @escaping (ReplyDTO) -> Void) {
        self.reply = reply
        self.onSaved = onSaved
        _bodyText = State(initialValue: reply.body)
    }

    var body: some View {
        NavigationStack {
            Form {
                TextField("Reply", text: $bodyText, axis: .vertical).lineLimit(3...10)
                HStack {
                    Text("\(bodyText.count)/500").foregroundStyle(bodyText.count > 500 ? .red : .secondary)
                    Spacer()
                }.font(.caption)
                if let errorMessage { Label(errorMessage, systemImage: "exclamationmark.circle").foregroundStyle(.red) }
            }
            .navigationTitle("Edit Reply")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(bodyText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || bodyText.count > 500 || isSaving)
                }
            }
        }
    }

    private func save() async {
        guard let path = ReplyRevisionDescriptor(replyID: reply.id).editAPIPath else { return }
        isSaving = true; errorMessage = nil; defer { isSaving = false }
        do {
            let updated: ReplyDTO = try await app.client.send(path, method: "PATCH", body: ReplyMutationBody(body: bodyText.trimmingCharacters(in: .whitespacesAndNewlines), replyTo: nil), idempotencyKey: UUID().uuidString)
            onSaved(updated); dismiss()
        } catch { errorMessage = error.localizedDescription }
    }
}

private struct ReplyHistoryView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.dismiss) private var dismiss
    let reply: ReplyDTO
    @State private var history: [ReplyRevisionDTO] = []
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            List {
                if history.isEmpty && errorMessage == nil { ProgressView("Loading history") }
                ForEach(history) { revision in
                    VStack(alignment: .leading, spacing: 5) {
                        Text(revision.body)
                        HStack { Text("Revision \(revision.revisionNumber)"); Spacer(); Text(revision.createdAt, style: .relative) }
                            .font(.caption).foregroundStyle(.secondary)
                    }
                }
                if let errorMessage { Label(errorMessage, systemImage: "exclamationmark.circle").foregroundStyle(.red) }
            }
            .navigationTitle("Revision History")
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } } }
            .task { await load() }
        }
    }

    private func load() async {
        guard let path = ReplyRevisionDescriptor(replyID: reply.id).historyAPIPath else { return }
        do { let response: ReplyHistoryDTO = try await app.client.get(path); history = response.revisions }
        catch { errorMessage = error.localizedDescription }
    }
}

struct MediaGalleryView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @AppStorage("dataSaver") private var dataSaver = false
    @AppStorage("wifiOnlyAutoplay") private var wifiOnlyAutoplay = false
    @ObservedObject private var network = NetworkPathObserver.shared
    let assets: [MediaAssetDTO]
    let echoTitle: String
    let caption: String?
    var body: some View {
        if !assets.isEmpty {
            Section("Media") {
                ForEach(Array(assets.enumerated()), id: \.element.id) { index, asset in
                    MediaAssetView(
                        asset: asset,
                        publicURL: app.client.publicURL(asset.publicURL),
                        thumbnailURL: app.client.publicURL(asset.thumbnailURL),
                        position: index + 1,
                        total: assets.count,
                        echoTitle: echoTitle,
                        caption: caption,
                        policy: MediaPlaybackPolicy(dataSaverEnabled: dataSaver, wifiOnlyAutoplay: wifiOnlyAutoplay, isOnWiFi: network.isOnWiFi, reduceMotionEnabled: reduceMotion)
                    )
                }
            }
        }
    }
}

private struct MediaAssetView: View {
    let asset: MediaAssetDTO
    let publicURL, thumbnailURL: URL?
    let position, total: Int
    let echoTitle: String
    let caption: String?
    let policy: MediaPlaybackPolicy
    @State private var explicitlyLoaded = false

    private var canRender: Bool { explicitlyLoaded || !policy.requiresExplicitLoad }
    private var mediaName: String {
        switch asset.mediaType { case "image": "Photo"; case "video": "Video"; case "audio": "Audio"; default: "Media" }
    }
    private var accessibleDescription: String {
        let positionText = total > 1 ? " \(position) of \(total)" : ""
        let context = caption?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false ? caption! : echoTitle
        return "\(mediaName)\(positionText) for \(context)"
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if !canRender {
                Button { explicitlyLoaded = true } label: { Label("Load \(mediaName.lowercased())", systemImage: "arrow.down.circle") }
                    .accessibilityHint("Loads this media using network data")
            } else {
                switch asset.mediaType {
                case "image":
                    AsyncImage(url: thumbnailURL ?? publicURL) { phase in
                        switch phase {
                        case let .success(image): image.resizable().scaledToFit()
                        case .failure: Label("Image unavailable", systemImage: "photo.badge.exclamationmark")
                        default: ProgressView("Loading image")
                        }
                    }
                    .frame(maxHeight: 420)
                    .accessibilityLabel(accessibleDescription)
                case "video":
                    if let publicURL { VideoMediaView(url: publicURL, accessibilityDescription: accessibleDescription, autoplay: policy.allowsAutoplay) }
                    else { Label("Video unavailable", systemImage: "video.slash") }
                case "audio":
                    if let publicURL { AudioMediaView(url: publicURL, duration: asset.durationSeconds, accessibilityDescription: accessibleDescription) }
                    else { Label("Audio unavailable", systemImage: "waveform.badge.exclamationmark") }
                default:
                    Label("Media unavailable", systemImage: "exclamationmark.triangle")
                }
            }
            if let transcript = asset.transcript, !transcript.isEmpty {
                DisclosureGroup("Transcript") {
                    Text(transcript).textSelection(.enabled)
                        .accessibilityLabel("Transcript: \(transcript)")
                }
                .accessibilityHint("Shows the text transcript for this \(mediaName.lowercased())")
            } else if asset.mediaType == "audio" || asset.mediaType == "video" {
                Label("Transcript not available", systemImage: "captions.bubble")
                    .font(.caption).foregroundStyle(.secondary)
                    .accessibilityLabel("No transcript is available for this \(mediaName.lowercased())")
            }
        }
        .accessibilityElement(children: .contain)
    }
}

private struct VideoMediaView: View {
    @State private var player: AVPlayer
    let accessibilityDescription: String
    let autoplay: Bool
    init(url: URL, accessibilityDescription: String, autoplay: Bool) { self.accessibilityDescription = accessibilityDescription; self.autoplay = autoplay; _player = State(initialValue: AVPlayer(url: url)) }
    var body: some View {
        VideoPlayer(player: player)
            .frame(minHeight: 220)
            .accessibilityLabel(accessibilityDescription)
            .accessibilityHint(autoplay ? "Playback may start automatically under your media settings" : "Playback starts only when you press play")
            .onAppear { if autoplay { player.play() } }
            .onDisappear { player.pause() }
    }
}

private struct AudioMediaView: View {
    let duration: Double?
    let accessibilityDescription: String
    @State private var player: AVPlayer
    @State private var isPlaying = false
    init(url: URL, duration: Double?, accessibilityDescription: String) { self.duration = duration; self.accessibilityDescription = accessibilityDescription; _player = State(initialValue: AVPlayer(url: url)) }
    var body: some View {
        HStack {
            Button {
                if isPlaying { player.pause() } else { player.play() }
                isPlaying.toggle()
            } label: {
                Label(isPlaying ? "Pause audio" : "Play audio", systemImage: isPlaying ? "pause.circle.fill" : "play.circle.fill")
            }
            Spacer()
            if let duration { Text(Duration.seconds(duration).formatted(.time(pattern: .minuteSecond))) }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(accessibilityDescription)
        .accessibilityHint(isPlaying ? "Audio is playing" : "Audio is paused")
        .onDisappear { player.pause(); isPlaying = false }
    }
}

@MainActor private final class NetworkPathObserver: ObservableObject {
    static let shared = NetworkPathObserver()
    @Published private(set) var isOnWiFi = false
    private let monitor = NWPathMonitor()
    private let queue = DispatchQueue(label: "tv.subcult.echotrails.network-path")

    private init() {
        monitor.pathUpdateHandler = { [weak self] path in
            let isOnWiFi = path.status == .satisfied && path.usesInterfaceType(.wifi)
            DispatchQueue.main.async { self?.isOnWiFi = isOnWiFi }
        }
        monitor.start(queue: queue)
    }
}

struct ActivityView: View {
    @EnvironmentObject private var app: AppModel
    var body: some View {
        List {
            ForEach(app.activity) { item in
                Button { Task { await app.openActivity(item) } } label: {
                    VStack(alignment: .leading) {
                        HStack { Text(item.preview).font(.body); Spacer(); if item.readAt == nil { Circle().fill(.tint).frame(width: 8, height: 8).accessibilityLabel("Unread") } }
                        HStack {
                            Text(item.createdAt, style: .relative)
                            Spacer()
                            if ActivityTargetKind.resolve(notificationKind: item.kind) != .none { Label("Open", systemImage: "chevron.right") }
                        }.font(.caption).foregroundStyle(.secondary)
                    }.accessibilityElement(children: .combine)
                }
                .buttonStyle(.plain)
            }
            if app.activityNextCursor != nil {
                Button { Task { await app.loadActivity(reset: false) } } label: {
                    HStack { Spacer(); if app.isLoadingActivity { ProgressView() } else { Text("Load more Activity") }; Spacer() }
                }.disabled(app.isLoadingActivity)
            }
        }
        .navigationTitle("Activity")
        .refreshable { await app.loadActivity() }
        .overlay { if app.activity.isEmpty && !app.isLoadingActivity { ContentUnavailableView("No activity yet", systemImage: "bell", description: Text("Replies, reactions, Stitches, Place alerts, uploads, and moderation updates appear here.")) } }
    }
}

struct ProfileView: View {
    @EnvironmentObject private var app: AppModel
    @Query(sort: \DurableOutboxItem.createdAt) private var outbox: [DurableOutboxItem]
    var body: some View {
        List {
            if let profile = app.profile {
                Section {
                    HStack(spacing: 14) {
                        ProfileAvatar(url: app.client.publicURL(profile.avatarURL), name: profile.displayName, size: 64)
                        VStack(alignment: .leading) {
                            Text(profile.displayName).font(.title2)
                            Text("@\(profile.handle)").foregroundStyle(.secondary)
                        }
                    }
                    if let bio = profile.bio, !bio.isEmpty { Text(bio) }
                    NavigationLink("Edit profile and avatar") { ProfileEditorView(profile: profile) }
                }
            }
            Section("Offline & uploads") {
                LabeledContent("Pending outbox", value: "\(outbox.filter { $0.state != "completed" }.count)")
                NavigationLink("Upload & publishing status") { UploadStatusView() }
                Toggle("Wi-Fi-only autoplay", isOn: Binding(get: { UserDefaults.standard.bool(forKey: "wifiOnlyAutoplay") }, set: { UserDefaults.standard.set($0, forKey: "wifiOnlyAutoplay") }))
                Toggle("Reduce media data", isOn: Binding(get: { UserDefaults.standard.bool(forKey: "dataSaver") }, set: { UserDefaults.standard.set($0, forKey: "dataSaver") }))
            }
            Section("Safety & privacy") {
                NavigationLink("Safety & Appeals") { AppealsView() }
                NavigationLink("Notification settings") { NotificationSettingsView() }
                NavigationLink("Signed-in devices") { SessionManagementView() }
                NavigationLink("Export or delete account") { PrivacySettingsView() }
            }
            Section { Button("Sign out", role: .destructive) { Task { await app.signOut() } } }
        }
        .navigationTitle("Profile")
        .task { await app.retryPendingAvatarSelection() }
    }
}

struct UploadStatusView: View {
    @EnvironmentObject private var app: AppModel
    @Query(sort: \DurableOutboxItem.createdAt, order: .reverse) private var outbox: [DurableOutboxItem]
    @Query(sort: \CachedUpload.updatedAt, order: .reverse) private var uploads: [CachedUpload]
    @State private var echoes: [String: EchoDTO] = [:]
    @State private var refreshError: String?
    @State private var deletingEchoID: String?

    var body: some View {
        List {
            Section("Publishing") {
                if echoIDs.isEmpty { Text("No media Echoes are processing.").foregroundStyle(.secondary) }
                ForEach(echoIDs, id: \.self) { echoID in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(echoes[echoID]?.title ?? "Echo \(echoID.prefix(8))")
                        let presentation = ModerationStatusPresentation(rawValue: echoes[echoID]?.status ?? localStatus(for: echoID))
                        Label(presentation.title, systemImage: presentation.isAdverse ? "exclamationmark.shield" : "arrow.triangle.2.circlepath")
                            .font(.caption).foregroundStyle(.secondary)
                    }.accessibilityElement(children: .combine)
                    Button("Delete this Echo", role: .destructive) { deletingEchoID = echoID }
                }
                if let refreshError { Label(refreshError, systemImage: "wifi.exclamationmark").foregroundStyle(.secondary) }
            }
            Section("Attachments") {
                if uploads.isEmpty { Text("No attachment uploads yet.").foregroundStyle(.secondary) }
                ForEach(uploads, id: \.uploadID) { upload in
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Attachment \(upload.sortOrder + 1)")
                        Text(upload.status.replacingOccurrences(of: "_", with: " ").capitalized)
                            .font(.caption).foregroundStyle(.secondary)
                    }.accessibilityElement(children: .combine)
                }
            }
            Section("Durable outbox") {
                if outbox.isEmpty { Text("The outbox is empty.").foregroundStyle(.secondary) }
                ForEach(outbox.prefix(50)) { item in
                    VStack(alignment: .leading, spacing: 5) {
                        HStack { Text(item.operation.replacingOccurrences(of: "_", with: " ").capitalized); Spacer(); Text(item.state.capitalized).foregroundStyle(.secondary) }
                        Text("Attempt \(item.attemptCount) of \(DurableOutboxPolicy.maximumAttempts)").font(.caption)
                        if let error = item.lastError { Text(error).font(.caption).foregroundStyle(.red) }
                        if item.state == "failed" {
                            Button("Retry") { Task { await app.retryOutboxItem(item.id); await refresh() } }
                        }
                    }.accessibilityElement(children: .contain)
                }
            }
        }
        .navigationTitle("Upload Status")
        .task { await refresh() }
        .refreshable { await app.processOutbox(); await app.recoverBackgroundUploads(); await refresh() }
        .confirmationDialog("Delete this Echo?", isPresented: Binding(get: { deletingEchoID != nil }, set: { if !$0 { deletingEchoID = nil } })) {
            Button("Delete Echo", role: .destructive) { Task { await deletePendingEcho() } }
            Button("Cancel", role: .cancel) { deletingEchoID = nil }
        } message: { Text("Uploads and local retry files for this Echo will be removed after the server confirms deletion.") }
    }

    private var echoIDs: [String] { Array(Set(uploads.map(\.echoID))).sorted() }
    private func localStatus(for echoID: String) -> String {
        uploads.filter { $0.echoID == echoID }.contains { $0.status == "retry_required" || $0.status == "failed" } ? "failed" : "processing"
    }
    private func refresh() async {
        refreshError = nil
        for echoID in echoIDs {
            guard let safe = ContractPath.safeSegment(echoID) else { continue }
            do { echoes[echoID] = try await app.client.get(OpenAPIPaths.getEcho(echoId: safe)) }
            catch { refreshError = error.localizedDescription }
        }
    }
    private func deletePendingEcho() async {
        guard let id = deletingEchoID else { return }
        if await app.deleteEcho(id: id) { echoes[id] = nil }
        deletingEchoID = nil
    }
}

private struct ProfileAvatar: View {
    let url: URL?
    let name: String
    let size: CGFloat
    var body: some View {
        Group {
            if let url {
                AsyncImage(url: url) { phase in
                    if let image = phase.image { image.resizable().scaledToFill() }
                    else if phase.error != nil { fallback }
                    else { ProgressView() }
                }
            } else { fallback }
        }
        .frame(width: size, height: size)
        .clipShape(Circle())
        .accessibilityLabel("Profile photo for \(name)")
    }
    private var fallback: some View {
        Image(systemName: "person.crop.circle.fill").resizable().foregroundStyle(.secondary)
    }
}

struct ProfileEditorView: View {
    @EnvironmentObject private var app: AppModel
    @State private var handle: String
    @State private var displayName: String
    @State private var bio: String
    @State private var avatarSelection: PhotosPickerItem?
    @State private var isUploadingAvatar = false
    @State private var avatarMessage: String?

    init(profile: MeDTO) {
        _handle = State(initialValue: profile.handle)
        _displayName = State(initialValue: profile.displayName)
        _bio = State(initialValue: profile.bio ?? "")
    }

    var body: some View {
        Form {
            Section("Public profile") {
                TextField("Handle", text: $handle).textInputAutocapitalization(.never).autocorrectionDisabled()
                TextField("Display name", text: $displayName)
                TextField("Bio", text: $bio, axis: .vertical).lineLimit(3...8)
                Text("Handles can be changed only once every 30 days.").font(.caption).foregroundStyle(.secondary)
                Button("Save profile") { Task { await app.saveProfile(handle: handle, displayName: displayName, bio: bio) } }
                    .disabled(!valid || app.isWorking)
            }
            Section("Profile photo") {
                if let profile = app.profile {
                    ProfileAvatar(url: app.client.publicURL(profile.avatarURL), name: profile.displayName, size: 88)
                }
                PhotosPicker(selection: $avatarSelection, matching: .images) {
                    Label("Choose a photo", systemImage: "photo")
                }
                .disabled(isUploadingAvatar)
                if isUploadingAvatar { ProgressView("Uploading and checking photo") }
                if let avatarMessage { Text(avatarMessage).foregroundStyle(.secondary).accessibilityLabel(avatarMessage) }
                if app.profile?.avatarURL != nil {
                    Button("Remove profile photo", role: .destructive) { Task { _ = await app.removeAvatar() } }
                }
                Text("The photo stays private until malware scanning, metadata removal, and safety review finish.")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Edit Profile")
        .onChange(of: avatarSelection) { Task { await uploadSelection() } }
        .task { await app.retryPendingAvatarSelection() }
    }

    private var valid: Bool {
        handle.range(of: "^[A-Za-z][A-Za-z0-9_]{2,29}$", options: .regularExpression) != nil &&
        displayName.count <= 80 && bio.count <= 160
    }

    private func uploadSelection() async {
        guard let avatarSelection else { return }
        isUploadingAvatar = true; avatarMessage = nil
        defer { isUploadingAvatar = false; self.avatarSelection = nil }
        do {
            guard let data = try await avatarSelection.loadTransferable(type: Data.self), data.count <= 10 * 1024 * 1024 else {
                avatarMessage = "Choose an image no larger than 10 MB."
                return
            }
            let contentType: String
            if data.starts(with: [0x89, 0x50, 0x4e, 0x47]) { contentType = "image/png" }
            else if data.count > 12, String(data: data[4..<12], encoding: .ascii)?.contains("ftyp") == true { contentType = "image/heic" }
            else { contentType = "image/jpeg" }
            let result = try await app.uploadAvatar(data: data, contentType: contentType)
            avatarMessage = result == .selected ? "Profile photo updated." : "Photo uploaded. It will appear after safety review."
        } catch { avatarMessage = error.localizedDescription }
    }
}

struct PublicProfileView: View {
    @EnvironmentObject private var app: AppModel
    let handle: String
    @State private var profile: PublicProfileDTO?
    @State private var echoes: [EchoDTO] = []
    @State private var nextCursor: String?
    @State private var isLoadingEchoes = false
    @State private var errorMessage: String?
    @State private var showingBlockConfirmation = false
    @State private var reportTarget: SafetyReportTarget?
    var body: some View {
        List {
            if let profile {
                Section {
                    ProfileAvatar(url: app.client.publicURL(profile.avatarURL), name: profile.displayName, size: 88)
                    Text(profile.displayName).font(.title2)
                    Text("@\(profile.handle)").foregroundStyle(.secondary)
                    if let bio = profile.bio, !bio.isEmpty { Text(bio) }
                }
                Section { Text("Only this public profile and safely published Echoes are visible. Email, subscriptions, devices, and precise locations are private.") }
                Section("Published Echoes") {
                    if echoes.isEmpty && !isLoadingEchoes {
                        Text("No published Echoes yet.").foregroundStyle(.secondary)
                    }
                    ForEach(echoes) { echo in
                        NavigationLink { EchoDetailView(echo: echo) } label: {
                            VStack(alignment: .leading, spacing: 3) {
                                Text(echo.title).font(.headline)
                                if let date = echo.publishedAt ?? echo.createdAt {
                                    Text(date, style: .relative).font(.caption).foregroundStyle(.secondary)
                                }
                            }
                        }
                    }
                    if nextCursor != nil {
                        Button { Task { await loadEchoes(reset: false) } } label: {
                            HStack { Spacer(); if isLoadingEchoes { ProgressView() } else { Text("Load more Echoes") }; Spacer() }
                        }.disabled(isLoadingEchoes)
                    }
                }
                if profile.handle != app.profile?.handle {
                    Section("Safety") {
                        Button("Block @\(profile.handle)", role: .destructive) { showingBlockConfirmation = true }
                        Button("Report @\(profile.handle)", role: .destructive) { reportTarget = SafetyReportTarget(type: "user", label: "@\(profile.handle)", handle: profile.handle) }
                    }
                }
            } else if let errorMessage {
                ContentUnavailableView("Profile unavailable", systemImage: "person.crop.circle.badge.exclamationmark", description: Text(errorMessage))
            } else { ProgressView("Loading profile") }
        }
        .navigationTitle("Profile")
        .task { await load() }
        .sheet(item: $reportTarget) { target in SafetyReportView(target: target) }
        .confirmationDialog("Block @\(profile?.handle ?? handle)?", isPresented: $showingBlockConfirmation) {
            Button("Block user", role: .destructive) { Task { await block() } }
            Button("Cancel", role: .cancel) {}
        } message: { Text("Their Echoes, Replies, and notifications will be hidden from you.") }
    }
    private func load() async {
        guard let safe = ContractPath.safeSegment(handle) else { errorMessage = "Invalid profile handle."; return }
        do {
            async let loadedProfile: PublicProfileDTO = app.client.get(OpenAPIPaths.getPublicProfile(handle: safe), authenticated: false)
            profile = try await loadedProfile
            await loadEchoes(reset: true)
        }
        catch { errorMessage = error.localizedDescription }
    }
    private func loadEchoes(reset: Bool) async {
        guard !isLoadingEchoes, let safe = ContractPath.safeSegment(handle) else { return }
        let cursor = reset ? nil : nextCursor
        if !reset && cursor == nil { return }
        var path = "\(OpenAPIPaths.listPublicProfileEchoes(handle: safe))?limit=20"
        if let cursor { path += "&cursor=" + (cursor.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? "") }
        isLoadingEchoes = true; defer { isLoadingEchoes = false }
        do {
            let page: PublicProfileEchoesDTO = try await app.client.get(path)
            if reset { echoes = page.echoes }
            else {
                let existing = Set(echoes.map(\.id))
                echoes.append(contentsOf: page.echoes.filter { !existing.contains($0.id) })
            }
            nextCursor = page.nextCursor?.isEmpty == false ? page.nextCursor : nil
        } catch { errorMessage = error.localizedDescription }
    }
    private func block() async {
        do {
            try await app.client.sendNoContent(OpenAPIPaths.blockUser, method: "POST", body: ["target_handle": profile?.handle ?? handle], idempotencyKey: UUID().uuidString)
            profile = nil
            errorMessage = "This user is blocked."
            await app.loadDiscovery()
        } catch { errorMessage = error.localizedDescription }
    }
}

struct NotificationSettingsView: View {
    @EnvironmentObject private var app: AppModel
    @State private var direct = true; @State private var places = true; @State private var moderation = true; @State private var uploads = true
    @State private var isSaving = false
    @State private var didSave = false
    var body: some View {
        Form {
            Toggle("Replies, reactions, and Stitches", isOn: $direct)
            Toggle("Subscribed Place alerts", isOn: $places)
            Toggle("Moderation updates", isOn: $moderation)
            Toggle("Upload status", isOn: $uploads)
            Button { Task { await save() } } label: { if isSaving { ProgressView() } else { Text("Save notification settings") } }.disabled(isSaving)
            if didSave { Label("Saved", systemImage: "checkmark.circle.fill").foregroundStyle(.green) }
        }
        .navigationTitle("Notifications")
        .task { if app.notificationPreferences == nil { await app.loadNotificationPreferences() }; applyServerValues() }
    }
    private func applyServerValues() {
        guard let values = app.notificationPreferences else { return }
        direct = values.directActivityEnabled; places = values.placeAlertsEnabled; moderation = values.moderationEnabled; uploads = values.uploadStatusEnabled
    }
    private func save() async {
        isSaving = true; didSave = false
        didSave = await app.saveNotificationPreferences(direct: direct, places: places, moderation: moderation, uploads: uploads)
        isSaving = false
        if didSave { applyServerValues() }
    }
}

struct PrivacySettingsView: View {
    @EnvironmentObject private var app: AppModel
    @State private var analyticsOptOut = false
    @State private var loadedAnalyticsPreference = false
    @State private var exportJob: PrivacyJobDTO?
    @State private var deletionJob: PrivacyJobDTO?
    @State private var exportFile: URL?
    @State private var showingDeletionConfirmation = false
    @State private var errorMessage: String?
    var body: some View {
        Form {
            Section("Analytics") {
                Toggle("Opt out of first-party product analytics", isOn: $analyticsOptOut)
                    .onChange(of: analyticsOptOut) {
                        if loadedAnalyticsPreference {
                            Task { try? await app.client.sendNoContent(OpenAPIPaths.setAnalyticsOptOut, method: "POST", body: ["opt_out": analyticsOptOut], idempotencyKey: UUID().uuidString) }
                        }
                    }
            }
            Section("Your data") {
                Button("Request data export") { Task { await requestExport() } }
                if let exportJob {
                    LabeledContent("Export", value: exportJob.status.replacingOccurrences(of: "_", with: " ").capitalized)
                    if exportJob.downloadURL != nil { Button("Prepare private export") { Task { await downloadExport() } } }
                    else { Button("Refresh export status") { Task { await refreshExport() } } }
                    if let exportFile { ShareLink(item: exportFile) { Label("Share or save export", systemImage: "square.and.arrow.up") } }
                }
                Button("Delete account", role: .destructive) { showingDeletionConfirmation = true }
                if let deletionJob { LabeledContent("Deletion", value: deletionJob.status.capitalized) }
            }
            Section { Text("Exports are private, expire after seven days, and never include raw authentication or push tokens. Account deletion removes stored media objects and revokes every device session.") }
            if let errorMessage { Section { Label(errorMessage, systemImage: "exclamationmark.circle").foregroundStyle(.red) } }
        }
        .navigationTitle("Account privacy")
        .task { await loadAnalyticsPreference() }
        .confirmationDialog("Permanently delete your EchoTrails account?", isPresented: $showingDeletionConfirmation) {
            Button("Delete account and media", role: .destructive) { Task { await requestDeletion() } }
            Button("Cancel", role: .cancel) {}
        } message: { Text("This queues permanent deletion of your profile, Echoes, Replies, media, sessions, and push tokens. This cannot be undone after processing completes.") }
    }
    private func loadAnalyticsPreference() async {
        struct Preference: Decodable { let optOut: Bool; enum CodingKeys: String, CodingKey { case optOut = "opt_out" } }
        if let value: Preference = try? await app.client.get(OpenAPIPaths.getAnalyticsOptOut) { analyticsOptOut = value.optOut }
        loadedAnalyticsPreference = true
    }
    private func requestExport() async {
        do { exportJob = try await app.client.send(OpenAPIPaths.requestPrivacyExport, method: "POST", body: EmptyRequest(), idempotencyKey: UUID().uuidString); errorMessage = nil }
        catch { errorMessage = error.localizedDescription }
    }
    private func refreshExport() async {
        guard let exportJob else { return }
        do { self.exportJob = try await app.client.get(OpenAPIPaths.getPrivacyExport(jobId: exportJob.jobID)); errorMessage = nil }
        catch { errorMessage = error.localizedDescription }
    }
    private func downloadExport() async {
        guard let job = exportJob, let path = job.downloadURL else { return }
        do { exportFile = try await app.client.downloadAuthenticated(path, filename: "EchoTrails-export-\(job.jobID).json"); errorMessage = nil }
        catch { errorMessage = error.localizedDescription }
    }
    private func requestDeletion() async {
        do {
            deletionJob = try await app.client.send(OpenAPIPaths.requestAccountDeletion, method: "POST", body: EmptyRequest(), idempotencyKey: UUID().uuidString)
            errorMessage = nil
            await app.completeAccountDeletionLocally()
        }
        catch { errorMessage = error.localizedDescription }
    }
}
struct PrivacyJobDTO: Decodable { let jobID, status: String; let downloadURL: String?; enum CodingKeys: String, CodingKey { case jobID = "job_id", status; case downloadURL = "download_url" } }

struct AccountSessionDTO: Decodable, Identifiable { let id, deviceName: String; let createdAt, lastSeenAt, expiresAt: Date?; enum CodingKeys: String, CodingKey { case id; case deviceName = "device_name", createdAt = "created_at", lastSeenAt = "last_seen_at", expiresAt = "expires_at" } }
struct AccountSessionsDTO: Decodable {
    let sessions: [AccountSessionDTO]
    let nextCursor: String?
    enum CodingKeys: String, CodingKey { case sessions; case nextCursor = "next_cursor" }
}
struct SessionManagementView: View {
    @EnvironmentObject private var app: AppModel
    @State private var sessions: [AccountSessionDTO] = []
    var body: some View {
        List {
            ForEach(sessions) { session in
                VStack(alignment: .leading) {
                    Text(session.deviceName)
                    if let lastSeen = session.lastSeenAt {
                        HStack { Text("Last active"); Text(lastSeen, style: .relative) }.font(.caption).foregroundStyle(.secondary)
                    }
                    Button("Revoke this device", role: .destructive) {
                        Task {
                            try? await app.client.sendNoContent(OpenAPIPaths.revokeAccountSession(targetId: session.id), method: "DELETE", body: Optional<EmptyRequest>.none, idempotencyKey: UUID().uuidString)
                            await load()
                        }
                    }
                }
            }
        }
        .navigationTitle("Signed-in devices")
        .task { await load() }
    }
    private func load() async {
        do {
            var values: [AccountSessionDTO] = []
            var cursor: String?
            var seen = Set<String>()
            repeat {
                var path = "\(OpenAPIPaths.listAccountSessions)?limit=50"
                if let cursor { path += "&cursor=" + (cursor.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? "") }
                let response: AccountSessionsDTO = try await app.client.get(path)
                values.append(contentsOf: response.sessions)
                if let next = response.nextCursor, !next.isEmpty, seen.insert(next).inserted { cursor = next } else { cursor = nil }
            } while cursor != nil
            sessions = values
        } catch { app.errorMessage = error.localizedDescription }
    }
}
