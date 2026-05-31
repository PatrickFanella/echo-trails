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
        scope == .nearby ? "/discovery/nearby" : "/places/search"
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
