import SwiftUI

@main
struct EchoTrailsApp: App {
    @State private var echoStore = EchoStore()
    @State private var locationService = LocationService()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(echoStore)
                .environment(locationService)
                .preferredColorScheme(.dark)
        }
    }
}
