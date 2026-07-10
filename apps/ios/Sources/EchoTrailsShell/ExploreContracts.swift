import Foundation

public enum ExploreScope: String, CaseIterable, Sendable {
    case places
    case nearby
    case subscribedPlaces
    case city
    case echoText
}

public enum RankingReason: String, Sendable {
    case recent
    case popularNearby
    case subscribedPlace
    case notable
}

/// The three server-supported scopes for weighted Echo and Place search.
public enum DiscoverySearchScope: String, CaseIterable, Identifiable, Sendable {
    case city
    case nearby
    case place

    public var id: String { rawValue }
}

/// Builds the generated `/search` contract without allowing UI code to hand-roll
/// or incorrectly escape query strings.
public struct DiscoverySearchQuery: Equatable, Sendable {
    public let text: String
    public let scope: DiscoverySearchScope
    public let placeID: String?
    public let latitude: Double?
    public let longitude: Double?
    public let radiusMeters: Int
    public let limit: Int
    public let cursor: String?

    public init(
        text: String,
        scope: DiscoverySearchScope,
        placeID: String? = nil,
        latitude: Double? = nil,
        longitude: Double? = nil,
        radiusMeters: Int = 1_000,
        limit: Int = 30,
        cursor: String? = nil
    ) {
        self.text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        self.scope = scope
        self.placeID = placeID?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.latitude = latitude
        self.longitude = longitude
        self.radiusMeters = radiusMeters
        self.limit = limit
        self.cursor = cursor?.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public var isValid: Bool {
        guard !text.isEmpty, text.count <= 200, (1...50).contains(limit) else { return false }
        switch scope {
        case .city:
            return true
        case .place:
            return placeID?.isEmpty == false
        case .nearby:
            guard let latitude, let longitude else { return false }
            return (-90...90).contains(latitude) && (-180...180).contains(longitude) && (10...10_000).contains(radiusMeters)
        }
    }

    public var apiPath: String? {
        guard isValid else { return nil }
        var components = URLComponents()
        components.path = OpenAPIPaths.searchDiscovery
        var items = [
            URLQueryItem(name: "q", value: text),
            URLQueryItem(name: "scope", value: scope.rawValue),
            URLQueryItem(name: "limit", value: String(limit)),
        ]
        switch scope {
        case .city:
            break
        case .place:
            items.append(URLQueryItem(name: "place_id", value: placeID))
        case .nearby:
            items.append(URLQueryItem(name: "lat", value: latitude.map { String($0) }))
            items.append(URLQueryItem(name: "lon", value: longitude.map { String($0) }))
            items.append(URLQueryItem(name: "radius_meters", value: String(radiusMeters)))
        }
        if let cursor, !cursor.isEmpty { items.append(URLQueryItem(name: "cursor", value: cursor)) }
        components.queryItems = items
        return components.string
    }

    public func withCursor(_ cursor: String?) -> DiscoverySearchQuery {
        DiscoverySearchQuery(
            text: text, scope: scope, placeID: placeID, latitude: latitude,
            longitude: longitude, radiusMeters: radiusMeters, limit: limit, cursor: cursor
        )
    }
}

public struct ExploreSearchQuery: Equatable, Sendable {
    public let text: String
    public let scope: ExploreScope
    public let city: String?

    public init(text: String, scope: ExploreScope = .places, city: String? = nil) {
        self.text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        self.scope = scope
        self.city = city?.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public var isValid: Bool {
        !text.isEmpty && text.count <= 120
    }

    public var apiPath: String {
        scope == .nearby ? OpenAPIPaths.nearbyPlaces : OpenAPIPaths.searchPlaces
    }
}

public struct ExploreResultDescriptor: Equatable, Sendable {
    public let placeID: String
    public let title: String
    public let subtitle: String
    public let rankingReason: RankingReason
    public let accessibilityLabel: String

    public init(placeID: String, title: String, subtitle: String, rankingReason: RankingReason) {
        self.placeID = placeID
        self.title = title
        self.subtitle = subtitle
        self.rankingReason = rankingReason
        self.accessibilityLabel = "Explore result: \(title)"
    }
}

public struct ExploreSectionDescriptor: Equatable, Sendable {
    public let title: String
    public let scope: ExploreScope
    public let emptyState: String
    public let results: [ExploreResultDescriptor]

    public init(title: String, scope: ExploreScope, emptyState: String, results: [ExploreResultDescriptor] = []) {
        self.title = title
        self.scope = scope
        self.emptyState = emptyState
        self.results = results
    }

    public static let defaultSections: [ExploreSectionDescriptor] = [
        ExploreSectionDescriptor(title: "Nearby Places", scope: .nearby, emptyState: "No nearby Places yet."),
        ExploreSectionDescriptor(title: "Subscribed Places", scope: .subscribedPlaces, emptyState: "Follow Places to see them here."),
        ExploreSectionDescriptor(title: "Notable Echoes", scope: .echoText, emptyState: "No notable Echoes yet.")
    ]
}
