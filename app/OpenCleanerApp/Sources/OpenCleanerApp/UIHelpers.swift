import Foundation
import OpenCleanerClient
import SwiftUI

let openCleanerCategories: [OpenCleanerClient.Category] = [.system, .developer, .apps]

extension OpenCleanerClient.Category {
    var title: String {
        switch self {
        case .system: return "System"
        case .developer: return "Developer"
        case .apps: return "Apps"
        }
    }

    var systemImageName: String {
        switch self {
        case .system: return "gear"
        case .developer: return "hammer"
        case .apps: return "square.grid.2x2"
        }
    }
}

extension OpenCleanerClient.SafetyLevel {
    var title: String {
        switch self {
        case .safe: return "Safe"
        case .moderate: return "Moderate"
        case .risky: return "Risky"
        }
    }

    var tint: Color {
        switch self {
        case .safe: return .green
        case .moderate: return .orange
        case .risky: return .red
        }
    }
}

func formatBytes(_ bytes: Int64) -> String {
    ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file)
}

func sumSizes(_ items: [ScanItem]) -> Int64 {
    items.reduce(0) { $0 + $1.size }
}
