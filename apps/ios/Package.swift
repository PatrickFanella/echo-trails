// swift-tools-version: 5.10

import PackageDescription

let package = Package(
    name: "EchoTrailsShell",
    platforms: [.iOS(.v17)],
    products: [
        .library(name: "EchoTrailsShell", targets: ["EchoTrailsShell"]),
    ],
    targets: [
        .target(name: "EchoTrailsShell"),
        .testTarget(name: "EchoTrailsShellTests", dependencies: ["EchoTrailsShell"]),
    ]
)
