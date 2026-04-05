import OpenCleanerClient
import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        Form {
            Section("Daemon") {
                TextField("Socket path", text: $model.socketPath)
                    .textSelection(.enabled)
                HStack {
                    Button("Use Default") { model.resetSocketPathToDefault() }
                    Spacer()
                    Button("Check Status") { model.refreshStatus() }
                }

                Text("Default: \(OpenCleanerDefaults.defaultSocketPath())")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }

            Section("Safety") {
                Text("Risky items are not selected by default. To include them, enable ‘Allow risky items (unsafe)’ in the Preview & Clean sheet.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(16)
        .frame(minWidth: 520)
    }
}
