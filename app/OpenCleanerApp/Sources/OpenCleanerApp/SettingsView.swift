import OpenCleanerClient
import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var model: AppModel

    @State private var draftSocketPath: String = ""

    private var statusColor: Color {
        switch model.connectionState {
        case .online:
            return .green
        case .offline:
            return .red
        case .unknown:
            return .gray
        }
    }

    private var defaultSocketPath: String { OpenCleanerDefaults.defaultSocketPath() }

    var body: some View {
        Form {
            Section("Daemon") {
                HStack(alignment: .firstTextBaseline) {
                    Text("Status")
                        .frame(width: OCLayout.labelColumnWidth, alignment: .leading)
                        .foregroundStyle(.secondary)

                    HStack(spacing: 8) {
                        Circle()
                            .fill(statusColor)
                            .frame(width: 8, height: 8)
                        Text(model.statusLine)
                    }

                    Spacer(minLength: 0)

                    Button("Refresh") { model.refreshStatus() }
                }

                HStack(alignment: .firstTextBaseline) {
                    Text("Socket")
                        .frame(width: OCLayout.labelColumnWidth, alignment: .leading)
                        .foregroundStyle(.secondary)

                    TextField("/tmp/opencleaner.<uid>.sock", text: $draftSocketPath)
                        .font(.callout.monospaced())
                        .textSelection(.enabled)
                }

                Text(draftSocketPath == defaultSocketPath
                        ? "Using default socket path."
                        : "Custom socket path override. Default: \(defaultSocketPath)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)

                HStack {
                    Button("Use Default") {
                        draftSocketPath = defaultSocketPath
                    }

                    Spacer()

                    Button("Apply") {
                        let trimmed = draftSocketPath.trimmingCharacters(in: .whitespacesAndNewlines)
                        guard !trimmed.isEmpty else { return }
                        model.socketPath = trimmed
                        model.refreshStatus()
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(
                        draftSocketPath.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                            || draftSocketPath.trimmingCharacters(in: .whitespacesAndNewlines) == model.socketPath
                    )
                }
            }

            Section("Safety") {
                Text("Risky items are not selected by default. To include them, enable ‘Allow risky items (unsafe)’ in the Preview & Clean sheet.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(OCLayout.pagePadding)
        .frame(minWidth: 560)
        .onAppear {
            if draftSocketPath.isEmpty {
                draftSocketPath = model.socketPath
            }
        }
        .onChange(of: model.socketPath) { _, newValue in
            if draftSocketPath != newValue {
                draftSocketPath = newValue
            }
        }
        .navigationTitle("Settings")
    }
}
