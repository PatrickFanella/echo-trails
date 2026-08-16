import XCTest
@testable import EchoTrailsShell

final class MediaComposerContractsTests: XCTestCase {
    func testMediaValidationAllowsOneToFivePhotos() {
        let photos = (1...5).map { MediaAttachmentDescriptor(localID: "photo-\($0)", kind: .photo, contentType: "image/jpeg", byteSize: 10) }

        XCTAssertTrue(MediaValidationResult.validate(photos).isValid)
        XCTAssertFalse(MediaValidationResult.validate(photos + [MediaAttachmentDescriptor(localID: "photo-6", kind: .photo, contentType: "image/jpeg", byteSize: 10)]).isValid)
    }

    func testMediaValidationAllowsOneKindOnlyAndCaptionLimit() {
        let mixed = [
            MediaAttachmentDescriptor(localID: "photo", kind: .photo, contentType: "image/jpeg", byteSize: 10),
            MediaAttachmentDescriptor(localID: "audio", kind: .audio, contentType: "audio/mpeg", byteSize: 10)
        ]
        let longCaption = [MediaAttachmentDescriptor(localID: "video", kind: .video, contentType: "video/mp4", byteSize: 10, caption: String(repeating: "c", count: 501))]

        XCTAssertTrue(MediaValidationResult.validate(mixed).reasons.contains("one_media_kind_only"))
        XCTAssertTrue(MediaValidationResult.validate(longCaption).reasons.contains("caption_too_long"))
    }

    func testUploadIntentUsesMediaEndpointAndOpaqueStatus() {
        let attachment = MediaAttachmentDescriptor(localID: "audio", kind: .audio, contentType: "audio/mpeg", byteSize: 10)
        let intent = UploadIntent(echoID: "echo-local", attachment: attachment)
        let status = UploadStatusDescriptor(uploadID: "upl_123", objectKey: "media/original/ast_123/mp3", activityStatus: "processing")

        XCTAssertEqual(intent.uploadAPIPath, "/echoes/echo-local/media/uploads")
        XCTAssertTrue(OpenAPIPaths.all.contains("/echoes/{echoId}/media/uploads"))
        XCTAssertTrue(status.usesOpaqueIdentifiers)
    }
}
