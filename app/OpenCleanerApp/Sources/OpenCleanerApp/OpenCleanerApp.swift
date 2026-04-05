import OpenCleanerClient
import SwiftUI

@main
struct OpenCleanerApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        MenuBarExtra("OpenCleaner", systemImage: "sparkles") {
            ContentView()
                .environmentObject(model)
        }
        .menuBarExtraStyle(.window)
    }
}

@MainActor
final class AppModel: ObservableObject {
    @Published var statusLine: String = "Idle"
    @Published var lastError: String?
    @Published var lastScan: ScanResult?
    @Published var progressEvent: ProgressEvent?

    private let client: OpenCleanerClientProtocol
    private var progressTask: Task<Void, Never>?

    init(client: OpenCleanerClientProtocol = OpenCleanerDaemonClient()) {
        self.client = client
    }

    deinit {
        progressTask?.cancel()
    }

    func refreshStatus() {
        Task {
            do {
                let st = try await client.status()
                statusLine = st.ok ? "Daemon OK (\(st.version))" : "Daemon not OK"
                lastError = nil
            } catch {
                statusLine = "Daemon unavailable"
                lastError = String(describing: error)
            }
        }
    }

    func scan() {
        Task {
            do {
                statusLine = "Scanning…"
                startProgressConsumer(expectedStartType: "scanning")
                defer { stopProgressConsumer() }

                let res = try await client.scan()
                lastScan = res
                statusLine = "Found \(res.items.count) items"
                lastError = nil
                progressEvent = nil
            } catch {
                statusLine = "Scan failed"
                lastError = String(describing: error)
                stopProgressConsumer()
            }
        }
    }

    func cleanDryRunSelectedAll() {
        guard let ids = lastScan?.items.map(\.id), !ids.isEmpty else { return }
        Task {
            do {
                statusLine = "Dry-run cleaning…"
                startProgressConsumer(expectedStartType: "cleaning")
                defer { stopProgressConsumer() }

                _ = try await client.clean(request: CleanRequest(itemIDs: ids, strategy: .trash, dryRun: true))
                statusLine = "Dry-run complete"
                lastError = nil
                progressEvent = nil
            } catch {
                statusLine = "Clean failed"
                lastError = String(describing: error)
                stopProgressConsumer()
            }
        }
    }

    private func startProgressConsumer(expectedStartType: String) {
        stopProgressConsumer()
        progressEvent = nil

        progressTask = Task.detached { [weak self, client] in
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

private struct ContentView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(model.statusLine)
                .font(.headline)

            HStack(spacing: 8) {
                Button("Status") { model.refreshStatus() }
                Button("Scan") { model.scan() }
                Button("Dry-run Clean") { model.cleanDryRunSelectedAll() }
                    .disabled(model.lastScan?.items.isEmpty ?? true)
            }

            if let evt = model.progressEvent {
                VStack(alignment: .leading, spacing: 4) {
                    if let p = evt.progress {
                        ProgressView(value: p)
                    } else {
                        ProgressView()
                    }
                    HStack {
                        Text(evt.type)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Spacer()
                        if let p = evt.progress {
                            Text("\(Int(p * 100))%")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    if let msg = evt.message, !msg.isEmpty {
                        Text(msg).font(.caption)
                    }
                    if let cur = evt.current, !cur.isEmpty {
                        Text(cur)
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
            }

            if let err = model.lastError, !err.isEmpty {
                Text(err)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }

            Divider()

            if let scan = model.lastScan {
                HStack {
                    Text("Total:")
                    Text(ByteCountFormatter.string(fromByteCount: scan.totalSize, countStyle: .file))
                    Spacer()
                    Text("\(scan.items.count) items")
                }
                .font(.subheadline)

                List(scan.items.prefix(50)) { item in
                    VStack(alignment: .leading, spacing: 2) {
                        HStack {
                            Text(item.name)
                            Spacer()
                            Text(ByteCountFormatter.string(fromByteCount: item.size, countStyle: .file))
                                .foregroundStyle(.secondary)
                        }
                        Text(item.path)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                .frame(width: 420, height: 320)

                Text("Connected via daemon over Unix socket (Swift Network.framework).")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                Text("Start the daemon, then Scan.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(12)
    }
}
