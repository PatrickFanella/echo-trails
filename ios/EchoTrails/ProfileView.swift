import SwiftUI

struct ProfileView: View {
    @Environment(EchoStore.self) private var store
    @State private var selectedEcho: Echo?
    @State private var isEditingProfile = false

    private var myEchoes: [Echo] {
        store.echoes(by: store.currentUser.id)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 0) {
                    profileHeader

                    statsRow
                        .padding(.horizontal)
                        .padding(.top, 20)

                    VStack(alignment: .leading, spacing: 12) {
                        Text("Your Echoes")
                            .font(.headline)
                            .padding(.horizontal)
                            .padding(.top, 28)

                        if myEchoes.isEmpty {
                            emptyState
                        } else {
                            LazyVStack(spacing: 10) {
                                ForEach(myEchoes) { echo in
                                    Button {
                                        selectedEcho = echo
                                    } label: {
                                        echoCard(echo)
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                            .padding(.horizontal)
                        }
                    }
                }
                .padding(.bottom, 24)
            }
            .navigationTitle("Profile")
            .background(Color(.systemGroupedBackground))
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isEditingProfile = true
                    } label: {
                        Image(systemName: "gearshape")
                    }
                }
            }
            .sheet(item: $selectedEcho) { echo in
                EchoDetailView(echo: echo)
                    .presentationDetents([.large])
                    .presentationDragIndicator(.visible)
                    .presentationCornerRadius(24)
            }
            .sheet(isPresented: $isEditingProfile) {
                EditProfileView()
                    .presentationDetents([.medium])
                    .presentationDragIndicator(.visible)
            }
        }
    }

    private var profileHeader: some View {
        VStack(spacing: 14) {
            ZStack {
                Circle()
                    .fill(
                        LinearGradient(
                            colors: [Theme.amber.opacity(0.3), Theme.terracotta.opacity(0.2)],
                            startPoint: .topLeading,
                            endPoint: .bottomTrailing
                        )
                    )
                    .frame(width: 88, height: 88)

                Text(store.currentUser.initials)
                    .font(.system(size: 30, weight: .bold))
                    .foregroundStyle(Theme.amber)
            }

            Text(store.currentUser.displayName)
                .font(.title2.weight(.bold))

            Text(store.currentUser.bio)
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Text("Joined \(store.currentUser.joinedAt.formatted(.dateTime.month(.wide).year()))")
                .font(.caption)
                .foregroundStyle(.tertiary)
        }
        .padding(.top, 16)
    }

    private var statsRow: some View {
        HStack(spacing: 0) {
            statItem(value: "\(store.currentUser.echoCount)", label: "Echoes")
            Divider().frame(height: 32)
            statItem(value: "\(store.currentUser.totalReactionsReceived)", label: "Reactions")
            Divider().frame(height: 32)
            statItem(value: "\(myEchoes.filter { $0.replyCount > 0 }.count)", label: "Threads")
        }
        .padding(.vertical, 16)
        .background(Color(.secondarySystemGroupedBackground))
        .clipShape(.rect(cornerRadius: 16))
    }

    private func statItem(value: String, label: String) -> some View {
        VStack(spacing: 4) {
            Text(value)
                .font(.title3.weight(.bold))
                .foregroundStyle(Theme.amber)
            Text(label)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
    }

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "waveform.and.mic")
                .font(.system(size: 40))
                .foregroundStyle(Theme.amber.opacity(0.4))

            Text("No echoes yet")
                .font(.headline)

            Text("Go explore and drop your first echo!")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 48)
    }

    private func echoCard(_ echo: Echo) -> some View {
        HStack(spacing: 12) {
            ZStack {
                RoundedRectangle(cornerRadius: 12)
                    .fill(Theme.amber.opacity(0.1))
                    .frame(width: 48, height: 48)

                Image(systemName: echo.type.icon)
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundStyle(Theme.amber)
            }

            VStack(alignment: .leading, spacing: 4) {
                Text(echo.locationName)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(.primary)
                    .lineLimit(1)

                if !echo.snippet.isEmpty {
                    Text(echo.snippet)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 4) {
                Text(echo.timeAgo)
                    .font(.caption2)
                    .foregroundStyle(.tertiary)

                HStack(spacing: 4) {
                    Image(systemName: "heart.fill")
                        .font(.system(size: 9))
                    Text("\(echo.totalReactions)")
                        .font(.caption2)
                }
                .foregroundStyle(Theme.terracotta.opacity(0.7))
            }
        }
        .padding(12)
        .background(Color(.secondarySystemGroupedBackground))
        .clipShape(.rect(cornerRadius: 14))
    }
}

struct EditProfileView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(EchoStore.self) private var store
    @State private var displayName: String = ""
    @State private var bio: String = ""

    var body: some View {
        NavigationStack {
            Form {
                Section("Display Name") {
                    TextField("Your name", text: $displayName)
                }

                Section("Bio") {
                    TextField("Tell people about yourself", text: $bio, axis: .vertical)
                        .lineLimit(3...6)
                }
            }
            .navigationTitle("Edit Profile")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        store.currentUser.displayName = displayName
                        store.currentUser.bio = bio
                        dismiss()
                    }
                    .fontWeight(.semibold)
                }
            }
            .onAppear {
                displayName = store.currentUser.displayName
                bio = store.currentUser.bio
            }
        }
    }
}
