import SwiftUI

struct ContentView: View {
    var body: some View {
        TabView {
            Tab("Explore", systemImage: "map.fill") {
                MapScreenView()
            }

            Tab("Trending", systemImage: "flame.fill") {
                TrendingFeedView()
            }

            Tab("Profile", systemImage: "person.fill") {
                ProfileView()
            }
        }
        .tint(Theme.amber)
    }
}
