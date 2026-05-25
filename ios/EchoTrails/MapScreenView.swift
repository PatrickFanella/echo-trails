import SwiftUI
import MapKit

struct MapScreenView: View {
    @Environment(EchoStore.self) private var store
    @Environment(LocationService.self) private var locationService
    @State private var position: MapCameraPosition = .userLocation(fallback: .automatic)
    @State private var showCreateSheet = false
    @State private var selectedEcho: Echo?
    @State private var sheetDetent: PresentationDetent = .fraction(0.35)
    @State private var showNearbySheet = true
    @Namespace private var mapNamespace

    private var mainEchoes: [Echo] {
        store.echoes.filter { $0.parentEchoId == nil }
    }

    var body: some View {
        ZStack(alignment: .bottom) {
            Map(position: $position) {
                UserAnnotation()

                ForEach(mainEchoes) { echo in
                    Annotation("", coordinate: echo.coordinate) {
                        EchoAnnotationView(echo: echo)
                            .onTapGesture {
                                selectedEcho = echo
                            }
                    }
                }
            }
            .mapStyle(.standard(pointsOfInterest: .excludingAll))
            .mapControls {
                MapUserLocationButton()
                MapCompass()
            }
            .ignoresSafeArea()

            VStack(spacing: 0) {
                Spacer()

                Button {
                    showCreateSheet = true
                } label: {
                    ZStack {
                        Circle()
                            .fill(Theme.amber.opacity(0.2))
                            .frame(width: 72, height: 72)

                        Circle()
                            .fill(Theme.gradient)
                            .frame(width: 60, height: 60)
                            .shadow(color: Theme.amber.opacity(0.4), radius: 12, y: 4)

                        Image(systemName: "plus")
                            .font(.system(size: 24, weight: .bold))
                            .foregroundStyle(.white)
                    }
                }
                .sensoryFeedback(.impact(weight: .medium), trigger: showCreateSheet)
                .padding(.bottom, 8)
            }
        }
        .sheet(isPresented: $showNearbySheet) {
            NearbySheetView(selectedEcho: $selectedEcho)
                .presentationDetents([.fraction(0.35), .medium, .large], selection: $sheetDetent)
                .presentationDragIndicator(.visible)
                .presentationBackgroundInteraction(.enabled(upThrough: .medium))
                .presentationContentInteraction(.scrolls)
                .presentationCornerRadius(24)
                .presentationBackground(.ultraThinMaterial)
        }
        .sheet(isPresented: $showCreateSheet) {
            CreateEchoView()
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
                .presentationCornerRadius(24)
        }
        .sheet(item: $selectedEcho) { echo in
            EchoDetailView(echo: echo)
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
                .presentationCornerRadius(24)
        }
        .onAppear {
            locationService.requestPermission()
            locationService.startUpdating()
        }
    }
}

struct NearbySheetView: View {
    @Environment(EchoStore.self) private var store
    @Environment(LocationService.self) private var locationService
    @Binding var selectedEcho: Echo?

    private var nearbyEchoes: [Echo] {
        let mainEchoes = store.echoes.filter { $0.parentEchoId == nil }
        guard let location = locationService.location else { return mainEchoes }
        return mainEchoes.sorted { $0.distance(from: location) < $1.distance(from: location) }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                LazyVStack(spacing: 2) {
                    ForEach(nearbyEchoes) { echo in
                        Button {
                            selectedEcho = echo
                        } label: {
                            NearbyEchoRow(echo: echo, userLocation: locationService.location, store: store)
                        }
                        .buttonStyle(.plain)

                        if echo.id != nearbyEchoes.last?.id {
                            Divider()
                                .padding(.leading, 66)
                        }
                    }
                }
                .padding(.horizontal)
            }
            .navigationTitle("Nearby Echoes")
            .navigationBarTitleDisplayMode(.inline)
        }
    }
}
