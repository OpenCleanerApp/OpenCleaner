import OpenCleanerClient
import Sparkle
import SwiftUI

@main
struct OpenCleanerApp: App {
    @StateObject private var model = AppModel()
    private let updaterController = SPUStandardUpdaterController(
        startingUpdater: true,
        updaterDelegate: nil,
        userDriverDelegate: nil
    )

    var body: some Scene {
        MenuBarExtra("OpenCleaner", systemImage: "sparkles") {
            MenuBarPopoverView()
                .environmentObject(model)
        }
        .menuBarExtraStyle(.window)

        WindowGroup("OpenCleaner", id: "main") {
            Group {
                if model.hasCompletedOnboarding {
                    MainWindowView()
                } else {
                    OnboardingView()
                }
            }
            .environmentObject(model)
        }
        .defaultSize(width: 980, height: 680)
        .commands {
            CommandGroup(after: .newItem) {
                Button("Scan") {
                    model.scan()
                }
                .keyboardShortcut("s", modifiers: [.command])
                .disabled(!model.isOnline || model.activity != .idle)

                Button("Clean Selected…") {
                    model.showingCleanSheet = true
                }
                .keyboardShortcut(.delete, modifiers: [.command])
                .disabled(model.selectedItemIDs.isEmpty)

                Divider()

                Button("Undo Last Clean") {
                    model.undoLastClean()
                }
                .keyboardShortcut("z", modifiers: [.command])
                .disabled(model.lastCleanResult == nil)

                Button("Select All") {
                    model.selectAll()
                }
                .keyboardShortcut("a", modifiers: [.command])
            }
        }

        Settings {
            SettingsView(updater: updaterController.updater)
                .environmentObject(model)
        }
    }
}
