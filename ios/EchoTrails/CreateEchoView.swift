import SwiftUI
import CoreLocation

struct CreateEchoView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(EchoStore.self) private var store
    @Environment(LocationService.self) private var locationService
    @State private var selectedType: EchoType = .text
    @State private var textContent: String = ""
    @State private var privacyLevel: PrivacyLevel = .fullProfile
    @State private var isRecording = false
    @State private var recordingProgress: Double = 0
    @State private var hasRecorded = false

    private var canDrop: Bool {
        switch selectedType {
        case .text: return !textContent.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        case .audio: return hasRecorded
        case .photo, .video: return true
        }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 24) {
                    echoTypeSelector

                    contentArea

                    privacySelector

                    locationInfo
                }
                .padding()
            }
            .navigationTitle("Drop an Echo")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Drop") {
                        dropEcho()
                    }
                    .fontWeight(.semibold)
                    .disabled(!canDrop)
                }
            }
        }
    }

    private var echoTypeSelector: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("TYPE")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)

            HStack(spacing: 10) {
                ForEach(EchoType.allCases) { type in
                    Button {
                        withAnimation(.snappy) { selectedType = type }
                    } label: {
                        VStack(spacing: 6) {
                            Image(systemName: type.icon)
                                .font(.system(size: 20))
                                .frame(width: 48, height: 48)
                                .background(
                                    selectedType == type
                                    ? Theme.amber.opacity(0.2)
                                    : Color(.tertiarySystemGroupedBackground)
                                )
                                .clipShape(Circle())

                            Text(type.label)
                                .font(.caption2.weight(.medium))
                        }
                        .foregroundStyle(selectedType == type ? Theme.amber : .secondary)
                        .frame(maxWidth: .infinity)
                    }
                    .sensoryFeedback(.selection, trigger: selectedType)
                }
            }
        }
    }

    @ViewBuilder
    private var contentArea: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("CONTENT")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)

            switch selectedType {
            case .text:
                TextEditor(text: $textContent)
                    .frame(minHeight: 120)
                    .scrollContentBackground(.hidden)
                    .padding(12)
                    .background(Color(.secondarySystemGroupedBackground))
                    .clipShape(.rect(cornerRadius: 12))
                    .overlay(alignment: .topLeading) {
                        if textContent.isEmpty {
                            Text("What's the story of this place?")
                                .foregroundStyle(.tertiary)
                                .padding(.horizontal, 16)
                                .padding(.vertical, 20)
                                .allowsHitTesting(false)
                        }
                    }

            case .audio:
                VStack(spacing: 16) {
                    ZStack {
                        Circle()
                            .fill(isRecording ? Theme.terracotta.opacity(0.15) : Theme.amber.opacity(0.1))
                            .frame(width: 100, height: 100)
                            .scaleEffect(isRecording ? 1.2 : 1.0)
                            .animation(.easeInOut(duration: 0.8).repeatForever(autoreverses: true), value: isRecording)

                        Circle()
                            .fill(isRecording ? Theme.terracotta : Theme.amber)
                            .frame(width: 72, height: 72)

                        Image(systemName: isRecording ? "stop.fill" : "mic.fill")
                            .font(.system(size: 28))
                            .foregroundStyle(.white)
                    }
                    .onTapGesture {
                        withAnimation(.snappy) {
                            if isRecording {
                                isRecording = false
                                hasRecorded = true
                            } else {
                                isRecording = true
                                hasRecorded = false
                                simulateRecording()
                            }
                        }
                    }
                    .sensoryFeedback(.impact(weight: .heavy), trigger: isRecording)

                    if isRecording {
                        VStack(spacing: 6) {
                            ProgressView(value: recordingProgress)
                                .tint(Theme.terracotta)

                            Text("\(Int(recordingProgress * 30))s / 30s")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    } else if hasRecorded {
                        HStack(spacing: 8) {
                            Image(systemName: "checkmark.circle.fill")
                                .foregroundStyle(.green)
                            Text("Audio recorded")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                        }
                    } else {
                        Text("Tap to record (up to 30s)")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
                .background(Color(.secondarySystemGroupedBackground))
                .clipShape(.rect(cornerRadius: 16))

            case .photo:
                VStack(spacing: 12) {
                    Image(systemName: "photo.on.rectangle.angled")
                        .font(.system(size: 40))
                        .foregroundStyle(Theme.amber.opacity(0.6))

                    Text("Photo capture will be available on your device")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 32)
                .background(Color(.secondarySystemGroupedBackground))
                .clipShape(.rect(cornerRadius: 16))

            case .video:
                VStack(spacing: 12) {
                    Image(systemName: "video.fill")
                        .font(.system(size: 40))
                        .foregroundStyle(Theme.amber.opacity(0.6))

                    Text("Video capture will be available on your device")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 32)
                .background(Color(.secondarySystemGroupedBackground))
                .clipShape(.rect(cornerRadius: 16))
            }

            if selectedType != .text {
                TextField("Add a caption...", text: $textContent)
                    .padding(12)
                    .background(Color(.secondarySystemGroupedBackground))
                    .clipShape(.rect(cornerRadius: 10))
            }
        }
    }

    private var privacySelector: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("PRIVACY")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)

            ForEach(PrivacyLevel.allCases) { level in
                Button {
                    withAnimation(.snappy) { privacyLevel = level }
                } label: {
                    HStack(spacing: 12) {
                        Image(systemName: level.icon)
                            .font(.system(size: 18))
                            .frame(width: 36, height: 36)
                            .background(
                                privacyLevel == level
                                ? Theme.amber.opacity(0.2)
                                : Color(.tertiarySystemGroupedBackground)
                            )
                            .clipShape(Circle())
                            .foregroundStyle(privacyLevel == level ? Theme.amber : .secondary)

                        VStack(alignment: .leading, spacing: 2) {
                            Text(level.label)
                                .font(.subheadline.weight(.medium))
                                .foregroundStyle(.primary)

                            Text(level.description)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }

                        Spacer()

                        if privacyLevel == level {
                            Image(systemName: "checkmark.circle.fill")
                                .foregroundStyle(Theme.amber)
                        }
                    }
                    .padding(12)
                    .background(Color(.secondarySystemGroupedBackground))
                    .clipShape(.rect(cornerRadius: 12))
                }
                .sensoryFeedback(.selection, trigger: privacyLevel)
            }
        }
    }

    private var locationInfo: some View {
        HStack(spacing: 10) {
            Image(systemName: "location.fill")
                .foregroundStyle(Theme.amber)

            VStack(alignment: .leading, spacing: 2) {
                Text("Dropping at")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                Text(locationService.placeName.isEmpty ? "Current Location" : locationService.placeName)
                    .font(.subheadline.weight(.medium))
            }

            Spacer()
        }
        .padding(12)
        .background(Color(.secondarySystemGroupedBackground))
        .clipShape(.rect(cornerRadius: 12))
    }

    private func dropEcho() {
        let coordinate = locationService.location?.coordinate ?? .init(latitude: 37.7749, longitude: -122.4194)
        store.addEcho(
            type: selectedType,
            textContent: textContent.isEmpty ? nil : textContent,
            privacyLevel: privacyLevel,
            coordinate: coordinate,
            locationName: locationService.placeName
        )
        dismiss()
    }

    private func simulateRecording() {
        recordingProgress = 0
        Task {
            for i in 1...30 {
                try? await Task.sleep(for: .seconds(1))
                guard isRecording else { return }
                recordingProgress = Double(i) / 30.0
            }
            isRecording = false
            hasRecorded = true
        }
    }
}
