import SwiftUI

struct TrendingFeedView: View {
    @Environment(EchoStore.self) private var store
    @Environment(LocationService.self) private var locationService
    @State private var selectedEcho: Echo?

    private var trendingEchoes: [Echo] {
        store.trendingEchoes().filter { $0.parentEchoId == nil }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 16) {
                    if let top = trendingEchoes.first {
                        heroCard(top)
                    }

                    LazyVStack(spacing: 14) {
                        ForEach(Array(trendingEchoes.dropFirst())) { echo in
                            trendingCard(echo)
                        }
                    }
                }
                .padding(.horizontal)
                .padding(.bottom, 24)
            }
            .navigationTitle("Trending")
            .background(Color(.systemGroupedBackground))
            .sheet(item: $selectedEcho) { echo in
                EchoDetailView(echo: echo)
                    .presentationDetents([.large])
                    .presentationDragIndicator(.visible)
                    .presentationCornerRadius(24)
            }
        }
    }

    private func heroCard(_ echo: Echo) -> some View {
        Button {
            selectedEcho = echo
        } label: {
            VStack(alignment: .leading, spacing: 0) {
                ZStack(alignment: .bottomLeading) {
                    LinearGradient(
                        colors: [Theme.terracotta, Theme.amber.opacity(0.7)],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                    .frame(height: 180)

                    VStack(alignment: .leading) {
                        HStack {
                            Image(systemName: "flame.fill")
                                .font(.caption.weight(.bold))
                            Text("TOP ECHO")
                                .font(.caption.weight(.bold))
                        }
                        .foregroundStyle(.white.opacity(0.9))
                        .padding(.horizontal, 10)
                        .padding(.vertical, 5)
                        .background(.white.opacity(0.2), in: Capsule())

                        Spacer()

                        Text(echo.locationName)
                            .font(.title2.weight(.bold))
                            .foregroundStyle(.white)

                        HStack(spacing: 12) {
                            Label("\(echo.totalReactions)", systemImage: "heart.fill")
                            Label("\(echo.replyCount)", systemImage: "arrowshape.turn.up.left.fill")
                            Label("\(echo.stitchCount)", systemImage: "link")
                        }
                        .font(.caption.weight(.medium))
                        .foregroundStyle(.white.opacity(0.85))
                    }
                    .padding()
                }

                VStack(alignment: .leading, spacing: 8) {
                    HStack(spacing: 8) {
                        Image(systemName: echo.type.icon)
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundStyle(Theme.amber)

                        Text(echo.type.label)
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Theme.amber)

                        Spacer()

                        Text(echo.timeAgo)
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                    }

                    if let text = echo.textContent, !text.isEmpty {
                        Text(text)
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .lineLimit(3)
                            .multilineTextAlignment(.leading)
                    }

                    topReactions(echo)
                }
                .padding()
                .background(Color(.secondarySystemGroupedBackground))
            }
            .clipShape(.rect(cornerRadius: 20))
            .shadow(color: .black.opacity(0.08), radius: 12, y: 4)
        }
        .buttonStyle(.plain)
    }

    private func trendingCard(_ echo: Echo) -> some View {
        let authorName: String = {
            switch echo.privacyLevel {
            case .anonymous: return "Anonymous"
            case .displayNameOnly, .fullProfile:
                return store.user(for: echo.authorId)?.displayName ?? "Unknown"
            }
        }()

        return Button {
            selectedEcho = echo
        } label: {
            HStack(spacing: 14) {
                ZStack {
                    RoundedRectangle(cornerRadius: 14)
                        .fill(Theme.amber.opacity(0.1))
                        .frame(width: 56, height: 56)

                    Image(systemName: echo.type.icon)
                        .font(.system(size: 22, weight: .semibold))
                        .foregroundStyle(Theme.amber)
                }

                VStack(alignment: .leading, spacing: 4) {
                    Text(echo.locationName)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    if let text = echo.textContent, !text.isEmpty {
                        Text(text)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                            .multilineTextAlignment(.leading)
                    }

                    HStack(spacing: 8) {
                        Text(authorName)
                            .font(.caption2)
                            .foregroundStyle(.tertiary)

                        Spacer()

                        HStack(spacing: 4) {
                            Image(systemName: "heart.fill")
                                .font(.system(size: 9))
                            Text("\(echo.totalReactions)")
                                .font(.caption2.weight(.medium))
                        }
                        .foregroundStyle(Theme.terracotta)

                        HStack(spacing: 4) {
                            Image(systemName: "arrowshape.turn.up.left.fill")
                                .font(.system(size: 9))
                            Text("\(echo.replyCount)")
                                .font(.caption2)
                        }
                        .foregroundStyle(.tertiary)
                    }
                }
            }
            .padding(12)
            .background(Color(.secondarySystemGroupedBackground))
            .clipShape(.rect(cornerRadius: 16))
        }
        .buttonStyle(.plain)
    }

    private func topReactions(_ echo: Echo) -> some View {
        let sorted = echo.reactions.sorted { $0.value > $1.value }.prefix(4)

        return HStack(spacing: 6) {
            ForEach(Array(sorted), id: \.key) { reaction, count in
                HStack(spacing: 3) {
                    Text(reaction.emoji)
                        .font(.system(size: 14))
                    Text("\(count)")
                        .font(.caption2.weight(.medium))
                        .foregroundStyle(.secondary)
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(Color(.tertiarySystemGroupedBackground))
                .clipShape(Capsule())
            }

            Spacer()
        }
    }
}
