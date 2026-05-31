import Foundation

public enum EchoContentMode: String, CaseIterable, Sendable {
    case text
    case audio
    case photoSet
    case video
}

public enum CreateFlowStep: String, CaseIterable, Sendable {
    case choosePlace
    case captureProof
    case composeEcho
    case review
    case submit
}

public struct CreationTokenDescriptor: Equatable, Sendable {
    public let tokenID: String
    public let placeID: String
    public let expiresAt: Date

    public init(tokenID: String, placeID: String, expiresAt: Date) {
        self.tokenID = tokenID
        self.placeID = placeID
        self.expiresAt = expiresAt
    }

    public func isValid(at now: Date) -> Bool {
        !tokenID.isEmpty && !placeID.isEmpty && expiresAt > now
    }
}

public struct LocationProofPreflight: Equatable, Sendable {
    public let placeID: String
    public let requiresFreshProof: Bool
    public let creationToken: CreationTokenDescriptor?

    public init(placeID: String, requiresFreshProof: Bool = true, creationToken: CreationTokenDescriptor? = nil) {
        self.placeID = placeID
        self.requiresFreshProof = requiresFreshProof
        self.creationToken = creationToken
    }
}

public struct EchoDraft: Equatable, Sendable {
    public let placeID: String
    public let mode: EchoContentMode
    public let title: String
    public let body: String
    public let caption: String
    public let creationToken: CreationTokenDescriptor?

    public init(placeID: String, mode: EchoContentMode, title: String = "", body: String = "", caption: String = "", creationToken: CreationTokenDescriptor? = nil) {
        self.placeID = placeID
        self.mode = mode
        self.title = title.trimmingCharacters(in: .whitespacesAndNewlines)
        self.body = body.trimmingCharacters(in: .whitespacesAndNewlines)
        self.caption = caption.trimmingCharacters(in: .whitespacesAndNewlines)
        self.creationToken = creationToken
    }
}

public struct EchoDraftValidation: Equatable, Sendable {
    public let isValid: Bool
    public let reasons: [String]

    public static func validate(_ draft: EchoDraft, now: Date) -> EchoDraftValidation {
        var reasons: [String] = []
        if draft.placeID.isEmpty { reasons.append("place_required") }
        if draft.title.count > 80 { reasons.append("title_too_long") }
        if draft.body.count > 2_000 { reasons.append("body_too_long") }
        if draft.caption.count > 500 { reasons.append("caption_too_long") }
        if draft.mode == .text && draft.body.isEmpty { reasons.append("body_required") }
        if draft.creationToken?.isValid(at: now) != true { reasons.append("creation_token_required") }
        return EchoDraftValidation(isValid: reasons.isEmpty, reasons: reasons)
    }
}
