import OpenCleanerClient
import SwiftUI

struct CleanSheetView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss

    @State private var dryRun: Bool = true
    @State private var allowRisky: Bool = false

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Preview & Clean")
                .font(.title2)

            if let scan = model.lastScan {
                let selected = scan.items.filter { model.selectedItemIDs.contains($0.id) }
                let rows = selected.sorted(by: { $0.size > $1.size })
                let totalSize = sumSizes(selected)
                let containsRisky = selected.contains { $0.safetyLevel == .risky }

                Text("\(selected.count) items • \(formatBytes(totalSize))")
                    .foregroundStyle(.secondary)
                    .monospacedDigit()

                Toggle("Dry run (no files will be deleted)", isOn: $dryRun)
                Toggle("Allow risky items (unsafe)", isOn: $allowRisky)

                if containsRisky && !allowRisky {
                    OCBanner(
                        title: "Risky items blocked",
                        message: "Risky items are selected. Enable ‘Allow risky items (unsafe)’ to proceed.",
                        systemImage: "exclamationmark.triangle.fill",
                        tint: .orange
                    )
                }

                Table(rows) {
                    TableColumn("Name") { item in
                        VStack(alignment: .leading, spacing: 2) {
                            Text(item.name)
                                .lineLimit(1)

                            Text(item.path)
                                .font(.caption.monospaced())
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                                .truncationMode(.middle)
                                .textSelection(.enabled)
                                .help(item.path)
                        }
                    }

                    TableColumn("Safety") { item in
                        SafetyBadge(level: item.safetyLevel)
                    }
                    .width(min: 80, ideal: 90)

                    TableColumn("Size") { item in
                        Text(formatBytes(item.size))
                            .foregroundStyle(.secondary)
                            .monospacedDigit()
                    }
                    .width(min: 90, ideal: 110)
                }

                HStack {
                    Button("Cancel") { dismiss() }
                        .keyboardShortcut(.cancelAction)

                    Spacer()

                    Button(dryRun ? "Run Dry-Run" : "Clean Now") {
                        model.cleanSelected(dryRun: dryRun, unsafe: allowRisky)
                        dismiss()
                    }
                    .keyboardShortcut(.defaultAction)
                    .disabled(!model.isOnline || selected.isEmpty || containsRisky && !allowRisky || model.activity != .idle)
                    .buttonStyle(.borderedProminent)
                }
            } else {
                ContentUnavailableView {
                    Label("No scan results", systemImage: "magnifyingglass")
                } description: {
                    Text("Run a scan before previewing a clean.")
                }

                HStack {
                    Spacer()
                    Button("Close") { dismiss() }
                        .keyboardShortcut(.defaultAction)
                }
            }
        }
        .padding(OCLayout.pagePadding)
        .frame(minWidth: 760, minHeight: 520)
    }
}
