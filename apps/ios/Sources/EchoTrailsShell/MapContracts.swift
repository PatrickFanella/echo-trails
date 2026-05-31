import Foundation

public enum PublicLocationPrecision: String, Sendable {
    case approximate
    case neighborhood
    case city
}

public struct CoordinateValue: Equatable, Sendable {
    public let latitude: Double
    public let longitude: Double

    public init(latitude: Double, longitude: Double) {
        self.latitude = latitude
        self.longitude = longitude
    }

    public var isValidPublicCoordinate: Bool {
        (-90...90).contains(latitude) && (-180...180).contains(longitude)
    }
}

public struct MapViewport: Equatable, Sendable {
    public let center: CoordinateValue
    public let radiusMeters: Int

    public init(center: CoordinateValue, radiusMeters: Int) {
        self.center = center
        self.radiusMeters = radiusMeters
    }

    public var isValid: Bool {
        center.isValidPublicCoordinate && (10...10_000).contains(radiusMeters)
    }
}

public struct PlaceActivityPulse: Equatable, Sendable {
    public let recentEchoCount: Int
    public let hasNewActivity: Bool

    public init(recentEchoCount: Int, hasNewActivity: Bool) {
        self.recentEchoCount = max(0, recentEchoCount)
        self.hasNewActivity = hasNewActivity
    }
}

public struct PlaceMarkerDescriptor: Equatable, Sendable {
    public let placeID: String
    public let title: String
    public let publicCoordinate: CoordinateValue
    public let precision: PublicLocationPrecision
    public let activityPulse: PlaceActivityPulse
    public let accessibilityLabel: String
    public let detailAPIPath: String

    public init(placeID: String, title: String, publicCoordinate: CoordinateValue, precision: PublicLocationPrecision = .approximate, activityPulse: PlaceActivityPulse = PlaceActivityPulse(recentEchoCount: 0, hasNewActivity: false)) {
        self.placeID = placeID
        self.title = title
        self.publicCoordinate = publicCoordinate
        self.precision = precision
        self.activityPulse = activityPulse
        self.accessibilityLabel = "Place marker: \(title)"
        self.detailAPIPath = ContractPath.place(placeID) ?? ""
    }

    public var isRenderable: Bool {
        ContractPath.safeSegment(placeID) != nil && !title.isEmpty && publicCoordinate.isValidPublicCoordinate
    }
}

public struct PlaceDetailDescriptor: Equatable, Sendable {
    public let placeID: String
    public let echoesAPIPath: String
    public let nearbyAPIPath: String

    public init(placeID: String) {
        self.placeID = placeID
        self.echoesAPIPath = ContractPath.placeEchoes(placeID) ?? ""
        self.nearbyAPIPath = "/discovery/nearby"
    }
}
