import AppKit
import OpenCleanerClient
import SwiftUI

struct MenuBarPopoverView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.openWindow) private var openWindow
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            header

            if case .offline(let message) = model.connectionState {
                DaemonOfflineInlineView(socketPath: model.socketPath, message: message)
            }

            if let evt = model.progressEvent, model.activity != .idle {
                ProgressInlineView(event: evt)
            }

            if let scan = model.lastScan {
                ScanSummaryView(scan: scan)
            }

            if let clean = model.lastCleanResult {
                CleanSummaryView(clean: clean)
            }

            if let err = model.lastError, !err.isEmpty {
                Text(err)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            }

            Divider()

            HStack {
                Button("Scan") { model.scan() }
                    .disabled(!model.isOnline || model.activity != .idle)

                Button("Preview & Clean") { model.showingCleanSheet = true }
                    .disabled(!model.isOnline || model.selectedItemIDs.isEmpty || model.activity != .idle)

                Spacer()
            }

            HStack {
                Button("Open Full View") { openWindow(id: "main") }

                Button("Settings") { openSettings() }

                Spacer()

                Button("Quit") { NSApp.terminate(nil) }
            }
        }
        .padding(12)
        .frame(width: 380)
        .onAppear { model.refreshStatus() }
        .sheet(isPresented: $model.showingCleanSheet) {
            CleanSheetView()
                .environmentObject(model)
        }
    }

    private var header: some View {
        HStack(spacing: 8) {
            Circle()
                .fill(model.isOnline ? Color.green : Color.red)
                .frame(width: 8, height: 8)
            Text(model.statusLine)
                .font(.headline)
            Spacer()
            Button("Refresh") { model.refreshStatus() }
        }
    }
}

private struct DaemonOfflineInlineView: View {
    let socketPath: String
    let message: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Daemon offline")
                .font(.subheadline)
            Text("Socket: \(socketPath)")
                .font(.caption)
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
            Text(message)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(2)
        }
        .padding(10)
        .background(.thinMaterial)
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }
}

private struct ProgressInlineView: View {
    let event: ProgressEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let p = event.progress {
                ProgressView(value: p)
            } else {
                ProgressView()
            }

            HStack {
                Text(event.type)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                if let p = event.progress {
                    Text("\(Int(p * 100))%")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }

            if let msg = event.message, !msg.isEmpty {
                Text(msg)
                    .font(.caption)
            }

            if let cur = event.current, !cur.isEmpty {
                Text(cur)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
    }
}

private struct ScanSummaryView: View {
    let scan: ScanResult

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Last scan")
                    .font(.subheadline)
                Spacer()
                Text("\(scan.items.count) items")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            HStack {
                Text("Total")
                Spacer()
                Text(formatBytes(scan.totalSize))
                    .foregroundStyle(.secondary)
            }
            .font(.caption)

            HStack(spacing: 12) {
                ForEach(openCleanerCategories, id: \.rawValue) { cat in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(cat.title)
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                        Text(formatBytes(scan.categorizedSize[cat] ?? 0))
                            .font(.caption)
                    }
                }
            }
        }
        .padding(10)
        .background(.thinMaterial)
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }
}

private struct CleanSummaryView: View {
    let clean: CleanResult

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(clean.dryRun == true ? "Dry-run complete" : "Clean complete")
                    .font(.subheadline)
                Spacer()
                Text("\(clean.cleanedCount) items")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            HStack {
                Text("Freed")
                Spacer()
                Text(formatBytes(clean.cleanedSize))
                    .foregroundStyle(.secondary)
            }
            .font(.caption)

            Text("Audit: \(clean.auditLogPath)")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
        .padding(10)
        .background(.thinMaterial)
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }
}
