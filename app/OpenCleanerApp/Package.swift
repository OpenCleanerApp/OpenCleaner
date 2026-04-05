// swift-tools-version: 6.3

import PackageDescription

let package = Package(
    name: "OpenCleanerApp",
    platforms: [
        .macOS(.v14)
    ],
    dependencies: [
        .package(path: "../Packages/OpenCleanerClient")
    ],
    targets: [
        .executableTarget(
            name: "OpenCleanerApp",
            dependencies: [
                .product(name: "OpenCleanerClient", package: "OpenCleanerClient")
            ]
        ),
        .testTarget(
            name: "OpenCleanerAppTests",
            dependencies: ["OpenCleanerApp"]
        ),
    ],
    swiftLanguageModes: [.v6]
)
