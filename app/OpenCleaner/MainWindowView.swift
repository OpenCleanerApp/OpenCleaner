import AppKit
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
            ToolbarItemGroup(placement: .automatic) {
                Button {
                    model.refreshStatus()
                } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .keyboardShortcut("r", modifiers: [.command])
            }

            ToolbarItemGroup(placement: .primaryAction) {
                Button {
                    model.scan()
                } label: {
                    Label("Scan", systemImage: "magnifyingglass")
                }
                .keyboardShortcut("s", modifiers: [.command])
                .disabled(!model.isOnline || model.activity != .idle)

                Button {
                    model.showingCleanSheet = true
                } label: {
                    Label("Preview & Clean", systemImage: "trash")
                }
                .keyboardShortcut(.return, modifiers: [.command])
                .disabled(!model.isOnline || model.selectedItemIDs.isEmpty || model.activity != .idle)
            }
        }
        .onAppear { model.refreshStatus() }
        .sheet(isPresented: $model.showingCleanSheet) {
            CleanSheetView()
                .environmentObject(model)
        }
        .overlay(alignment: .bottom) {
            CompletionBannerView()
                .environmentObject(model)
        }
    }
}

private struct DashboardView: View {
    @EnvironmentObject var model: AppModel

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
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                DaemonStatusHeader(statusColor: statusColor, title: model.statusLine, subtitle: model.socketPath)

                if case .offline(let message) = model.connectionState {
                    OCBanner(
                        title: "Daemon offline",
                        message: message,
                        systemImage: "bolt.slash.fill",
                        tint: .orange
                    )
                }

                if model.scanWasInterrupted {
                    OCBanner(
                        title: "Scan interrupted",
                        message: "\(model.lastScan?.items.count ?? 0) items found so far. Resume to continue scanning.",
                        systemImage: "exclamationmark.triangle",
                        tint: .yellow
                    )
                    HStack {
                        Button("Restart Scan") {
                            model.scan()
                        }
                        .buttonStyle(.borderedProminent)
                        .controlSize(.small)
                        .disabled(!model.isOnline || model.activity != .idle)
                        Spacer()
                    }
                }

                if let scan = model.lastScan {
                    OCCard(title: "Last scan", systemImage: "magnifyingglass") {
                        VStack(alignment: .leading, spacing: 12) {
                            HStack(alignment: .firstTextBaseline) {
                                MetricView(title: "Total", value: formatBytes(scan.totalSize))
                                Spacer()
                                MetricView(title: "Items", value: "\(scan.items.count)")
                            }

                            Divider()

                            HStack(spacing: 18) {
                                ForEach(openCleanerCategories, id: \.rawValue) { cat in
                                    VStack(alignment: .leading, spacing: 4) {
                                        Label(cat.title, systemImage: cat.systemImageName)
                                            .font(.caption)
                                            .foregroundStyle(.secondary)
                                        Text(formatBytes(scan.categorizedSize[cat] ?? 0))
                                            .font(.headline)
                                            .monospacedDigit()
                                    }
                                }
                            }
                        }
                    }
                } else {
                    ContentUnavailableView {
                        Label("No scan results", systemImage: "magnifyingglass")
                    } description: {
                        Text("Run a scan to see results.")
                    }
                }

                if let evt = model.progressEvent, model.activity != .idle {
                    OCCard(
                        title: model.activity == .scanning ? "Scanning" : "Cleaning",
                        systemImage: model.activity == .scanning ? "magnifyingglass" : "trash"
                    ) {
                        VStack(alignment: .leading, spacing: 8) {
                            if let p = evt.progress {
                                ProgressView(value: p)
                            } else {
                                ProgressView()
                            }

                            if let msg = evt.message, !msg.isEmpty {
                                Text(msg)
                                    .font(.callout)
                            }

                            if let cur = evt.current, !cur.isEmpty {
                                Text(cur)
                                    .font(.caption.monospaced())
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                            }
                        }
                    }
                }

                Spacer(minLength: 0)
            }
            .padding(OCLayout.pagePadding)
        }
        .navigationTitle("Dashboard")
    }
}

private struct ResultsView: View {
    @EnvironmentObject var model: AppModel
    @State private var query: String = ""
    @State private var safetyFilter: SafetyFilter = .all

    private enum SafetyFilter: String, CaseIterable, Identifiable {
        case all = "All"
        case safe = "Safe"
        case risky = "Risky"

        var id: String { rawValue }

        func allows(_ item: ScanItem) -> Bool {
            switch self {
            case .all:
                return true
            case .safe:
                return item.safetyLevel != .risky
            case .risky:
                return item.safetyLevel == .risky
            }
        }
    }

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
                    Button {
                        model.scan()
                    } label: {
                        Label("Scan", systemImage: "magnifyingglass")
                    }
                    .disabled(!model.isOnline || model.activity != .idle)
                }
            }
        }
        .navigationTitle("Results")
    }

    private func resultsBody(scan: ScanResult) -> some View {
        let checkedItems = scan.items.filter { model.selectedItemIDs.contains($0.id) }

        let queryFiltered = scan.items.filter {
            query.isEmpty
                || $0.name.localizedCaseInsensitiveContains(query)
                || $0.path.localizedCaseInsensitiveContains(query)
        }

        let filtered = queryFiltered.filter { safetyFilter.allows($0) }
        let visibleIDs = Set(filtered.map(\.id))
        let grouped = Dictionary(grouping: filtered, by: { $0.category })

        return HStack(spacing: 0) {
            VStack(spacing: 0) {
                HStack {
                    Picker("Filter", selection: $safetyFilter) {
                        ForEach(SafetyFilter.allCases) { f in
                            Text(f.rawValue)
                                .tag(f)
                        }
                    }
                    .pickerStyle(.segmented)
                    .controlSize(.small)
                    .frame(maxWidth: 260)

                    Spacer(minLength: 0)

                    Text("\(checkedItems.count) selected • \(formatBytes(sumSizes(checkedItems)))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .monospacedDigit()
                }
                .padding(12)

                Divider()

                List(selection: $model.focusedItemID) {
                    ForEach(openCleanerCategories, id: \.rawValue) { cat in
                        let items = (grouped[cat] ?? []).sorted(by: { $0.size > $1.size })
                        if !items.isEmpty {
                            Section(cat.title) {
                                ForEach(items) { item in
                                    ScanItemRow(
                                        item: item,
                                        isChecked: Binding(
                                            get: { model.selectedItemIDs.contains(item.id) },
                                            set: { checked in
                                                if checked {
                                                    model.selectedItemIDs.insert(item.id)
                                                } else {
                                                    model.selectedItemIDs.remove(item.id)
                                                }
                                            }
                                        )
                                    )
                                    .tag(item.id)
                                }
                            }
                        }
                    }
                }
                .searchable(text: $query, placement: .toolbar)
                .onKeyPress(.space) {
                    if let focused = model.focusedItemID {
                        if model.selectedItemIDs.contains(focused) {
                            model.selectedItemIDs.remove(focused)
                        } else {
                            model.selectedItemIDs.insert(focused)
                        }
                        return .handled
                    }
                    return .ignored
                }
            }
            .frame(minWidth: 520)

            Divider()

            ResultDetailView(scan: scan)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .onChange(of: model.selectedItemIDs) { _, newValue in
            if model.focusedItemID == nil, newValue.count == 1 {
                model.focusedItemID = newValue.first
            }
        }
        .onChange(of: query) { _, _ in
            if let focused = model.focusedItemID, !visibleIDs.contains(focused) {
                model.focusedItemID = nil
            }
        }
        .onChange(of: safetyFilter) { _, _ in
            if let focused = model.focusedItemID, !visibleIDs.contains(focused) {
                model.focusedItemID = nil
            }
        }
    }
}

private struct ScanItemRow: View {
    let item: ScanItem
    @Binding var isChecked: Bool

    var body: some View {
        HStack(spacing: 8) {
            Toggle(isOn: $isChecked) {
                EmptyView()
            }
            .toggleStyle(.checkbox)
            .labelsHidden()
            .controlSize(.small)
            .accessibilityLabel(Text("Select \(item.name)"))
            .accessibilityHint(Text("Adds or removes this item from the clean selection"))

            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 8) {
                    Text(item.name)
                        .lineLimit(1)
                    SafetyBadge(level: item.safetyLevel)
                }

                Text(item.path)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .help(item.path)
            }

            Spacer(minLength: 0)

            Text(formatBytes(item.size))
                .font(.caption)
                .foregroundStyle(.secondary)
                .monospacedDigit()
        }
    }
}

private struct ResultDetailView: View {
    @EnvironmentObject var model: AppModel
    let scan: ScanResult

    var body: some View {
        let selected = scan.items.filter { model.selectedItemIDs.contains($0.id) }

        if let focused = model.focusedItemID, let item = scan.items.first(where: { $0.id == focused }) {
            VStack(alignment: .leading, spacing: 12) {
                Text(item.name)
                    .font(.title2)

                HStack(spacing: 10) {
                    SafetyBadge(level: item.safetyLevel)
                    Text(formatBytes(item.size))
                        .foregroundStyle(.secondary)
                        .monospacedDigit()
                }

                PathRow(label: "Path", path: item.path, labelWidth: 44)

                if let note = item.safetyNote, !note.isEmpty {
                    Text(note)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }

                if let desc = item.description, !desc.isEmpty {
                    Text(desc)
                        .font(.callout)
                        .fixedSize(horizontal: false, vertical: true)
                }

                Spacer(minLength: 0)
            }
            .padding(OCLayout.pagePadding)
        } else if !selected.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                OCCard(title: "Selection", systemImage: "checkmark.circle") {
                    HStack {
                        MetricView(title: "Items", value: "\(selected.count)")
                        Spacer()
                        MetricView(title: "Total", value: formatBytes(sumSizes(selected)))
                    }
                }

                ContentUnavailableView {
                    Label("Select an item", systemImage: "cursorarrow.click")
                } description: {
                    Text("Pick a focused item to see details.")
                }

                Spacer(minLength: 0)
            }
            .padding(OCLayout.pagePadding)
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
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if let clean = model.lastCleanResult {
                    OCCard(title: "Last clean", systemImage: "checkmark.circle") {
                        VStack(alignment: .leading, spacing: 10) {
                            HStack {
                                MetricView(title: "Freed", value: formatBytes(clean.cleanedSize))
                                Spacer()
                                MetricView(title: "Items", value: "\(clean.cleanedCount)")
                            }

                            if clean.dryRun == true {
                                Text("Dry-run")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }

                    OCCard(title: "Audit log", systemImage: "doc.text") {
                        VStack(alignment: .leading, spacing: 10) {
                            PathRow(label: "Path", path: clean.auditLogPath, labelWidth: 44)

                            HStack {
                                Button("Open") { model.openAuditLog() }
                                Button("Reveal") { model.revealAuditLogInFinder() }
                                Spacer()
                            }
                            .controlSize(.small)
                        }
                    }

                    if !clean.failedItems.isEmpty {
                        OCCard(title: "Failed items", systemImage: "exclamationmark.triangle.fill") {
                            VStack(alignment: .leading, spacing: 6) {
                                ForEach(clean.failedItems.prefix(8), id: \.self) { item in
                                    Text(item)
                                        .font(.caption)
                                        .textSelection(.enabled)
                                }

                                if clean.failedItems.count > 8 {
                                    Text("…and \(clean.failedItems.count - 8) more")
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                        }
                    }
                } else {
                    ContentUnavailableView {
                        Label("No audit log yet", systemImage: "doc.text.magnifyingglass")
                    } description: {
                        Text("Run a clean to generate an audit log.")
                    }
                }

                Spacer(minLength: 0)
            }
            .padding(OCLayout.pagePadding)
        }
        .navigationTitle("Audit")
    }
}
