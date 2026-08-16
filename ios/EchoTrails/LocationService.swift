import Foundation
import CoreLocation
import Observation


@Observable
class LocationService: NSObject, CLLocationManagerDelegate {
    let manager = CLLocationManager()
    var location: CLLocation?
    var authorizationStatus: CLAuthorizationStatus = .notDetermined
    var placeName: String = ""


    private let geocoder = CLGeocoder()


    override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
        authorizationStatus = manager.authorizationStatus
    }


    func requestPermission() {
        manager.requestWhenInUseAuthorization()
    }


    func requestLocation() {
        manager.requestLocation()
    }


    func startUpdating() {
        manager.startUpdatingLocation()
    }


    func stopUpdating() {
        manager.stopUpdatingLocation()
    }


    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let latestLocation = locations.last else { return }

        location = latestLocation
        geocoder.reverseGeocodeLocation(latestLocation) { [weak self] placemarks, _ in
            guard let self else { return }
            let placemark = placemarks?.first
            let name = [placemark?.name, placemark?.locality]
                .compactMap { $0 }
                .joined(separator: ", ")
            self.placeName = name
        }
    }

    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        authorizationStatus = manager.authorizationStatus
    }

    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        placeName = ""
    }
}
