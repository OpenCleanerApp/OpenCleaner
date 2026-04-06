import AppKit
import Foundation
import OpenCleanerClient
import SwiftUI

@MainActor
final class AppModel: ObservableObject {
    enum Activity {
        case idle
        case scanning
        case cleaning
    }

    enum ConnectionState {
        case unknown
        case online(DaemonStatus)
        case offline(message: String)
    }

    private enum DefaultsKey {
        static let socketPath = "socketPathOverride"
        static let excludedPaths = "excludedPaths"
    }

    @Published var socketPath: String = OpenCleanerDefaults.defaultSocketPath() {
        didSet {
            UserDefaults.standard.set(socketPath, forKey: DefaultsKey.socketPath)
        }
    }

    @Published var excludedPaths: [String] = [] {
        didSet {
            UserDefaults.standard.set(excludedPaths, forKey: DefaultsKey.excludedPaths)
        }
    }

    @Published var connectionState: ConnectionState = .unknown
    @Published var activity: Activity = .idle

    @Published var lastError: String?
    @Published var lastScan: ScanResult?
    @Published var progressEvent: ProgressEvent?

    @Published var selectedItemIDs: Set<String> = []
    @Published var focusedItemID: String?

    @Published var lastCleanResult: CleanResult?
    @Published var showingCleanSheet: Bool = false

    // Onboarding
    @Published var hasFullDiskAccess: Bool = PermissionService.hasFullDiskAccess()
    @AppStorage("hasCompletedOnboarding") var hasCompletedOnboarding: Bool = false

    // Undo
    @Published var undoResult: UndoResult?
    @Published var showingCompletionBanner: Bool = false

    // Scan resume
    @Published var scanWasInterrupted: Bool = false

    private var progressTask: Task<Void, Never>?
    private var statusTask: Task<Void, Never>?
    private var actionTask: Task<Void, Never>?

    init() {
        if let savedSocket = UserDefaults.standard.string(forKey: DefaultsKey.socketPath), !savedSocket.isEmpty {
            socketPath = savedSocket
        }
        if let savedExcluded = UserDefaults.standard.array(forKey: DefaultsKey.excludedPaths) as? [String] {
            excludedPaths = savedExcluded
        }
    }

    deinit {
        progressTask?.cancel()
        statusTask?.cancel()
        actionTask?.cancel()
    }

    var isOnline: Bool {
        if case .online = connectionState { return true }
        return false
    }

    var daemonStatus: DaemonStatus? {
        if case .online(let st) = connectionState { return st }
        return nil
    }

    var statusLine: String {
        switch connectionState {
        case .unknown:
            return "Daemon: unknown"
        case .online(let st):
            return st.ok ? "Daemon online (\(st.version))" : "Daemon not OK"
        case .offline:
            return "Daemon offline"
        }
    }

    func refreshStatus() {
        statusTask?.cancel()
        statusTask = Task {
            do {
                let st = try await makeClient().status()
                connectionState = .online(st)
                lastError = nil
            } catch {
                connectionState = .offline(message: String(describing: error))
                lastError = String(describing: error)
            }
        }
    }

    func scan() {
        guard activity == .idle else { return }

        lastError = nil
        lastCleanResult = nil
        scanWasInterrupted = false
        activity = .scanning

        startProgressConsumer(expectedStartType: "scanning")
        let client = makeClient()

        actionTask?.cancel()
        actionTask = Task {
            defer {
                stopProgressConsumer()
                activity = .idle
                progressEvent = nil
            }

            do {
                let res = try await client.scan()
                lastScan = res
                applyDefaultSelection(from: res)
            } catch is CancellationError {
                scanWasInterrupted = true
            } catch {
                lastError = String(describing: error)
            }
        }
    }

    func cleanSelected(dryRun: Bool, unsafe: Bool) {
        guard activity == .idle else { return }
        guard let scan = lastScan else { return }

        let selected = scan.items.filter { selectedItemIDs.contains($0.id) }
        let ids = selected
            .filter { !isExcludedPath($0.path) }
            .map(\.id)

        guard !ids.isEmpty else {
            if selected.isEmpty {
                lastError = "No valid items selected."
                selectedItemIDs = []
                focusedItemID = nil
            } else {
                lastError = "All selected items are excluded in Settings."
            }
            return
        }

        lastError = nil
        activity = .cleaning
        startProgressConsumer(expectedStartType: "cleaning")
        let client = makeClient()

        actionTask?.cancel()
        actionTask = Task {
            defer {
                stopProgressConsumer()
                activity = .idle
                progressEvent = nil
            }

            do {
                let req = CleanRequest(itemIDs: ids, excludePaths: excludedPaths.isEmpty ? nil : excludedPaths, strategy: .trash, dryRun: dryRun, unsafe: unsafe)
                let res = try await client.clean(request: req)
                lastCleanResult = res
                if !(dryRun) {
                    showingCompletionBanner = true
                }
            } catch {
                lastError = String(describing: error)
            }
        }
    }

    func openAuditLog() {
        guard let p = lastCleanResult?.auditLogPath, !p.isEmpty else { return }
        NSWorkspace.shared.open(URL(fileURLWithPath: p))
    }

    func revealAuditLogInFinder() {
        guard let p = lastCleanResult?.auditLogPath, !p.isEmpty else { return }
        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: p)])
    }

    func checkPermission() {
        hasFullDiskAccess = PermissionService.hasFullDiskAccess()
    }

    func cancelScan() {
        actionTask?.cancel()
        stopProgressConsumer()
        scanWasInterrupted = true
    }

    func undoLastClean() {
        guard activity == .idle else { return }
        activity = .cleaning
        lastError = nil
        let client = makeClient()

        actionTask = Task {
            defer { activity = .idle }
            do {
                undoResult = try await client.undo()
                lastCleanResult = nil
                showingCompletionBanner = false
            } catch {
                lastError = String(describing: error)
            }
        }
    }

    func selectAll() {
        guard let scan = lastScan else { return }
        selectedItemIDs = Set(scan.items.map(\.id))
    }

    func deselectAll() {
        selectedItemIDs = []
        focusedItemID = nil
    }

    func resetSocketPathToDefault() {
        socketPath = OpenCleanerDefaults.defaultSocketPath()
    }

    private func makeClient() -> OpenCleanerDaemonClient {
        OpenCleanerDaemonClient(socketPath: socketPath)
    }

    private func applyDefaultSelection(from scan: ScanResult) {
        let safeDefault = scan.items
            .filter { $0.safetyLevel != .risky && !isExcludedPath($0.path) }
            .map(\.id)
        selectedItemIDs = Set(safeDefault)
        focusedItemID = scan.items.first(where: { selectedItemIDs.contains($0.id) })?.id
    }

    private func isExcludedPath(_ path: String) -> Bool {
        guard !excludedPaths.isEmpty else { return false }

        let p = (path as NSString).standardizingPath
        for raw in excludedPaths {
            let ex = ((raw as NSString).expandingTildeInPath as NSString).standardizingPath
            if p == ex || p.hasPrefix(ex + "/") {
                return true
            }
        }
        return false
    }

    private func startProgressConsumer(expectedStartType: String) {
        stopProgressConsumer()
        progressEvent = nil

        let client = makeClient()
        progressTask = Task { [weak self] in
            var sawStart = false
            do {
                for try await evt in client.progressStream(reconnect: .default) {
                    if evt.type == expectedStartType { sawStart = true }
                    self?.progressEvent = evt
                    if sawStart && evt.type == "complete" { break }
                }
            } catch {
                self?.lastError = String(describing: error)
            }
        }
    }

    private func stopProgressConsumer() {
        progressTask?.cancel()
        progressTask = nil
    }
}
