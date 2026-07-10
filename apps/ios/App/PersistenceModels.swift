import Foundation
import SwiftData

enum DurableOutboxPolicy {
    static let maximumAttempts = 8
    static func delay(afterAttempt attempt: Int) -> TimeInterval {
        min(pow(2, Double(max(1, attempt))) * 5, 3600)
    }
}

@Model final class CachedPlace {
    @Attribute(.unique) var id: String
    var name: String
    var state: String
    var latitude: Double?
    var longitude: Double?
    var radiusMeters: Double?
    var category: String?
    var sensitive: Bool?
    var cachedAt: Date

    init(id: String, name: String, state: String, latitude: Double? = nil, longitude: Double? = nil, radiusMeters: Double? = nil, category: String? = nil, sensitive: Bool? = nil, cachedAt: Date = .now) {
        self.id = id; self.name = name; self.state = state; self.latitude = latitude; self.longitude = longitude; self.radiusMeters = radiusMeters; self.category = category; self.sensitive = sensitive; self.cachedAt = cachedAt
    }
}

@Model final class CachedEcho {
    @Attribute(.unique) var id: String
    var placeID: String
    var authorHandle: String
    var title: String
    var body: String?
    var caption: String?
    var contentMode: String
    var status: String
    var publishedAt: Date?
    var encodedEcho: Data?
    var rankingScore: Double = 0
    var rankingLabel: String = "Recent"
    var cachedAt: Date

    init(id: String, placeID: String, authorHandle: String, title: String, body: String?, caption: String?, contentMode: String, status: String, publishedAt: Date?, encodedEcho: Data? = nil, rankingScore: Double = 0, rankingLabel: String = "Recent") {
        self.id = id; self.placeID = placeID; self.authorHandle = authorHandle; self.title = title; self.body = body; self.caption = caption; self.contentMode = contentMode; self.status = status; self.publishedAt = publishedAt; self.encodedEcho = encodedEcho; self.rankingScore = rankingScore; self.rankingLabel = rankingLabel; self.cachedAt = .now
    }
}

@Model final class CachedProfile {
    @Attribute(.unique) var userID: String
    var handle: String
    var displayName: String
    var bio: String
    var avatarURL: String?
    var cachedAt: Date

    init(userID: String, handle: String, displayName: String, bio: String, avatarURL: String? = nil) {
        self.userID = userID; self.handle = handle; self.displayName = displayName; self.bio = bio; self.avatarURL = avatarURL; self.cachedAt = .now
    }
}

@Model final class DurableOutboxItem {
    @Attribute(.unique) var id: UUID
    var operation: String
    var endpoint: String
    var method: String
    var body: Data
    var creationToken: String?
    var filePaths: [String]
    var state: String
    var attemptCount: Int
    var nextAttemptAt: Date
    var createdAt: Date
    var lastError: String?

    init(operation: String, endpoint: String, method: String = "POST", body: Data, creationToken: String? = nil, filePaths: [String] = []) {
        self.id = UUID(); self.operation = operation; self.endpoint = endpoint; self.method = method; self.body = body; self.creationToken = creationToken; self.filePaths = filePaths; self.state = "queued"; self.attemptCount = 0; self.nextAttemptAt = .now; self.createdAt = .now
    }
}

@Model final class CachedUpload {
    @Attribute(.unique) var uploadID: String
    var echoID: String
    var mediaAssetID: String
    var localFilePath: String
    var status: String
    var sortOrder: Int
    var updatedAt: Date

    init(uploadID: String, echoID: String, mediaAssetID: String, localFilePath: String, status: String, sortOrder: Int) {
        self.uploadID = uploadID; self.echoID = echoID; self.mediaAssetID = mediaAssetID; self.localFilePath = localFilePath; self.status = status; self.sortOrder = sortOrder; self.updatedAt = .now
    }
}

enum PersistenceStack {
    static let schema = Schema([CachedPlace.self, CachedEcho.self, CachedProfile.self, DurableOutboxItem.self, CachedUpload.self])
    static func container(inMemory: Bool = false) throws -> ModelContainer {
        try ModelContainer(for: schema, configurations: ModelConfiguration(schema: schema, isStoredInMemoryOnly: inMemory))
    }
}
