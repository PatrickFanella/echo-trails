import Foundation

public enum MediaKind: String, CaseIterable, Sendable {
    case audio
    case photo
    case video
}

public struct MediaComposerLimits: Equatable, Sendable {
    public let maxPhotoCount: Int
    public let maxCaptionLength: Int
    public let maxBytes: Int

    public init(maxPhotoCount: Int = 5, maxCaptionLength: Int = 500, maxBytes: Int = 100 * 1024 * 1024) {
        self.maxPhotoCount = maxPhotoCount
        self.maxCaptionLength = maxCaptionLength
        self.maxBytes = maxBytes
    }
}

public struct MediaAttachmentDescriptor: Equatable, Sendable {
    public let localID: String
    public let kind: MediaKind
    public let contentType: String
    public let byteSize: Int
    public let caption: String

    public init(localID: String, kind: MediaKind, contentType: String, byteSize: Int, caption: String = "") {
        self.localID = localID
        self.kind = kind
        self.contentType = contentType
        self.byteSize = byteSize
        self.caption = caption.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

public struct MediaValidationResult: Equatable, Sendable {
    public let isValid: Bool
    public let reasons: [String]

    public static func validate(_ attachments: [MediaAttachmentDescriptor], limits: MediaComposerLimits = MediaComposerLimits()) -> MediaValidationResult {
        var reasons: [String] = []
        if attachments.isEmpty { reasons.append("media_required") }
        if attachments.filter({ $0.kind == .photo }).count > limits.maxPhotoCount { reasons.append("too_many_photos") }
        if Set(attachments.map(\.kind)).count > 1 { reasons.append("one_media_kind_only") }
        if attachments.contains(where: { $0.byteSize <= 0 || $0.byteSize > limits.maxBytes }) { reasons.append("invalid_media_size") }
        if attachments.contains(where: { $0.caption.count > limits.maxCaptionLength }) { reasons.append("caption_too_long") }
        return MediaValidationResult(isValid: reasons.isEmpty, reasons: reasons)
    }
}

public struct UploadIntent: Equatable, Sendable {
    public let echoID: String
    public let attachment: MediaAttachmentDescriptor
    public let uploadAPIPath: String

    public init(echoID: String, attachment: MediaAttachmentDescriptor) {
        self.echoID = echoID
        self.attachment = attachment
        self.uploadAPIPath = OpenAPIPaths.createMediaUpload(echoId: echoID)
    }
}

public struct UploadStatusDescriptor: Equatable, Sendable {
    public let uploadID: String
    public let objectKey: String
    public let activityStatus: String

    public init(uploadID: String, objectKey: String, activityStatus: String) {
        self.uploadID = uploadID
        self.objectKey = objectKey
        self.activityStatus = activityStatus
    }

    public var usesOpaqueIdentifiers: Bool {
        (uploadID.hasPrefix("upl_") || UUID(uuidString: uploadID) != nil) && objectKey.hasPrefix("media/original/")
    }
}
