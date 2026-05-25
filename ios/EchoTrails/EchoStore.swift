import Foundation
import CoreLocation

@Observable
class EchoStore {
    var echoes: [Echo] = []
    var currentUser: UserProfile
    var userReactions: [String: Set<ReactionType>] = [:]

    private let sampleUsers: [UserProfile]

    init() {
        let users = [
            UserProfile(id: "user1", displayName: "Maya Rivera", bio: "Storyteller & urban explorer", avatarURL: nil, joinedAt: Date().addingTimeInterval(-86400 * 120), echoCount: 47, totalReactionsReceived: 312),
            UserProfile(id: "user2", displayName: "James Chen", bio: "Capturing moments in sound", avatarURL: nil, joinedAt: Date().addingTimeInterval(-86400 * 90), echoCount: 23, totalReactionsReceived: 156),
            UserProfile(id: "user3", displayName: "Aisha Okafor", bio: "Historian at heart", avatarURL: nil, joinedAt: Date().addingTimeInterval(-86400 * 200), echoCount: 89, totalReactionsReceived: 734),
            UserProfile(id: "user4", displayName: "Liam Walsh", bio: "Musician, listener, wanderer", avatarURL: nil, joinedAt: Date().addingTimeInterval(-86400 * 60), echoCount: 12, totalReactionsReceived: 98),
            UserProfile(id: "currentUser", displayName: "You", bio: "Leaving echoes everywhere", avatarURL: nil, joinedAt: Date().addingTimeInterval(-86400 * 30), echoCount: 8, totalReactionsReceived: 42),
        ]
        sampleUsers = users
        currentUser = users[4]

        echoes = Self.generateSampleEchoes()
    }

    func user(for id: String) -> UserProfile? {
        sampleUsers.first(where: { $0.id == id })
    }

    func toggleReaction(_ reaction: ReactionType, on echo: Echo) {
        guard let index = echoes.firstIndex(where: { $0.id == echo.id }) else { return }

        var userSet = userReactions[echo.id] ?? []
        if userSet.contains(reaction) {
            userSet.remove(reaction)
            echoes[index].reactions[reaction, default: 0] = max(0, (echoes[index].reactions[reaction] ?? 0) - 1)
        } else {
            userSet.insert(reaction)
            echoes[index].reactions[reaction, default: 0] += 1
        }
        userReactions[echo.id] = userSet
    }

    func hasReacted(_ reaction: ReactionType, on echoId: String) -> Bool {
        userReactions[echoId]?.contains(reaction) ?? false
    }

    func addEcho(type: EchoType, textContent: String?, privacyLevel: PrivacyLevel, coordinate: CLLocationCoordinate2D, locationName: String) {
        let echo = Echo(
            id: UUID().uuidString,
            type: type,
            authorId: currentUser.id,
            privacyLevel: privacyLevel,
            latitude: coordinate.latitude,
            longitude: coordinate.longitude,
            locationName: locationName.isEmpty ? "Dropped Pin" : locationName,
            textContent: textContent,
            mediaURL: nil,
            audioDuration: type == .audio ? Double.random(in: 5...30) : nil,
            createdAt: Date(),
            reactions: [:],
            replyCount: 0,
            stitchCount: 0,
            parentEchoId: nil
        )
        echoes.insert(echo, at: 0)
        currentUser.echoCount += 1
    }

    func nearbyEchoes(from location: CLLocation, limit: Int = 50) -> [Echo] {
        echoes
            .sorted { $0.distance(from: location) < $1.distance(from: location) }
            .prefix(limit)
            .map { $0 }
    }

    func trendingEchoes() -> [Echo] {
        echoes
            .sorted { $0.totalReactions + $0.replyCount > $1.totalReactions + $1.replyCount }
    }

    func echoes(by userId: String) -> [Echo] {
        echoes.filter { $0.authorId == userId }
    }

    func replies(to echoId: String) -> [Echo] {
        echoes.filter { $0.parentEchoId == echoId }
    }

    private static func generateSampleEchoes() -> [Echo] {
        let baseLat = 37.7749
        let baseLng = -122.4194

        return [
            Echo(id: "e1", type: .audio, authorId: "user1", privacyLevel: .fullProfile,
                 latitude: baseLat + 0.001, longitude: baseLng + 0.002,
                 locationName: "Mission District, SF",
                 textContent: nil, mediaURL: nil, audioDuration: 18,
                 createdAt: Date().addingTimeInterval(-3600),
                 reactions: [.heart: 24, .fire: 8, .clap: 3], replyCount: 5, stitchCount: 1, parentEchoId: nil),

            Echo(id: "e2", type: .text, authorId: "user2", privacyLevel: .displayNameOnly,
                 latitude: baseLat - 0.002, longitude: baseLng + 0.001,
                 locationName: "Dolores Park",
                 textContent: "I proposed to my partner right here, on this bench, watching the sunset paint the city gold. She said yes before I finished the question.",
                 mediaURL: nil, audioDuration: nil,
                 createdAt: Date().addingTimeInterval(-7200),
                 reactions: [.heart: 156, .sad: 2, .clap: 45], replyCount: 12, stitchCount: 3, parentEchoId: nil),

            Echo(id: "e3", type: .photo, authorId: "user3", privacyLevel: .fullProfile,
                 latitude: baseLat + 0.003, longitude: baseLng - 0.001,
                 locationName: "Painted Ladies, Alamo Square",
                 textContent: "These houses have seen 120 years of San Francisco history. Every earthquake, every fog, every summer.",
                 mediaURL: nil, audioDuration: nil,
                 createdAt: Date().addingTimeInterval(-14400),
                 reactions: [.fire: 34, .heart: 67, .mindBlown: 12], replyCount: 8, stitchCount: 2, parentEchoId: nil),

            Echo(id: "e4", type: .audio, authorId: "user4", privacyLevel: .anonymous,
                 latitude: baseLat - 0.001, longitude: baseLng - 0.003,
                 locationName: "Golden Gate Bridge Vista",
                 textContent: nil, mediaURL: nil, audioDuration: 27,
                 createdAt: Date().addingTimeInterval(-28800),
                 reactions: [.heart: 89, .fire: 23, .laugh: 4], replyCount: 15, stitchCount: 5, parentEchoId: nil),

            Echo(id: "e5", type: .text, authorId: "user1", privacyLevel: .fullProfile,
                 latitude: baseLat + 0.004, longitude: baseLng + 0.003,
                 locationName: "City Lights Bookstore",
                 textContent: "Kerouac drank coffee here. Ginsberg read poetry here. And today, a stranger smiled at me over a shared love of Ferlinghetti.",
                 mediaURL: nil, audioDuration: nil,
                 createdAt: Date().addingTimeInterval(-43200),
                 reactions: [.heart: 45, .clap: 18, .fire: 7], replyCount: 6, stitchCount: 0, parentEchoId: nil),

            Echo(id: "e6", type: .video, authorId: "user3", privacyLevel: .displayNameOnly,
                 latitude: baseLat - 0.003, longitude: baseLng + 0.004,
                 locationName: "Fisherman's Wharf",
                 textContent: "The sea lions are singing their afternoon opera again.",
                 mediaURL: nil, audioDuration: nil,
                 createdAt: Date().addingTimeInterval(-57600),
                 reactions: [.laugh: 78, .heart: 34, .fire: 12], replyCount: 9, stitchCount: 1, parentEchoId: nil),

            Echo(id: "e7", type: .audio, authorId: "user2", privacyLevel: .fullProfile,
                 latitude: baseLat + 0.002, longitude: baseLng - 0.004,
                 locationName: "Haight-Ashbury",
                 textContent: nil, mediaURL: nil, audioDuration: 22,
                 createdAt: Date().addingTimeInterval(-72000),
                 reactions: [.fire: 56, .heart: 23, .mindBlown: 8], replyCount: 4, stitchCount: 2, parentEchoId: nil),

            Echo(id: "e8", type: .text, authorId: "user4", privacyLevel: .anonymous,
                 latitude: baseLat - 0.004, longitude: baseLng - 0.002,
                 locationName: "Twin Peaks Summit",
                 textContent: "From up here, every light is someone's story. Every window, a life unfolding. This city is a library of lives.",
                 mediaURL: nil, audioDuration: nil,
                 createdAt: Date().addingTimeInterval(-86400),
                 reactions: [.heart: 201, .fire: 45, .clap: 67, .mindBlown: 23], replyCount: 22, stitchCount: 7, parentEchoId: nil),

            Echo(id: "e9", type: .photo, authorId: "user1", privacyLevel: .fullProfile,
                 latitude: baseLat + 0.005, longitude: baseLng + 0.001,
                 locationName: "Chinatown Gate",
                 textContent: "My grandmother walked through this gate in 1962. Today I brought my daughter for the first time.",
                 mediaURL: nil, audioDuration: nil,
                 createdAt: Date().addingTimeInterval(-100800),
                 reactions: [.heart: 134, .sad: 12, .clap: 56], replyCount: 18, stitchCount: 4, parentEchoId: nil),

            Echo(id: "e10", type: .audio, authorId: "user3", privacyLevel: .displayNameOnly,
                 latitude: baseLat - 0.005, longitude: baseLng + 0.005,
                 locationName: "Baker Beach",
                 textContent: nil, mediaURL: nil, audioDuration: 14,
                 createdAt: Date().addingTimeInterval(-129600),
                 reactions: [.heart: 42, .fire: 15, .laugh: 6], replyCount: 3, stitchCount: 0, parentEchoId: nil),

            Echo(id: "r1", type: .text, authorId: "user3", privacyLevel: .fullProfile,
                 latitude: baseLat - 0.002, longitude: baseLng + 0.001,
                 locationName: "Dolores Park",
                 textContent: "This is the most beautiful thing I've read today. Congratulations to you both!",
                 mediaURL: nil, audioDuration: nil,
                 createdAt: Date().addingTimeInterval(-5400),
                 reactions: [.heart: 8, .clap: 3], replyCount: 0, stitchCount: 0, parentEchoId: "e2"),

            Echo(id: "r2", type: .audio, authorId: "user4", privacyLevel: .anonymous,
                 latitude: baseLat - 0.002, longitude: baseLng + 0.001,
                 locationName: "Dolores Park",
                 textContent: nil, mediaURL: nil, audioDuration: 9,
                 createdAt: Date().addingTimeInterval(-4800),
                 reactions: [.heart: 4], replyCount: 0, stitchCount: 0, parentEchoId: "e2"),
        ]
    }
}
