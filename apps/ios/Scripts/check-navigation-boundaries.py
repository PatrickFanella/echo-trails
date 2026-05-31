#!/usr/bin/env python3
"""Linux-safe checks for the pure Swift navigation shell boundary."""

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
SOURCES = ROOT / "Sources" / "EchoTrailsShell"
TESTS = ROOT / "Tests" / "EchoTrailsShellTests"

FORBIDDEN_IMPORTS = [
    "SwiftUI",
    "MapKit",
    "SwiftData",
    "AuthenticationServices",
    "Security",
]

FORBIDDEN_IMPORT_PATTERN = re.compile(
    r"^\s*(?:@_exported\s+)?import(?:\s+\w+)?\s+(SwiftUI|MapKit|SwiftData|AuthenticationServices|Security)\b",
    re.MULTILINE,
)

REQUIRED_SNIPPETS = {
    SOURCES / "AppIdentity.swift": ["case map", "case explore", "case activity", "case profile"],
    SOURCES / "NavigationRoute.swift": ["case placeDetail", "case echoDetail", "case userProfile", "case mediaUploadStatus", "requiresAuthentication"],
    SOURCES / "NavigationState.swift": ["ScreenDescriptor", "requiredAPIPath", "accessibilityLabel"],
    TESTS / "NavigationShellTests.swift": ["testDefaultNavigationStartsOnMapTab", "testDeepLinkParsingForCoreRoutes", "testRejectsUnsafeOrUnknownRoutes"],
}


def main() -> int:
    for file_path, snippets in REQUIRED_SNIPPETS.items():
        if not file_path.exists():
            print(f"missing required file: {file_path}", file=sys.stderr)
            return 1
        text = file_path.read_text(encoding="utf-8")
        for snippet in snippets:
            if snippet not in text:
                print(f"{file_path} missing required snippet: {snippet}", file=sys.stderr)
                return 1

    for swift_file in SOURCES.rglob("*.swift"):
        text = swift_file.read_text(encoding="utf-8")
        match = FORBIDDEN_IMPORT_PATTERN.search(text)
        if match:
            print(f"forbidden framework import in #36 navigation shell: {swift_file}: {match.group(0).strip()}", file=sys.stderr)
            return 1

    identity = (SOURCES / "AppIdentity.swift").read_text(encoding="utf-8")
    order = [identity.index(f"case {tab}") for tab in ["map", "explore", "activity", "profile"]]
    if order != sorted(order):
        print("ShellTabs order must be map, explore, activity, profile", file=sys.stderr)
        return 1

    print("iOS navigation boundary check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
