import OpenCleanerClient
import SwiftUI

struct CleanSheetView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss

    @State private var dryRun: Bool = true
    @State private var allowRisky: Bool = false

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Preview & Clean")
                .font(.title2)

            if let scan = model.lastScan {
                let selected = scan.items.filter { model.selectedItemIDs.contains($0.id) }
                let totalSize = sumSizes(selected)
                let containsRisky = selected.contains { $0.safetyLevel == .risky }

                HStack {
                    Text("\(selected.count) items • \(formatBytes(totalSize))")
                        .foregroundStyle(.secondary)
                    Spacer()
                }

                Toggle("Dry run (no files will be deleted)", isOn: $dryRun)
                Toggle("Allow risky items (unsafe)", isOn: $allowRisky)

                if containsRisky && !allowRisky {
                    Text("Risky items are selected. Enable ‘Allow risky items (unsafe)’ to proceed.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                List(selected.sorted(by: { $0.size > $1.size })) { item in
                    HStack {
                        Text(item.name)
                        Spacer()
                        Text(formatBytes(item.size))
                            .foregroundStyle(.secondary)
                    }
                }

                HStack {
                    Button("Cancel") { dismiss() }

                    Spacer()

                    Button(dryRun ? "Run Dry-Run" : "Clean Now") {
                        model.cleanSelected(dryRun: dryRun, unsafe: allowRisky)
                        dismiss()
                    }
                    .disabled(!model.isOnline || selected.isEmpty || containsRisky && !allowRisky || model.activity != .idle)
                    .buttonStyle(.borderedProminent)
                }
            } else {
                Text("No scan results to clean.")
                    .foregroundStyle(.secondary)

                HStack {
                    Spacer()
                    Button("Close") { dismiss() }
                }
            }
        }
        .padding(16)
        .frame(minWidth: 560, minHeight: 520)
    }
}
