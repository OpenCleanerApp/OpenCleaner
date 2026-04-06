import AppKit
import Foundation

enum PermissionService {
    /// Check Full Disk Access by probing ~/Library/Mail (protected path)
    static func hasFullDiskAccess() -> Bool {
        let testPath = "\(NSHomeDirectory())/Library/Mail"
        return FileManager.default.isReadableFile(atPath: testPath)
    }

    /// Deep-link to System Settings > Privacy > Full Disk Access
    static func openFullDiskAccessSettings() {
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles") {
            NSWorkspace.shared.open(url)
        }
    }
}
