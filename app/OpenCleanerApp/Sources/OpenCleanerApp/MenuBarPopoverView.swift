import AppKit
import OpenCleanerClient
import SwiftUI

struct MenuBarPopoverView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.openWindow) private var openWindow
    @Environment(\.openSettings) private var openSettings

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

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            DaemonStatusHeader(statusColor: statusColor, title: model.statusLine, subtitle: model.socketPath) {
                Button {
                    model.refreshStatus()
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .help("Refresh")
                .buttonStyle(.borderless)
            }

            if case .offline(let message) = model.connectionState {
                OCCard(title: "Daemon offline", systemImage: "bolt.slash.fill") {
                    VStack(alignment: .leading, spacing: 8) {
                        PathRow(label: "Socket", path: model.socketPath, labelWidth: 52)
                        Text(message)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(3)
                            .textSelection(.enabled)
                    }
                }
            }

            if let evt = model.progressEvent, model.activity != .idle {
                OCCard(
                    title: model.activity == .scanning ? "Scanning" : "Cleaning",
                    systemImage: model.activity == .scanning ? "magnifyingglass" : "trash"
                ) {
                    ProgressInlineView(event: evt)
                }
            }

            if let scan = model.lastScan {
                OCCard(title: "Last scan", systemImage: "clock") {
                    ScanSummaryView(scan: scan)
                }
            }

            if let clean = model.lastCleanResult {
                OCCard(
                    title: clean.dryRun == true ? "Dry-run complete" : "Clean complete",
                    systemImage: "checkmark.circle"
                ) {
                    CleanSummaryView(clean: clean)
                }
            }

            if let err = model.lastError, !err.isEmpty {
                OCBanner(title: "Error", message: err, systemImage: "xmark.octagon.fill", tint: .red)
            }

            Divider()

            HStack {
                Button("Scan") { model.scan() }
                    .buttonStyle(.bordered)
                    .disabled(!model.isOnline || model.activity != .idle)

                Button("Preview & Clean") {
                    openWindow(id: "main")
                    Task { @MainActor in
                        model.showingCleanSheet = true
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(!model.isOnline || model.selectedItemIDs.isEmpty || model.activity != .idle)

                Spacer()
            }

            HStack {
                Button("Open Full View") { openWindow(id: "main") }
                Button("Settings") { openSettings() }

                Spacer()

                Button("Quit") { NSApp.terminate(nil) }
            }
            .controlSize(.small)
        }
        .padding(12)
        .frame(width: 360)
        .onAppear { model.refreshStatus() }
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
                        .monospacedDigit()
                }
            }

            if let msg = event.message, !msg.isEmpty {
                Text(msg)
                    .font(.caption)
            }

            if let cur = event.current, !cur.isEmpty {
                Text(cur)
                    .font(.caption2.monospaced())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
        }
    }
}

private struct ScanSummaryView: View {
    let scan: ScanResult

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("\(scan.items.count) items")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Text(formatBytes(scan.totalSize))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
            }

            HStack(spacing: 12) {
                ForEach(openCleanerCategories, id: \.rawValue) { cat in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(cat.title)
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                        Text(formatBytes(scan.categorizedSize[cat] ?? 0))
                            .font(.caption)
                            .monospacedDigit()
                    }
                }
            }
        }
    }
}

private struct CleanSummaryView: View {
    let clean: CleanResult

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Freed")
                    .foregroundStyle(.secondary)
                Spacer()
                Text(formatBytes(clean.cleanedSize))
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
            }
            .font(.caption)

            HStack {
                Text("Items")
                    .foregroundStyle(.secondary)
                Spacer()
                Text("\(clean.cleanedCount)")
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
            }
            .font(.caption)

            Text(clean.auditLogPath)
                .font(.caption2.monospaced())
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .textSelection(.enabled)
        }
    }
}
