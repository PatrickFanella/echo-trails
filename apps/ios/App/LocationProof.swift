import CoreLocation
import EchoTrailsShell
import Foundation

enum LocationProofError: LocalizedError {
    case denied, unavailable, inaccurate
    var errorDescription: String? { switch self { case .denied: "Location permission is required only while creating at a Place."; case .unavailable: "A current location could not be obtained."; case .inaccurate: "Move outdoors and try again when GPS accuracy is within 100 meters." } }
}

struct CreationTokenRequest: Encodable {
    let placeID: String; let latitude, longitude, accuracyMeters: Double; let capturedAt: Date
    enum CodingKeys: String, CodingKey { case placeID = "place_id", latitude, longitude, capturedAt = "captured_at", accuracyMeters = "accuracy_meters" }
}
struct CreationTokenResponse: Decodable { let creationToken, placeID: String; let expiresAt: Date; enum CodingKeys: String, CodingKey { case creationToken = "creation_token", placeID = "place_id", expiresAt = "expires_at" } }

@MainActor final class LocationProofProvider: NSObject, @preconcurrency CLLocationManagerDelegate {
    private let manager = CLLocationManager()
    private var continuation: CheckedContinuation<CLLocation, Error>?

    override init() { super.init(); manager.delegate = self; manager.desiredAccuracy = kCLLocationAccuracyBest; manager.distanceFilter = kCLDistanceFilterNone }

    func currentLocation() async throws -> CLLocation {
        guard continuation == nil else { throw LocationProofError.unavailable }
        if manager.authorizationStatus == .denied || manager.authorizationStatus == .restricted { throw LocationProofError.denied }
        if manager.authorizationStatus == .notDetermined { manager.requestWhenInUseAuthorization() }
        return try await withCheckedThrowingContinuation { continuation = $0; manager.requestLocation() }
    }

    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let continuation, let location = locations.last else { return }
        self.continuation = nil
        guard location.horizontalAccuracy > 0, location.horizontalAccuracy <= 100, abs(location.timestamp.timeIntervalSinceNow) <= 120 else { continuation.resume(throwing: LocationProofError.inaccurate); return }
        continuation.resume(returning: location)
    }
    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) { continuation?.resume(throwing: error); continuation = nil }
    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) { if manager.authorizationStatus == .denied { continuation?.resume(throwing: LocationProofError.denied); continuation = nil } }

    func token(for placeID: String, client: APIClient) async throws -> CreationTokenResponse {
        let location = try await currentLocation()
        let request = CreationTokenRequest(placeID: placeID, latitude: location.coordinate.latitude, longitude: location.coordinate.longitude, accuracyMeters: location.horizontalAccuracy, capturedAt: location.timestamp)
        // The exact coordinate is intentionally sent directly and is never persisted or logged by the app.
        return try await client.send(OpenAPIPaths.createCreationToken, method: "POST", body: request, idempotencyKey: UUID().uuidString)
    }
}
