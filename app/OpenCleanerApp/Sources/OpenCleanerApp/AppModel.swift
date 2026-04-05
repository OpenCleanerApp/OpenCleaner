import AppKit
import Foundation
import OpenCleanerClient

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

    @Published var socketPath: String = OpenCleanerDefaults.defaultSocketPath()

    @Published var connectionState: ConnectionState = .unknown
    @Published var activity: Activity = .idle

    @Published var lastError: String?
    @Published var lastScan: ScanResult?
    @Published var progressEvent: ProgressEvent?

    @Published var selectedItemIDs: Set<String> = []
    @Published var focusedItemID: String?

    @Published var lastCleanResult: CleanResult?
    @Published var showingCleanSheet: Bool = false

    private var progressTask: Task<Void, Never>?
    private var statusTask: Task<Void, Never>?
    private var actionTask: Task<Void, Never>?

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
            } catch {
                lastError = String(describing: error)
            }
        }
    }

    func cleanSelected(dryRun: Bool, unsafe: Bool) {
        guard activity == .idle else { return }
        guard let scan = lastScan else { return }

        let ids = scan.items
            .filter { selectedItemIDs.contains($0.id) }
            .map(\.id)

        guard !ids.isEmpty else {
            lastError = "No valid items selected."
            selectedItemIDs = []
            focusedItemID = nil
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
                let req = CleanRequest(itemIDs: ids, strategy: .trash, dryRun: dryRun, unsafe: unsafe)
                let res = try await client.clean(request: req)
                lastCleanResult = res
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

    func resetSocketPathToDefault() {
        socketPath = OpenCleanerDefaults.defaultSocketPath()
    }

    private func makeClient() -> OpenCleanerDaemonClient {
        OpenCleanerDaemonClient(socketPath: socketPath)
    }

    private func applyDefaultSelection(from scan: ScanResult) {
        let safeDefault = scan.items.filter { $0.safetyLevel != .risky }.map(\.id)
        selectedItemIDs = Set(safeDefault)
        focusedItemID = scan.items.first(where: { selectedItemIDs.contains($0.id) })?.id
    }

    private func startProgressConsumer(expectedStartType: String) {
        stopProgressConsumer()
        progressEvent = nil

        let client = makeClient()
        progressTask = Task.detached { [weak self] in
            var sawStart = false
            do {
                for try await evt in client.progressStream(reconnect: .default) {
                    if evt.type == expectedStartType { sawStart = true }
                    await MainActor.run { self?.progressEvent = evt }
                    if sawStart && evt.type == "complete" { break }
                }
            } catch {
                await MainActor.run { self?.lastError = String(describing: error) }
            }
        }
    }

    private func stopProgressConsumer() {
        progressTask?.cancel()
        progressTask = nil
    }
}
