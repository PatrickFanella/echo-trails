import SwiftUI
import CoreLocation

struct NearbyEchoRow: View {
    let echo: Echo
    let userLocation: CLLocation?
    let store: EchoStore

    private var authorName: String {
        switch echo.privacyLevel {
        case .anonymous: return "Anonymous"
        case .displayNameOnly, .fullProfile:
            return store.user(for: echo.authorId)?.displayName ?? "Unknown"
        }
    }

    var body: some View {
        HStack(spacing: 14) {
            ZStack {
                Circle()
                    .fill(Theme.amber.opacity(0.15))
                    .frame(width: 48, height: 48)

                Image(systemName: echo.type.icon)
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundStyle(Theme.amber)
            }

            VStack(alignment: .leading, spacing: 4) {
                HStack {
                    Text(echo.locationName)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    Spacer()

                    if let loc = userLocation {
                        Text(echo.formattedDistance(from: loc))
                            .font(.caption)
                            .foregroundStyle(Theme.amber)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 3)
                            .background(Theme.amber.opacity(0.12), in: Capsule())
                    }
                }

                if !echo.snippet.isEmpty {
                    Text(echo.snippet)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }

                HStack(spacing: 12) {
                    Text(authorName)
                        .font(.caption2)
                        .foregroundStyle(.tertiary)

                    Text(echo.timeAgo)
                        .font(.caption2)
                        .foregroundStyle(.tertiary)

                    Spacer()

                    if echo.totalReactions > 0 {
                        HStack(spacing: 3) {
                            Image(systemName: "heart.fill")
                                .font(.system(size: 9))
                            Text("\(echo.totalReactions)")
                                .font(.caption2)
                        }
                        .foregroundStyle(Theme.terracotta.opacity(0.8))
                    }

                    if echo.replyCount > 0 {
                        HStack(spacing: 3) {
                            Image(systemName: "arrowshape.turn.up.left.fill")
                                .font(.system(size: 9))
                            Text("\(echo.replyCount)")
                                .font(.caption2)
                        }
                        .foregroundStyle(.tertiary)
                    }
                }
            }
        }
        .padding(.vertical, 6)
        .padding(.horizontal, 4)
    }
}
