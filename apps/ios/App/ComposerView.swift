import AVFoundation
import PhotosUI
import Observation
import SwiftData
import SwiftUI
import UniformTypeIdentifiers
import UIKit

struct CreateEchoBody: Codable {
    let placeID, contentMode, title, bodyText, captionText, creationToken: String
    enum CodingKeys: String, CodingKey { case placeID = "place_id", contentMode = "content_mode", title, bodyText = "body_text", captionText = "caption_text", creationToken = "creation_token" }
}
struct CreateUploadBody: Codable { let mediaType, contentType: String; let byteSize, sortOrder: Int; enum CodingKeys: String, CodingKey { case mediaType = "media_type", contentType = "content_type", byteSize = "byte_size", sortOrder = "sort_order" } }
struct StitchCreateBody: Codable {
    let placeID, contentMode, title, body, captionText, creationToken: String
    let quotedReplyID: String?
    let quoteWarningAcknowledged: Bool
    enum CodingKeys: String, CodingKey { case title, body; case placeID = "place_id", contentMode = "content_mode", captionText = "caption_text", creationToken = "creation_token", quotedReplyID = "quoted_reply_id", quoteWarningAcknowledged = "quote_warning_acknowledged" }
}
struct StitchResponseDTO: Decodable { let echoID: String; enum CodingKeys: String, CodingKey { case echoID = "echo_id" } }
struct UploadResponseDTO: Decodable { let uploadID, mediaAssetID, uploadURL, status: String; enum CodingKeys: String, CodingKey { case status; case uploadID = "upload_id", mediaAssetID = "media_asset_id", uploadURL = "upload_url" } }

/// A checkpointed composer transaction. The creation proof and every idempotency
/// key are captured before the first draft request is sent. `echoID` and upload
/// IDs are filled in only after the corresponding server acknowledgement has
/// been durably saved, so an ambiguous network failure can safely replay the
/// exact same operation.
struct ComposerTransactionPayload: Codable, Equatable {
    struct Attachment: Codable, Equatable {
        let localFilePath, mediaType, contentType: String
        let byteSize, sortOrder: Int
        let uploadIdempotencyKey: String
        var uploadID: String?
        var mediaAssetID: String?
        var uploadURL: String?

        init(file: LocalMedia, sortOrder: Int, uploadIdempotencyKey: String = UUID().uuidString) {
            localFilePath = file.url.path; mediaType = file.mediaType; contentType = file.contentType
            byteSize = file.byteSize; self.sortOrder = sortOrder; self.uploadIdempotencyKey = uploadIdempotencyKey
        }
    }

    let placeID, contentMode, title, bodyText, captionText, creationToken: String
    let creationTokenExpiresAt: Date
    let sourceEchoID, quotedReplyID: String?
    let quoteWarningAcknowledged: Bool
    let draftIdempotencyKey, submitIdempotencyKey: String
    var echoID: String?
    var attachments: [Attachment]

    init(placeID: String, contentMode: String, title: String, bodyText: String, captionText: String,
         creationToken: String, creationTokenExpiresAt: Date, sourceEchoID: String?, quotedReplyID: String?,
         quoteWarningAcknowledged: Bool, media: [LocalMedia], draftIdempotencyKey: String = UUID().uuidString,
         submitIdempotencyKey: String = UUID().uuidString) {
        self.placeID = placeID; self.contentMode = contentMode; self.title = title; self.bodyText = bodyText
        self.captionText = captionText; self.creationToken = creationToken; self.creationTokenExpiresAt = creationTokenExpiresAt
        self.sourceEchoID = sourceEchoID; self.quotedReplyID = quotedReplyID
        self.quoteWarningAcknowledged = quoteWarningAcknowledged; self.draftIdempotencyKey = draftIdempotencyKey
        self.submitIdempotencyKey = submitIdempotencyKey; attachments = media.enumerated().map { Attachment(file: $0.element, sortOrder: $0.offset) }
    }
}

enum ComposerMode: String, CaseIterable, Identifiable { case text = "Text", photo = "Photos", audio = "Audio", video = "Video"; var id: String { rawValue }; var apiValue: String { switch self { case .text: "text"; case .photo: "photo_set"; case .audio: "audio"; case .video: "video" } } }
struct LocalMedia: Identifiable { let id = UUID(); let url: URL; let mediaType, contentType: String; let byteSize: Int; let durationSeconds: Double? }

struct ComposerView: View {
    @EnvironmentObject private var app: AppModel
    @Environment(\.dismiss) private var dismiss
    let place: PlaceDTO
    var sourceEchoID: String? = nil
    var quotedReplyID: String? = nil
    @State private var mode: ComposerMode = .text
    @State private var title = ""
    @State private var story = ""
    @State private var caption = ""
    @State private var selections: [PhotosPickerItem] = []
    @State private var media: [LocalMedia] = []
    @State private var recorder = AudioRecorder()
    @State private var isCreating = false
    @State private var localError: String?
    @State private var showingCamera = false
    @State private var quoteAcknowledged = false

    var body: some View {
        Form {
            Section("Place") { Text(place.name); Text("Location is checked only when you create.").font(.caption).foregroundStyle(.secondary) }
            if requiresQuoteAcknowledgement { Section("Reply screenshot") { Text("A canonical screenshot of the current Reply revision becomes part of this Stitch. Normal edits or deletion do not change it; privacy deletion or safety removal suppresses it."); Toggle("I understand", isOn: $quoteAcknowledged) } }
            Picker("Echo type", selection: $mode) { ForEach(ComposerMode.allCases) { Text($0.rawValue).tag($0) } }.pickerStyle(.segmented).onChange(of: mode) { recorder.cancel(); media = []; selections = [] }
            Section("Story") {
                TextField("Title", text: $title).textInputAutocapitalization(.sentences)
                if mode == .text { TextField("What happened here?", text: $story, axis: .vertical).lineLimit(4...10) }
                else { TextField("Caption (optional)", text: $caption, axis: .vertical).lineLimit(2...6) }
            }
            mediaSection
            if let mediaValidationError { Text(mediaValidationError).foregroundStyle(.red).accessibilityLabel("Media requirement: \(mediaValidationError)") }
            if let localError { Text(localError).foregroundStyle(.red).accessibilityLabel("Error: \(localError)") }
            Button { Task { await create() } } label: { if isCreating { ProgressView() } else { Label("Create Echo", systemImage: "waveform.path.ecg") } }
                .disabled(!valid || isCreating).frame(maxWidth: .infinity)
        }
        .navigationTitle("New Echo")
        .onChange(of: selections) { Task { await importSelections() } }
        .onChange(of: recorder.completionCount) { if let completed = recorder.completedMedia { media = [completed] } }
        .onDisappear { recorder.cancel() }
        .fullScreenCover(isPresented: $showingCamera) { CameraCaptureView(mediaType: mode == .video ? .movie : .image) { captured in if mode == .photo { media.append(captured) } else { media = [captured] }; showingCamera = false } onCancel: { showingCamera = false } }
    }

    @ViewBuilder private var mediaSection: some View {
        switch mode {
        case .text: EmptyView()
        case .photo:
            Section("Photos") { PhotosPicker(selection: $selections, maxSelectionCount: 5, matching: .images) { Label("Choose 1–5 photos", systemImage: "photo.on.rectangle") }; if UIImagePickerController.isSourceTypeAvailable(.camera) { Button("Take photo") { showingCamera = true } }; Text("\(media.count) selected").foregroundStyle(.secondary) }
        case .video:
            Section("Video") { PhotosPicker(selection: $selections, maxSelectionCount: 1, matching: .videos) { Label("Choose a video up to 60 seconds", systemImage: "video") }; if UIImagePickerController.isSourceTypeAvailable(.camera) { Button("Record video") { showingCamera = true } }; Text(media.first?.url.lastPathComponent ?? "No video selected").foregroundStyle(.secondary) }
        case .audio:
            Section("Audio") { Button(recorder.isRecording ? "Stop recording" : "Record up to 2 minutes") { if recorder.isRecording { if let file = recorder.stop() { media = [file] } } else { do { try recorder.start() } catch { localError = error.localizedDescription } } }; if let first = media.first { Text(first.url.lastPathComponent).foregroundStyle(.secondary) } }
        }
    }

    private var requiresQuoteAcknowledgement: Bool { quotedReplyID != nil && !app.quoteWarningAcknowledged }
    private var valid: Bool { !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && title.count <= 80 && (mode == .text ? (!story.isEmpty && story.count <= 500) : (!media.isEmpty && caption.count <= 500 && mediaValidationError == nil)) && (!requiresQuoteAcknowledgement || quoteAcknowledged) }

    private var mediaValidationError: String? {
        switch mode {
        case .text: return nil
        case .photo:
            guard (1...5).contains(media.count) else { return "Choose between one and five photos." }
            if media.contains(where: { $0.byteSize > 20 * 1024 * 1024 }) { return "Each photo must be 20 MB or smaller." }
            if media.reduce(0, { $0 + $1.byteSize }) > 75 * 1024 * 1024 { return "The photo set must be 75 MB or smaller." }
        case .audio:
            guard media.count == 1 else { return "Choose one audio recording." }
            if media[0].byteSize > 25 * 1024 * 1024 { return "Audio must be 25 MB or smaller." }
            if let duration = media[0].durationSeconds, duration > 120.25 { return "Audio must be two minutes or shorter." }
        case .video:
            guard media.count == 1 else { return "Choose one video." }
            if media[0].byteSize > 100 * 1024 * 1024 { return "Video must be 100 MB or smaller." }
            if let duration = media[0].durationSeconds, duration > 60.25 { return "Video must be one minute or shorter." }
        }
        return nil
    }

    private func importSelections() async {
        var imported: [LocalMedia] = []
        for item in selections {
            guard let data = try? await item.loadTransferable(type: Data.self) else { continue }
            let isVideo = mode == .video
            var persistedData = data
            var imageType = Self.imageType(data)
            // Echo photo contract is JPEG/HEIC. Normalize PNG picker input to
            // JPEG before it enters the durable outbox.
            if !isVideo, imageType.contentType == "image/png", let image = UIImage(data: data), let jpeg = image.jpegData(compressionQuality: 0.92) {
                persistedData = jpeg
                imageType = ("image/jpeg", "jpg")
            }
            let ext = isVideo ? "mov" : imageType.extension; let type = isVideo ? "video/quicktime" : imageType.contentType
            if let url = try? OutboxFiles.persist(data: persistedData, extension: ext) {
                let duration = isVideo ? (try? await AVURLAsset(url: url).load(.duration).seconds) : nil
                imported.append(LocalMedia(url: url, mediaType: isVideo ? "video" : "image", contentType: type, byteSize: persistedData.count, durationSeconds: duration))
            }
        }
        media = imported
    }

    private static func imageType(_ data: Data) -> (contentType: String, extension: String) {
        if data.starts(with: [0x89, 0x50, 0x4e, 0x47]) { return ("image/png", "png") }
        if data.count > 12, String(data: data[4..<12], encoding: .ascii)?.contains("ftyp") == true { return ("image/heic", "heic") }
        return ("image/jpeg", "jpg")
    }

    private func create() async {
        isCreating = true; localError = nil; defer { isCreating = false }
        do {
            let proof = try await LocationProofProvider().token(for: place.id, client: app.client)
            let transaction = ComposerTransactionPayload(
                placeID: place.id, contentMode: mode.apiValue,
                title: title.trimmingCharacters(in: .whitespacesAndNewlines),
                bodyText: mode == .text ? story : "", captionText: mode == .text ? "" : caption,
                creationToken: proof.creationToken, creationTokenExpiresAt: proof.expiresAt,
                sourceEchoID: sourceEchoID, quotedReplyID: quotedReplyID,
                quoteWarningAcknowledged: quoteAcknowledged || app.quoteWarningAcknowledged, media: media
            )
            try app.enqueueComposer(transaction)
            // Try immediately for the responsive online path. Regardless of the
            // result, the transaction was saved first and foreground/bootstrap
            // processing will resume it with the captured proof and stable keys.
            await app.processOutbox()
            dismiss()
        } catch { localError = error.localizedDescription }
    }
}

private enum AudioRecorderError: LocalizedError {
    case couldNotStart
    var errorDescription: String? { "Audio recording could not start. Check microphone permission and try again." }
}

@Observable final class AudioRecorder: NSObject, AVAudioRecorderDelegate {
    var isRecording = false
    var completedMedia: LocalMedia?
    var completionCount = 0
    private var recorder: AVAudioRecorder?
    private var url: URL?
    func start() throws {
        let session = AVAudioSession.sharedInstance(); try session.setCategory(.record, mode: .spokenAudio); try session.setActive(true)
        let target = try OutboxFiles.newURL(extension: "m4a"); url = target; completedMedia = nil
        recorder = try AVAudioRecorder(url: target, settings: [AVFormatIDKey: kAudioFormatMPEG4AAC, AVSampleRateKey: 44_100, AVNumberOfChannelsKey: 1, AVEncoderBitRateKey: 96_000])
        recorder?.delegate = self
        guard recorder?.record(forDuration: 120) == true else { throw AudioRecorderError.couldNotStart }
        isRecording = true
    }
    func stop() -> LocalMedia? {
        let duration = recorder?.currentTime
        recorder?.delegate = nil; recorder?.stop(); isRecording = false
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        let captured = media(duration: duration); completedMedia = captured; recorder = nil
        return captured
    }
    func cancel() {
        guard recorder != nil else { return }
        recorder?.delegate = nil; recorder?.stop(); isRecording = false
        if completedMedia == nil, let url { try? FileManager.default.removeItem(at: url) }
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        recorder = nil; url = nil
    }
    func audioRecorderDidFinishRecording(_ recorder: AVAudioRecorder, successfully flag: Bool) {
        isRecording = false
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        if flag, let captured = media(duration: recorder.currentTime > 0 ? recorder.currentTime : 120) {
            completedMedia = captured; completionCount += 1
        } else if let url { try? FileManager.default.removeItem(at: url) }
    }
    private func media(duration: Double?) -> LocalMedia? {
        guard let url, let size = try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize else { return nil }
        return LocalMedia(url: url, mediaType: "audio", contentType: "audio/mp4", byteSize: size, durationSeconds: duration)
    }
}

enum OutboxFiles {
    static func directory() throws -> URL { let base = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true).appendingPathComponent("OutboxMedia", isDirectory: true); try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true); return base }
    static func newURL(extension ext: String) throws -> URL { try directory().appendingPathComponent(UUID().uuidString).appendingPathExtension(ext) }
    static func persist(data: Data, extension ext: String) throws -> URL { let url = try newURL(extension: ext); try data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication]); return url }
}

struct CameraCaptureView: UIViewControllerRepresentable {
    let mediaType: UTType
    let onCapture: (LocalMedia) -> Void
    let onCancel: () -> Void
    func makeCoordinator() -> Coordinator { Coordinator(parent: self) }
    func makeUIViewController(context: Context) -> UIImagePickerController {
        let picker = UIImagePickerController(); picker.sourceType = .camera; picker.mediaTypes = [mediaType.identifier]; picker.videoMaximumDuration = 60; picker.videoQuality = .typeHigh; picker.delegate = context.coordinator; return picker
    }
    func updateUIViewController(_ uiViewController: UIImagePickerController, context: Context) {}
    final class Coordinator: NSObject, UINavigationControllerDelegate, UIImagePickerControllerDelegate {
        let parent: CameraCaptureView
        init(parent: CameraCaptureView) { self.parent = parent }
        func imagePickerControllerDidCancel(_ picker: UIImagePickerController) { parent.onCancel() }
        func imagePickerController(_ picker: UIImagePickerController, didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey : Any]) {
            if let image = info[.originalImage] as? UIImage, let data = image.jpegData(compressionQuality: 0.92), let url = try? OutboxFiles.persist(data: data, extension: "jpg") { parent.onCapture(LocalMedia(url: url, mediaType: "image", contentType: "image/jpeg", byteSize: data.count, durationSeconds: nil)); return }
            if let source = info[.mediaURL] as? URL, let data = try? Data(contentsOf: source), let url = try? OutboxFiles.persist(data: data, extension: "mov") {
                Task { @MainActor in
                    let duration = try? await AVURLAsset(url: url).load(.duration).seconds
                    parent.onCapture(LocalMedia(url: url, mediaType: "video", contentType: "video/quicktime", byteSize: data.count, durationSeconds: duration))
                }
                return
            }
            parent.onCancel()
        }
    }
}
