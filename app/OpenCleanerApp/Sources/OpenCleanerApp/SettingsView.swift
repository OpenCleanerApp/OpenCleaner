import OpenCleanerClient
import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var model: AppModel

    @State private var draftSocketPath: String = ""
    @State private var draftExcludedPathsText: String = ""

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

    private func normalizedExcludedPaths(from text: String) -> [String] {
        let home = FileManager.default.homeDirectoryForCurrentUser.path

        var out: [String] = []
        var seen: Set<String> = []

        for line in text.components(separatedBy: .newlines) {
            let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else { continue }

            let expanded: String
            if trimmed.hasPrefix("~") {
                expanded = (trimmed as NSString).expandingTildeInPath
            } else if trimmed.hasPrefix("/") {
                expanded = trimmed
            } else {
                expanded = URL(fileURLWithPath: home).appendingPathComponent(trimmed).path
            }

            let standardized = (expanded as NSString).standardizingPath
            guard !standardized.isEmpty else { continue }

            if !seen.contains(standardized) {
                seen.insert(standardized)
                out.append(standardized)
            }
        }

        return out
    }

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

                HStack(alignment: .top) {
                    Text("Exclude")
                        .frame(width: OCLayout.labelColumnWidth, alignment: .leading)
                        .foregroundStyle(.secondary)

                    TextEditor(text: $draftExcludedPathsText)
                        .font(.callout.monospaced())
                        .frame(minHeight: 120)
                        .textSelection(.enabled)
                }

                Text("One path per line. Excluded paths are never cleaned; any selected item under an excluded prefix will be skipped.")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                HStack {
                    Button("Clear") { draftExcludedPathsText = "" }

                    Spacer()

                    Button("Apply Excludes") {
                        let normalized = normalizedExcludedPaths(from: draftExcludedPathsText)
                        model.excludedPaths = normalized
                        draftExcludedPathsText = normalized.joined(separator: "\n")
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(normalizedExcludedPaths(from: draftExcludedPathsText) == model.excludedPaths)
                }
            }
        }
        .padding(OCLayout.pagePadding)
        .frame(minWidth: 560)
        .onAppear {
            if draftSocketPath.isEmpty {
                draftSocketPath = model.socketPath
            }
            if draftExcludedPathsText.isEmpty {
                draftExcludedPathsText = model.excludedPaths.joined(separator: "\n")
            }
        }
        .onChange(of: model.socketPath) { _, newValue in
            if draftSocketPath != newValue {
                draftSocketPath = newValue
            }
        }
        .onChange(of: model.excludedPaths) { _, newValue in
            let rendered = newValue.joined(separator: "\n")
            if draftExcludedPathsText != rendered {
                draftExcludedPathsText = rendered
            }
        }
        .navigationTitle("Settings")
    }
}
