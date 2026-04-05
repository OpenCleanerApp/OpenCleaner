import OpenCleanerClient
import SwiftUI

@main
struct OpenCleanerApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        MenuBarExtra("OpenCleaner", systemImage: "sparkles") {
            MenuBarPopoverView()
                .environmentObject(model)
        }
        .menuBarExtraStyle(.window)

        WindowGroup("OpenCleaner", id: "main") {
            MainWindowView()
                .environmentObject(model)
        }
        .defaultSize(width: 980, height: 680)

        Settings {
            SettingsView()
                .environmentObject(model)
        }
    }
}
