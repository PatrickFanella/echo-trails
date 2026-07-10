import EchoTrailsShell
import SwiftUI

struct AppealsView: View {
    @EnvironmentObject private var app: AppModel
    @State private var appeals: [AppealDTO] = []
    @State private var nextCursor: String?
    @State private var isLoading = false
    @State private var showingComposer = false
    @State private var errorMessage: String?

    var body: some View {
        List {
            Section {
                Text("You can ask for another review of a moderation action affecting your account or content. Your appeal and its outcome are private.")
                    .font(.callout)
            }
            if appeals.isEmpty && !isLoading && errorMessage == nil {
                ContentUnavailableView("No appeals", systemImage: "checkmark.shield", description: Text("If a moderation action is eligible, open its Activity notice to appeal."))
            }
            ForEach(appeals) { appeal in
                NavigationLink { AppealDetailView(appeal: appeal) { updated in replace(updated) } } label: {
                    VStack(alignment: .leading, spacing: 5) {
                        Label(appeal.statusPresentation.title, systemImage: statusSymbol(appeal.statusPresentation))
                            .font(.headline)
                        Text(appeal.body).lineLimit(2)
                        Text(appeal.createdAt, style: .date).font(.caption).foregroundStyle(.secondary)
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityLabel("\(appeal.statusPresentation.title). Appeal submitted \(appeal.createdAt.formatted(date: .long, time: .omitted)). \(appeal.body)")
                    .accessibilityHint("Opens appeal details")
                }
            }
            if nextCursor != nil {
                Button { Task { await load(reset: false) } } label: {
                    HStack { Spacer(); if isLoading { ProgressView() } else { Text("Load more appeals") }; Spacer() }
                }.disabled(isLoading)
            }
            if let errorMessage { Label(errorMessage, systemImage: "wifi.exclamationmark").foregroundStyle(.secondary) }
        }
        .navigationTitle("Safety & Appeals")
        .toolbar { ToolbarItem(placement: .primaryAction) { Button { showingComposer = true } label: { Label("New appeal", systemImage: "square.and.pencil") } } }
        .sheet(isPresented: $showingComposer) { NavigationStack { AppealComposerView(moderationActionID: nil) { created in appeals.insert(created, at: 0) } } }
        .task { if appeals.isEmpty { await load() } }
        .refreshable { await load() }
    }

    private func load(reset: Bool = true) async {
        guard !isLoading else { return }
        let cursor = reset ? nil : nextCursor
        if !reset && cursor == nil { return }
        guard let path = AppealListRequest(cursor: cursor).apiPath else { return }
        isLoading = true; errorMessage = nil; defer { isLoading = false }
        do {
            let page: AppealPageDTO = try await app.client.get(path)
            if reset { appeals = page.appeals }
            else {
                let existing = Set(appeals.map(\.id))
                appeals.append(contentsOf: page.appeals.filter { !existing.contains($0.id) })
            }
            nextCursor = page.nextCursor?.isEmpty == false ? page.nextCursor : nil
        } catch { errorMessage = error.localizedDescription }
    }

    private func replace(_ updated: AppealDTO) {
        if let index = appeals.firstIndex(where: { $0.id == updated.id }) { appeals[index] = updated }
    }

    private func statusSymbol(_ status: AppealStatus) -> String {
        switch status {
        case .open: "clock.badge.questionmark"
        case .accepted: "checkmark.shield.fill"
        case .rejected: "xmark.shield"
        case .withdrawn: "arrow.uturn.backward.circle"
        }
    }
}

struct AppealComposerView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.dismiss) private var dismiss
    let initialActionID: String?
    let onCreated: ((AppealDTO) -> Void)?
    @State private var actionID: String
    @State private var bodyText = ""
    @State private var isSubmitting = false
    @State private var errorMessage: String?

    init(moderationActionID: String?, onCreated: ((AppealDTO) -> Void)? = nil) {
        initialActionID = moderationActionID
        self.onCreated = onCreated
        _actionID = State(initialValue: moderationActionID ?? "")
    }

    var body: some View {
        Form {
            Section("Moderation action") {
                if initialActionID != nil {
                    LabeledContent("Action reference", value: actionID)
                } else {
                    TextField("Action reference", text: $actionID)
                        .textInputAutocapitalization(.never).autocorrectionDisabled()
                    Text("The action reference appears in the moderation notice in Activity. Opening that notice fills it in automatically.")
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
            Section("Why should we review this again?") {
                TextField("Add relevant context", text: $bodyText, axis: .vertical).lineLimit(5...14)
                    .accessibilityHint("Appeal explanation, up to 2,000 characters")
                HStack { Spacer(); Text("\(bodyText.count)/2000") }
                    .font(.caption).foregroundStyle(bodyText.count > 2_000 ? .red : .secondary)
            }
            if let errorMessage { Label(errorMessage, systemImage: "exclamationmark.circle").foregroundStyle(.red) }
            Section { Text("A moderator will review the action and this explanation. Safety evidence remains private.") }
        }
        .navigationTitle("Submit Appeal")
        .toolbar {
            ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
            ToolbarItem(placement: .confirmationAction) {
                Button("Submit") { Task { await submit() } }.disabled(!draft.isValid || isSubmitting)
            }
        }
    }

    private var draft: AppealDraft { AppealDraft(moderationActionID: actionID, body: bodyText) }

    private func submit() async {
        guard draft.isValid else { return }
        isSubmitting = true; errorMessage = nil; defer { isSubmitting = false }
        do {
            let created: AppealDTO = try await app.client.send(OpenAPIPaths.createAppeal, method: "POST", body: AppealMutationDTO(moderationActionID: draft.moderationActionID, body: draft.body), idempotencyKey: UUID().uuidString)
            onCreated?(created); dismiss()
        } catch { errorMessage = error.localizedDescription }
    }
}

struct AppealDetailView: View {
    @EnvironmentObject private var app: AppModel
    @State private var appeal: AppealDTO
    @State private var bodyText: String
    @State private var isEditing = false
    @State private var isSaving = false
    @State private var errorMessage: String?
    let onUpdated: (AppealDTO) -> Void

    init(appeal: AppealDTO, onUpdated: @escaping (AppealDTO) -> Void) {
        _appeal = State(initialValue: appeal)
        _bodyText = State(initialValue: appeal.body)
        self.onUpdated = onUpdated
    }

    var body: some View {
        Form {
            Section("Status") {
                Label(appeal.statusPresentation.title, systemImage: appeal.statusPresentation == .open ? "clock.badge.questionmark" : "shield")
                    .accessibilityLabel("Appeal status: \(appeal.statusPresentation.title)")
                LabeledContent("Submitted", value: appeal.createdAt.formatted(date: .long, time: .shortened))
            }
            Section("Your explanation") {
                if isEditing {
                    TextField("Appeal explanation", text: $bodyText, axis: .vertical).lineLimit(5...14)
                    Text("\(bodyText.count)/2000").font(.caption).foregroundStyle(bodyText.count > 2_000 ? .red : .secondary)
                } else { Text(appeal.body).textSelection(.enabled) }
            }
            Section("Reference") { Text(appeal.moderationActionID).font(.caption.monospaced()).textSelection(.enabled) }
            if let errorMessage { Label(errorMessage, systemImage: "exclamationmark.circle").foregroundStyle(.red) }
        }
        .navigationTitle("Appeal Details")
        .toolbar {
            if appeal.statusPresentation.isEditable {
                ToolbarItem(placement: .primaryAction) {
                    if isEditing { Button("Save") { Task { await save() } }.disabled(!validBody || isSaving) }
                    else { Button("Edit") { isEditing = true } }
                }
            }
        }
    }

    private var validBody: Bool {
        let trimmed = bodyText.trimmingCharacters(in: .whitespacesAndNewlines)
        return !trimmed.isEmpty && trimmed.count <= 2_000
    }

    private func save() async {
        guard validBody, let id = ContractPath.safeSegment(appeal.id) else { return }
        isSaving = true; errorMessage = nil; defer { isSaving = false }
        do {
            let updated: AppealDTO = try await app.client.send(OpenAPIPaths.updateAppeal(targetId: id), method: "PATCH", body: AppealMutationDTO(moderationActionID: nil, body: bodyText.trimmingCharacters(in: .whitespacesAndNewlines)), idempotencyKey: UUID().uuidString)
            appeal = updated; bodyText = updated.body; isEditing = false; onUpdated(updated)
        } catch { errorMessage = error.localizedDescription }
    }
}
