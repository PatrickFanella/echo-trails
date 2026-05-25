import Foundation
import CoreLocation


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


    nonisolated func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {