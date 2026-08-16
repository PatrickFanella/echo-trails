import Foundation

public enum ContractPath {
    public static func safeSegment(_ value: String) -> String? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, trimmed.count <= 128, !trimmed.contains("..") else { return nil }
        let allowed = trimmed.allSatisfy { character in
            (character >= "a" && character <= "z") ||
                (character >= "A" && character <= "Z") ||
                (character >= "0" && character <= "9") ||
                character == "-" || character == "_"
        }
        return allowed ? trimmed : nil
    }

    public static func place(_ placeID: String) -> String? {
        safeSegment(placeID).map { OpenAPIPaths.getPlace(placeId: $0) }
    }

    public static func placeEchoes(_ placeID: String) -> String? {
        safeSegment(placeID).map { OpenAPIPaths.listPlaceEchoes(placeId: $0) }
    }

    public static func userProfile(_ handle: String) -> String? {
        safeSegment(handle).map { OpenAPIPaths.getPublicProfile(handle: $0) }
    }
}
