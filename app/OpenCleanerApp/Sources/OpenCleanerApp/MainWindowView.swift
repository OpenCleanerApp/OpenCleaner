import OpenCleanerClient
import SwiftUI

struct MainWindowView: View {
    @EnvironmentObject var model: AppModel

    enum Section: String, CaseIterable, Identifiable {
        case dashboard
        case results
        case audit
        case settings

        var id: String { rawValue }

        var title: String {
            switch self {
            case .dashboard: return "Dashboard"
            case .results: return "Results"
            case .audit: return "Audit Log"
            case .settings: return "Settings"
            }
        }

        var systemImage: String {
            switch self {
            case .dashboard: return "gauge"
            case .results: return "list.bullet.rectangle"
            case .audit: return "doc.text.magnifyingglass"
            case .settings: return "gearshape"
            }
        }
    }

    @State private var section: Section? = .dashboard

    var body: some View {
        NavigationSplitView {
            List(Section.allCases, selection: $section) { s in
                Label(s.title, systemImage: s.systemImage)
                    .tag(s as Section?)
            }
            .navigationTitle("OpenCleaner")
        } detail: {
            switch section ?? .dashboard {
            case .dashboard:
                DashboardView()
            case .results:
                ResultsView()
            case .audit:
                AuditView()
            case .settings:
                SettingsView()
            }
        }
        .toolbar {
            ToolbarItemGroup {
                Button("Refresh") { model.refreshStatus() }
                    .keyboardShortcut("r", modifiers: [.command])

                Button("Scan") { model.scan() }
                    .keyboardShortcut("s", modifiers: [.command])
                    .disabled(!model.isOnline || model.activity != .idle)

                Button("Preview & Clean") { model.showingCleanSheet = true }
                    .keyboardShortcut(.return, modifiers: [.command])
                    .disabled(!model.isOnline || model.selectedItemIDs.isEmpty || model.activity != .idle)
            }
        }
        .onAppear { model.refreshStatus() }
        .sheet(isPresented: $model.showingCleanSheet) {
            CleanSheetView()
                .environmentObject(model)
        }
    }
}

private struct DashboardView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                Circle()
                    .fill(model.isOnline ? Color.green : Color.red)
                    .frame(width: 10, height: 10)
                Text(model.statusLine)
                    .font(.headline)
                Spacer()
            }

            if case .offline(let message) = model.connectionState {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Daemon offline")
                        .font(.title3)
                    Text("Socket: \(model.socketPath)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .textSelection(.enabled)
                    Text(message)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .padding(12)
                .background(.thinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 12))
            }

            if let scan = model.lastScan {
                VStack(alignment: .leading, spacing: 10) {
                    Text("Last scan")
                        .font(.title3)

                    HStack {
                        VStack(alignment: .leading) {
                            Text("Total")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            Text(formatBytes(scan.totalSize))
                                .font(.title2)
                        }

                        Spacer()

                        VStack(alignment: .leading) {
                            Text("Items")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            Text("\(scan.items.count)")
                                .font(.title2)
                        }
                    }

                    HStack(spacing: 16) {
                        ForEach(openCleanerCategories, id: \.rawValue) { cat in
                            VStack(alignment: .leading, spacing: 2) {
                                Label(cat.title, systemImage: cat.systemImageName)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                Text(formatBytes(scan.categorizedSize[cat] ?? 0))
                                    .font(.headline)
                            }
                        }
                    }
                }
                .padding(12)
                .background(.thinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 12))
            } else {
                Text("Run a scan to see results.")
                    .foregroundStyle(.secondary)
            }

            if let evt = model.progressEvent, model.activity != .idle {
                VStack(alignment: .leading, spacing: 8) {
                    Text(model.activity == .scanning ? "Scanning" : "Cleaning")
                        .font(.headline)
                    if let p = evt.progress {
                        ProgressView(value: p)
                    } else {
                        ProgressView()
                    }
                    if let msg = evt.message, !msg.isEmpty {
                        Text(msg)
                            .font(.caption)
                    }
                }
                .padding(12)
                .background(.thinMaterial)
                .clipShape(RoundedRectangle(cornerRadius: 12))
            }

            Spacer()
        }
        .padding(16)
    }
}

private struct ResultsView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        Group {
            if let scan = model.lastScan {
                resultsBody(scan: scan)
            } else {
                ContentUnavailableView {
                    Label("No scan results", systemImage: "magnifyingglass")
                } description: {
                    Text("Run a scan to populate results.")
                } actions: {
                    Button("Scan") { model.scan() }
                        .disabled(!model.isOnline || model.activity != .idle)
                }
            }
        }
        .navigationTitle("Results")
    }

    private func resultsBody(scan: ScanResult) -> some View {
        let grouped = Dictionary(grouping: scan.items, by: { $0.category })
        let selectedItems = scan.items.filter { model.selectedItemIDs.contains($0.id) }

        return VStack(spacing: 0) {
            HStack {
                Text("Selected: \(selectedItems.count) • \(formatBytes(sumSizes(selectedItems)))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button("Preview & Clean") { model.showingCleanSheet = true }
                    .disabled(!model.isOnline || model.selectedItemIDs.isEmpty || model.activity != .idle)
            }
            .padding(12)

            Divider()

            HStack(spacing: 0) {
                List(selection: $model.selectedItemIDs) {
                    ForEach(openCleanerCategories, id: \.rawValue) { cat in
                        let items = (grouped[cat] ?? []).sorted(by: { $0.size > $1.size })
                        if !items.isEmpty {
                            Section(cat.title) {
                                ForEach(items) { item in
                                    ScanItemRow(item: item)
                                        .tag(item.id)
                                        .onTapGesture { model.focusedItemID = item.id }
                                }
                            }
                        }
                    }
                }
                .frame(minWidth: 380)

                Divider()

                ResultDetailView(scan: scan)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .onChange(of: model.selectedItemIDs) { _, newValue in
            if newValue.count == 1 {
                model.focusedItemID = newValue.first
            } else {
                model.focusedItemID = nil
            }
        }
    }
}

private struct ScanItemRow: View {
    let item: ScanItem

    var body: some View {
        HStack(spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(item.name)
                    SafetyBadge(level: item.safetyLevel)
                }
                Text(item.path)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer()
            Text(formatBytes(item.size))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }
}

private struct SafetyBadge: View {
    let level: SafetyLevel

    var body: some View {
        Text(level.title)
            .font(.caption2)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(level.tint.opacity(0.18))
            .foregroundStyle(level.tint)
            .clipShape(Capsule())
    }
}

private struct ResultDetailView: View {
    @EnvironmentObject var model: AppModel
    let scan: ScanResult

    var body: some View {
        let selected = scan.items.filter { model.selectedItemIDs.contains($0.id) }

        if let focused = model.focusedItemID, let item = scan.items.first(where: { $0.id == focused }) {
            VStack(alignment: .leading, spacing: 10) {
                Text(item.name)
                    .font(.title2)

                HStack(spacing: 10) {
                    SafetyBadge(level: item.safetyLevel)
                    Text(formatBytes(item.size))
                        .foregroundStyle(.secondary)
                }

                Text(item.path)
                    .font(.callout)
                    .textSelection(.enabled)

                if let note = item.safetyNote, !note.isEmpty {
                    Text(note)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }

                if let desc = item.description, !desc.isEmpty {
                    Text(desc)
                        .font(.callout)
                }

                Spacer()
            }
            .padding(16)
        } else if !selected.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                Text("\(selected.count) items selected")
                    .font(.title2)
                Text("Total: \(formatBytes(sumSizes(selected)))")
                    .foregroundStyle(.secondary)
                Spacer()
            }
            .padding(16)
        } else {
            ContentUnavailableView {
                Label("Select an item", systemImage: "cursorarrow.click")
            }
        }
    }
}

private struct AuditView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Audit Log")
                .font(.title2)

            if let clean = model.lastCleanResult {
                Text(clean.auditLogPath)
                    .font(.callout)
                    .textSelection(.enabled)

                Button("Open Audit Log") { model.openAuditLog() }

                if let failed = clean.failedItems.first {
                    Text("Some items failed (e.g. \(failed)).")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            } else {
                Text("Run a clean to generate an audit log.")
                    .foregroundStyle(.secondary)
            }

            Spacer()
        }
        .padding(16)
        .navigationTitle("Audit")
    }
}
