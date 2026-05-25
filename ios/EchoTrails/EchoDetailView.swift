import SwiftUI
import CoreLocation

struct EchoDetailView: View {
    let echo: Echo
    @Environment(EchoStore.self) private var store
    @Environment(LocationService.self) private var locationService
    @State private var isPlaying = false
    @State private var playbackProgress: Double = 0
    @State private var showReplySheet = false

    private var authorName: String {
        switch echo.privacyLevel {
        case .anonymous: return "Anonymous"
        case .displayNameOnly, .fullProfile:
            return store.user(for: echo.authorId)?.displayName ?? "Unknown"
        }
    }

    private var replies: [Echo] {
        store.replies(to: echo.id)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    headerSection

                    VStack(alignment: .leading, spacing: 20) {
                        if echo.type == .audio {
                            audioPlayer
                        }

                        if let text = echo.textContent, !text.isEmpty {
                            Text(text)
                                .font(.body)
                                .lineSpacing(4)
                        }

                        if echo.type == .photo || echo.type == .video {
                            mediaPlaceholder
                        }

                        metaInfo

                        reactionBar

                        actionButtons

                        if !replies.isEmpty {
                            replySection
                        }
                    }
                    .padding()
                }
            }
            .navigationBarTitleDisplayMode(.inline)
            .sheet(isPresented: $showReplySheet) {
                CreateEchoView()
                    .presentationDetents([.large])
                    .presentationDragIndicator(.visible)
            }
        }
    }

    private var headerSection: some View {
        VStack(spacing: 0) {
            ZStack(alignment: .bottomLeading) {
                LinearGradient(
                    colors: [Theme.terracotta.opacity(0.6), Theme.amber.opacity(0.3), Color(.systemBackground)],
                    startPoint: .top,
                    endPoint: .bottom
                )
                .frame(height: 160)

                HStack(spacing: 12) {
                    ZStack {
                        Circle()
                            .fill(Theme.amber.opacity(0.2))
                            .frame(width: 52, height: 52)

                        Image(systemName: echo.type.icon)
                            .font(.system(size: 22, weight: .semibold))
                            .foregroundStyle(Theme.amber)
                    }

                    VStack(alignment: .leading, spacing: 4) {
                        Text(echo.locationName)
                            .font(.title3.weight(.bold))

                        HStack(spacing: 8) {
                            authorBadge

                            Text(echo.timeAgo)
                                .font(.caption)
                                .foregroundStyle(.secondary)

                            if let loc = locationService.location {
                                Text(echo.formattedDistance(from: loc))
                                    .font(.caption.weight(.medium))
                                    .foregroundStyle(Theme.amber)
                            }
                        }
                    }
                }
                .padding()
            }
        }
    }

    @ViewBuilder
    private var authorBadge: some View {
        HStack(spacing: 4) {
            Image(systemName: echo.privacyLevel.icon)
                .font(.system(size: 10))
            Text(authorName)
                .font(.caption.weight(.medium))
        }
        .foregroundStyle(.secondary)
    }

    private var audioPlayer: some View {
        VStack(spacing: 12) {
            HStack(spacing: 16) {
                Button {
                    withAnimation(.snappy) {
                        isPlaying.toggle()
                        if isPlaying { simulatePlayback() }
                    }
                } label: {
                    ZStack {
                        Circle()
                            .fill(Theme.gradient)
                            .frame(width: 56, height: 56)
                            .shadow(color: Theme.amber.opacity(0.3), radius: 8, y: 2)

                        Image(systemName: isPlaying ? "pause.fill" : "play.fill")
                            .font(.system(size: 22))
                            .foregroundStyle(.white)
                            .contentTransition(.symbolEffect(.replace))
                    }
                }
                .sensoryFeedback(.impact(weight: .light), trigger: isPlaying)

                VStack(spacing: 8) {
                    waveformView

                    HStack {
                        Text(formatTime(playbackProgress * (echo.audioDuration ?? 0)))
                            .font(.caption2.monospacedDigit())
                            .foregroundStyle(.secondary)

                        Spacer()

                        if let duration = echo.audioDuration {
                            Text(formatTime(duration))
                                .font(.caption2.monospacedDigit())
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            }
        }
        .padding(16)
        .background(Color(.secondarySystemGroupedBackground))
        .clipShape(.rect(cornerRadius: 16))
    }

    private var waveformView: some View {
        GeometryReader { geo in
            HStack(spacing: 2) {
                ForEach(0..<40, id: \.self) { i in
                    let height = waveformHeight(for: i)
                    let filled = Double(i) / 40.0 <= playbackProgress

                    RoundedRectangle(cornerRadius: 1)
                        .fill(filled ? Theme.amber : Theme.amber.opacity(0.2))
                        .frame(width: max((geo.size.width - 78) / 40, 2), height: height)
                }
            }
            .frame(height: 32, alignment: .center)
        }
        .frame(height: 32)
    }

    private func waveformHeight(for index: Int) -> CGFloat {
        let seed = sin(Double(index) * 0.8) * cos(Double(index) * 0.3) + sin(Double(index) * 1.5) * 0.5
        return CGFloat(8 + abs(seed) * 24)
    }

    private var mediaPlaceholder: some View {
        VStack(spacing: 12) {
            Image(systemName: echo.type == .photo ? "photo.fill" : "video.fill")
                .font(.system(size: 36))
                .foregroundStyle(Theme.amber.opacity(0.5))

            Text(echo.type == .photo ? "Photo" : "Video")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .frame(height: 200)
        .background(Color(.secondarySystemGroupedBackground))
        .clipShape(.rect(cornerRadius: 16))
    }

    private var metaInfo: some View {
        HStack(spacing: 16) {
            Label("\(echo.totalReactions)", systemImage: "heart.fill")
                .font(.subheadline)
                .foregroundStyle(Theme.terracotta)

            Label("\(echo.replyCount)", systemImage: "arrowshape.turn.up.left.fill")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Label("\(echo.stitchCount)", systemImage: "link")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Spacer()
        }
    }

    private var reactionBar: some View {
        HStack(spacing: 8) {
            ForEach(ReactionType.allCases) { reaction in
                let count = echo.reactions[reaction] ?? 0
                let hasReacted = store.hasReacted(reaction, on: echo.id)

                Button {
                    withAnimation(.snappy) {
                        store.toggleReaction(reaction, on: echo)
                    }
                } label: {
                    HStack(spacing: 4) {
                        Text(reaction.emoji)
                            .font(.system(size: 18))

                        if count > 0 {
                            Text("\(count)")
                                .font(.caption2.weight(.semibold))
                                .foregroundStyle(hasReacted ? Theme.amber : .secondary)
                        }
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 8)
                    .background(
                        hasReacted
                        ? Theme.amber.opacity(0.15)
                        : Color(.tertiarySystemGroupedBackground)
                    )
                    .clipShape(Capsule())
                }
                .sensoryFeedback(.impact(weight: .light), trigger: hasReacted)
            }
        }
    }

    private var actionButtons: some View {
        HStack(spacing: 12) {
            Button {
                showReplySheet = true
            } label: {
                Label("Reply", systemImage: "arrowshape.turn.up.left")
                    .font(.subheadline.weight(.medium))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .tint(Theme.amber)

            Button {
            } label: {
                Label("Stitch", systemImage: "link")
                    .font(.subheadline.weight(.medium))
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)

            Button {
            } label: {
                Image(systemName: "square.and.arrow.up")
                    .font(.subheadline.weight(.medium))
            }
            .buttonStyle(.bordered)
        }
    }

    @ViewBuilder
    private var replySection: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Replies")
                .font(.headline)
                .padding(.top, 8)

            ForEach(replies) { reply in
                replyCard(reply)
            }
        }
    }

    private func replyCard(_ reply: Echo) -> some View {
        let replyAuthor: String = {
            switch reply.privacyLevel {
            case .anonymous: return "Anonymous"
            case .displayNameOnly, .fullProfile:
                return store.user(for: reply.authorId)?.displayName ?? "Unknown"
            }
        }()

        return HStack(alignment: .top, spacing: 10) {
            Circle()
                .fill(Theme.amber.opacity(0.2))
                .frame(width: 32, height: 32)
                .overlay {
                    Image(systemName: reply.type.icon)
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(Theme.amber)
                }

            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(replyAuthor)
                        .font(.caption.weight(.semibold))
                    Text(reply.timeAgo)
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }

                if let text = reply.textContent {
                    Text(text)
                        .font(.subheadline)
                }

                if reply.type == .audio {
                    HStack(spacing: 6) {
                        Image(systemName: "waveform")
                            .font(.caption)
                            .foregroundStyle(Theme.amber)
                        if let dur = reply.audioDuration {
                            Text("\(Int(dur))s")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .background(Theme.amber.opacity(0.1))
                    .clipShape(Capsule())
                }
            }

            Spacer()
        }
        .padding(12)
        .background(Color(.secondarySystemGroupedBackground))
        .clipShape(.rect(cornerRadius: 12))
    }

    private func formatTime(_ seconds: TimeInterval) -> String {
        let mins = Int(seconds) / 60
        let secs = Int(seconds) % 60
        return String(format: "%d:%02d", mins, secs)
    }

    private func simulatePlayback() {
        playbackProgress = 0
        let duration = echo.audioDuration ?? 15
        Task {
            let steps = Int(duration * 10)
            for i in 1...steps {
                try? await Task.sleep(for: .milliseconds(100))
                guard isPlaying else { return }
                playbackProgress = Double(i) / Double(steps)
            }
            isPlaying = false
            playbackProgress = 0
        }
    }
}
