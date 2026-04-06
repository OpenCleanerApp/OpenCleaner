import OpenCleanerClient
import SwiftUI

struct CleanSheetView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss

    @State private var dryRun: Bool = true
    @State private var allowRisky: Bool = false
    @State private var showConfirmClean: Bool = false

    private func isExcluded(_ path: String, excludedPaths: [String]) -> Bool {
        guard !excludedPaths.isEmpty else { return false }

        let p = (path as NSString).standardizingPath
        for ex in excludedPaths {
            let e = (ex as NSString).standardizingPath
            if p == e || p.hasPrefix(e + "/") {
                return true
            }
        }
        return false
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Preview & Clean")
                .font(.title2)

            if let scan = model.lastScan {
                let selected = scan.items.filter { model.selectedItemIDs.contains($0.id) }

                let actionable = selected.filter { !isExcluded($0.path, excludedPaths: model.excludedPaths) }
                let excludedCount = selected.count - actionable.count

                let rows = actionable.sorted(by: { $0.size > $1.size })
                let totalSize = sumSizes(actionable)
                let containsRisky = actionable.contains { $0.safetyLevel == .risky }

                Text("\(actionable.count) items • \(formatBytes(totalSize))")
                    .foregroundStyle(.secondary)
                    .monospacedDigit()

                Toggle("Dry run (no files will be deleted)", isOn: $dryRun)
                Toggle("Allow risky items (unsafe)", isOn: $allowRisky)

                if excludedCount > 0 {
                    OCBanner(
                        title: "Excluded paths",
                        message: "\(excludedCount) selected item(s) are under excluded paths in Settings and will be skipped.",
                        systemImage: "hand.raised.fill",
                        tint: .gray
                    )
                }

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
                        if dryRun {
                            model.cleanSelected(dryRun: true, unsafe: allowRisky)
                            dismiss()
                        } else {
                            showConfirmClean = true
                        }
                    }
                    .confirmationDialog(
                        "Confirm Clean",
                        isPresented: $showConfirmClean,
                        titleVisibility: .visible
                    ) {
                        Button("Move \(actionable.count) items (\(formatBytes(totalSize))) to Trash", role: .destructive) {
                            model.cleanSelected(dryRun: false, unsafe: allowRisky)
                            dismiss()
                        }
                        Button("Cancel", role: .cancel) {}
                    } message: {
                        let riskyCount = actionable.filter { $0.safetyLevel == .risky }.count
                        if riskyCount > 0 && allowRisky {
                            Text("⚠️ This selection includes \(riskyCount) risky item(s). Move \(actionable.count) items (\(formatBytes(totalSize))) to Trash? This action can be undone with ⌘Z.")
                        } else if containsRisky {
                            Text("Move \(actionable.count) items (\(formatBytes(totalSize))) to Trash? This selection includes risky items.")
                        } else {
                            Text("Move \(actionable.count) items (\(formatBytes(totalSize))) to Trash? This action can be undone with ⌘Z.")
                        }
                    }
                    .keyboardShortcut(.defaultAction)
                    .disabled(!model.isOnline || actionable.isEmpty || (containsRisky && !allowRisky) || model.activity != .idle)
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
