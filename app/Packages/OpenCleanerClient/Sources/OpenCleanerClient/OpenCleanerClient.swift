import Foundation
import Network

public enum Category: String, Codable, Sendable {
    case system
    case developer
    case apps
}

public enum SafetyLevel: String, Codable, Sendable {
    case safe
    case moderate
    case risky
}

public struct ScanItem: Codable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let path: String
    public let size: Int64
    public let category: Category
    public let safetyLevel: SafetyLevel
    public let safetyNote: String?
    public let description: String?
    public let lastAccess: Date?

    public init(
        id: String,
        name: String,
        path: String,
        size: Int64,
        category: Category,
        safetyLevel: SafetyLevel,
        safetyNote: String? = nil,
        description: String? = nil,
        lastAccess: Date? = nil
    ) {
        self.id = id
        self.name = name
        self.path = path
        self.size = size
        self.category = category
        self.safetyLevel = safetyLevel
        self.safetyNote = safetyNote
        self.description = description
        self.lastAccess = lastAccess
    }

    private enum CodingKeys: String, CodingKey {
        case id
        case name
        case path
        case size
        case category
        case safetyLevel = "safety_level"
        case safetyNote = "safety_note"
        case description
        case lastAccess = "last_access"
    }
}

public struct ScanResult: Codable, Sendable {
    public let totalSize: Int64
    public let items: [ScanItem]
    public let scanDurationMs: Int64
    public let categorizedSize: [Category: Int64]

    public init(totalSize: Int64, items: [ScanItem], scanDurationMs: Int64, categorizedSize: [Category: Int64]) {
        self.totalSize = totalSize
        self.items = items
        self.scanDurationMs = scanDurationMs
        self.categorizedSize = categorizedSize
    }

    private enum CodingKeys: String, CodingKey {
        case totalSize = "total_size"
        case items
        case scanDurationMs = "scan_duration_ms"
        case categorizedSize = "categorized_size"
    }
}

public enum CleanStrategy: String, Codable, Sendable {
    case trash
    case delete
}

public struct CleanRequest: Codable, Sendable {
    public let itemIDs: [String]
    public let strategy: CleanStrategy
    public let dryRun: Bool?
    public let unsafe: Bool?
    public let force: Bool?

    public init(itemIDs: [String], strategy: CleanStrategy, dryRun: Bool? = nil, unsafe: Bool? = nil, force: Bool? = nil) {
        self.itemIDs = itemIDs
        self.strategy = strategy
        self.dryRun = dryRun
        self.unsafe = unsafe
        self.force = force
    }

    private enum CodingKeys: String, CodingKey {
        case itemIDs = "item_ids"
        case strategy
        case dryRun = "dry_run"
        case unsafe
        case force
    }
}

public struct CleanResult: Codable, Sendable {
    public let cleanedSize: Int64
    public let cleanedCount: Int
    public let failedItems: [String]
    public let auditLogPath: String
    public let dryRun: Bool?

    public init(cleanedSize: Int64, cleanedCount: Int, failedItems: [String], auditLogPath: String, dryRun: Bool? = nil) {
        self.cleanedSize = cleanedSize
        self.cleanedCount = cleanedCount
        self.failedItems = failedItems
        self.auditLogPath = auditLogPath
        self.dryRun = dryRun
    }

    private enum CodingKeys: String, CodingKey {
        case cleanedSize = "cleaned_size"
        case cleanedCount = "cleaned_count"
        case failedItems = "failed_items"
        case auditLogPath = "audit_log_path"
        case dryRun = "dry_run"
    }
}

public struct DaemonStatus: Codable, Sendable {
    public let ok: Bool
    public let version: String
    public let socketPath: String

    public init(ok: Bool, version: String, socketPath: String) {
        self.ok = ok
        self.version = version
        self.socketPath = socketPath
    }

    private enum CodingKeys: String, CodingKey {
        case ok
        case version
        case socketPath = "socket_path"
    }
}

public struct ProgressEvent: Codable, Sendable {
    public let type: String
    public let current: String?
    public let progress: Double?
    public let message: String?
    public let errors: [String]?

    public init(type: String, current: String? = nil, progress: Double? = nil, message: String? = nil, errors: [String]? = nil) {
        self.type = type
        self.current = current
        self.progress = progress
        self.message = message
        self.errors = errors
    }
}

public enum OpenCleanerError: Error, CustomStringConvertible, Sendable {
    case invalidUTF8
    case cliNotFound(String)
    case nonZeroExit(code: Int32, stderr: String)
    case httpError(statusCode: Int, body: String)

    public var description: String {
        switch self {
        case .invalidUTF8:
            return "invalid UTF-8 from opencleaner"
        case .cliNotFound(let hint):
            return "opencleaner CLI not found (hint: \(hint))"
        case .nonZeroExit(let code, let stderr):
            return "opencleaner failed (exit \(code)): \(stderr)"
        case .httpError(let status, let body):
            return "daemon HTTP \(status): \(body)"
        }
    }
}

public protocol OpenCleanerClientProtocol: Sendable {
    func status() async throws -> DaemonStatus
    func scan() async throws -> ScanResult
    func clean(request: CleanRequest) async throws -> CleanResult
    func progressStream() -> AsyncThrowingStream<ProgressEvent, Error>
}

/// Temporary dev client that shells out to the Go CLI.
///
/// This is intentionally a stopgap until the Swift side implements HTTP/JSON over a Unix domain socket.
public struct OpenCleanerCLIClient: OpenCleanerClientProtocol {
    public var socketPath: String
    public var executablePathHint: String

    public init(socketPath: String = "/tmp/opencleaner.sock", executablePathHint: String = "Build Go CLI and ensure 'opencleaner' is on PATH") {
        self.socketPath = socketPath
        self.executablePathHint = executablePathHint
    }

    public func status() async throws -> DaemonStatus {
        try await runJSON(["--socket=\(socketPath)", "--json", "status"], as: DaemonStatus.self)
    }

    public func scan() async throws -> ScanResult {
        try await runJSON(["--socket=\(socketPath)", "--json", "scan"], as: ScanResult.self)
    }

    public func clean(request: CleanRequest) async throws -> CleanResult {
        let ids = request.itemIDs.joined(separator: ",")
        var args = ["--socket=\(socketPath)", "--json", "clean", ids]
        if request.dryRun == true { args.append("--dry-run") }
        if request.unsafe == true { args.append("--unsafe") }
        if request.force == true { args.append("--force") }
        args.append("--strategy=\(request.strategy.rawValue)")
        return try await runJSON(args, as: CleanResult.self)
    }

    public func progressStream() -> AsyncThrowingStream<ProgressEvent, Error> {
        AsyncThrowingStream { $0.finish() }
    }

    private func runJSON<T: Decodable>(_ args: [String], as type: T.Type) async throws -> T {
        let data = try await runCLI(args)
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(T.self, from: data)
    }

    private func runCLI(_ args: [String]) async throws -> Data {
        guard let url = resolveExecutableURL() else {
            throw OpenCleanerError.cliNotFound(executablePathHint)
        }

        let proc = Process()
        proc.executableURL = url
        proc.arguments = args

        let out = Pipe()
        let err = Pipe()
        proc.standardOutput = out
        proc.standardError = err

        try proc.run()

        return try await withCheckedThrowingContinuation { cont in
            proc.terminationHandler = { p in
                let outData = out.fileHandleForReading.readDataToEndOfFile()
                let errData = err.fileHandleForReading.readDataToEndOfFile()
                let stderr = String(data: errData, encoding: .utf8) ?? ""

                if p.terminationStatus == 0 {
                    cont.resume(returning: outData)
                } else {
                    cont.resume(throwing: OpenCleanerError.nonZeroExit(code: p.terminationStatus, stderr: stderr.trimmingCharacters(in: .whitespacesAndNewlines)))
                }
            }
        }
    }

    private func resolveExecutableURL() -> URL? {
        // Prefer PATH lookup via /usr/bin/env.
        // If you want a fixed path, wrap this client and set proc.executableURL directly.
        let envURL = URL(fileURLWithPath: "/usr/bin/env")
        if FileManager.default.isExecutableFile(atPath: envURL.path) {
            // We'll run: env opencleaner ...
            // But Process requires executableURL for the binary itself, so instead attempt common install locations.
        }

        let candidates: [String] = [
            "/opt/homebrew/bin/opencleaner",
            "/usr/local/bin/opencleaner",
            "opencleaner"
        ]

        for c in candidates {
            if c == "opencleaner" {
                // Rely on PATH by invoking /usr/bin/which
                let which = URL(fileURLWithPath: "/usr/bin/which")
                if FileManager.default.isExecutableFile(atPath: which.path) {
                    let p = Process()
                    p.executableURL = which
                    p.arguments = ["opencleaner"]
                    let pipe = Pipe()
                    p.standardOutput = pipe
                    p.standardError = Pipe()
                    try? p.run()
                    p.waitUntilExit()
                    let data = pipe.fileHandleForReading.readDataToEndOfFile()
                    if let s = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines), !s.isEmpty {
                        let u = URL(fileURLWithPath: s)
                        if FileManager.default.isExecutableFile(atPath: u.path) { return u }
                    }
                }
                continue
            }

            let u = URL(fileURLWithPath: c)
            if FileManager.default.isExecutableFile(atPath: u.path) { return u }
        }
        return nil
    }
}

// MARK: - Daemon client (HTTP/JSON over Unix domain socket)

public struct OpenCleanerDaemonClient: OpenCleanerClientProtocol {
    public var socketPath: String

    public init(socketPath: String = "/tmp/opencleaner.sock") {
        self.socketPath = socketPath
    }

    public func status() async throws -> DaemonStatus {
        try await requestJSON(method: "GET", path: "/api/v1/status", body: nil, as: DaemonStatus.self)
    }

    public func scan() async throws -> ScanResult {
        try await requestJSON(method: "POST", path: "/api/v1/scan", body: nil, as: ScanResult.self)
    }

    public func clean(request: CleanRequest) async throws -> CleanResult {
        let enc = JSONEncoder()
        let body = try enc.encode(request)
        return try await requestJSON(method: "POST", path: "/api/v1/clean", body: body, as: CleanResult.self)
    }

    public func progressStream() -> AsyncThrowingStream<ProgressEvent, Error> {
        AsyncThrowingStream { continuation in
            let conn = NWConnection(to: .unix(path: socketPath), using: .tcp)
            let reader = SSEReader(connection: conn, continuation: continuation)
            continuation.onTermination = { [weak reader] _ in
                reader?.cancel()
            }
            reader.start()
        }
    }

    private func requestJSON<T: Decodable>(method: String, path: String, body: Data?, as type: T.Type) async throws -> T {
        let resp = try await request(method: method, path: path, body: body)
        let dec = JSONDecoder()
        dec.dateDecodingStrategy = .iso8601
        return try dec.decode(T.self, from: resp.body)
    }

    fileprivate struct HTTPResponse {
        let statusCode: Int
        let headers: [String: String]
        let body: Data
    }

    private func request(method: String, path: String, body: Data?) async throws -> HTTPResponse {
        let conn = NWConnection(to: .unix(path: socketPath), using: .tcp)
        let queue = DispatchQueue(label: "opencleaner.http")

        let body = body ?? Data()
        var req = "\(method) \(path) HTTP/1.1\r\n"
        req += "Host: unix\r\n"
        req += "Content-Type: application/json\r\n"
        req += "Content-Length: \(body.count)\r\n"
        req += "Connection: close\r\n\r\n"

        try await connect(conn, on: queue)
        var payload = Data(req.utf8)
        payload.append(body)
        try await send(conn, data: payload)
        let raw = try await receiveAll(conn)
        conn.cancel()

        let parsed = try parseHTTPResponse(raw)
        if parsed.statusCode >= 400 {
            let msg = String(data: parsed.body, encoding: .utf8) ?? ""
            throw OpenCleanerError.httpError(statusCode: parsed.statusCode, body: msg.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        return parsed
    }
}

fileprivate final class Once: @unchecked Sendable {
    private let lock = NSLock()
    private var ran = false

    func run(_ f: () -> Void) {
        lock.lock()
        defer { lock.unlock() }
        guard !ran else { return }
        ran = true
        f()
    }
}

fileprivate final class SSEReader: @unchecked Sendable {
    private let conn: NWConnection
    private var continuation: AsyncThrowingStream<ProgressEvent, Error>.Continuation?
    private let queue = DispatchQueue(label: "opencleaner.sse")
    private let finishOnce = Once()

    private var headerDone = false
    private var headerBuf = Data()

    private var httpIsChunked = false
    private var httpChunkBuf = Data()

    private var eventBuf = Data()
    private let maxEventBufferBytes = 4 << 20
    private let maxHeaderBytes = 64 * 1024
    private let maxChunkBufferBytes = 2 << 20

    private let headerMarker = Data("\r\n\r\n".utf8)

    init(connection: NWConnection, continuation: AsyncThrowingStream<ProgressEvent, Error>.Continuation) {
        self.conn = connection
        self.continuation = continuation
    }

    func start() {
        conn.stateUpdateHandler = { [weak self] state in
            self?.handleState(state)
        }
        conn.start(queue: queue)
    }

    func cancel() {
        queue.async { [weak self] in
            guard let self else { return }
            self.conn.cancel()
            self.finish(nil)
        }
    }

    private func finish(_ err: Error?) {
        finishOnce.run {
            if let err {
                self.continuation?.finish(throwing: err)
            } else {
                self.continuation?.finish()
            }
            self.continuation = nil
        }
    }

    private func yield(_ evt: ProgressEvent) {
        continuation?.yield(evt)
    }

    private func handleState(_ state: NWConnection.State) {
        switch state {
        case .ready:
            sendRequest()
        case .failed(let err):
            finish(err)
            conn.cancel()
        case .cancelled:
            finish(nil)
        default:
            break
        }
    }

    private func sendRequest() {
        let req = "GET /api/v1/progress/stream HTTP/1.1\r\nHost: unix\r\nAccept: text/event-stream\r\nConnection: keep-alive\r\n\r\n"
        conn.send(content: req.data(using: .utf8), completion: .contentProcessed { [weak self] err in
            guard let self else { return }
            if let err {
                self.finish(err)
                self.conn.cancel()
                return
            }
            self.receiveNext()
        })
    }

    private func receiveNext() {
        conn.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { [weak self] data, _, isComplete, err in
            guard let self else { return }
            if let err {
                self.finish(err)
                self.conn.cancel()
                return
            }
            if let data {
                self.process(data)
            }
            if isComplete {
                self.finish(nil)
                self.conn.cancel()
                return
            }
            self.receiveNext()
        }
    }

    private func process(_ data: Data) {
        if !headerDone {
            headerBuf.append(data)
            if headerBuf.count > maxHeaderBytes {
                finish(OpenCleanerError.invalidUTF8)
                conn.cancel()
                return
            }
            guard let hdrEnd = headerBuf.range(of: headerMarker) else { return }

            let hdrData = headerBuf.subdata(in: headerBuf.startIndex..<hdrEnd.lowerBound)
            let rest = headerBuf.subdata(in: hdrEnd.upperBound..<headerBuf.endIndex)
            headerBuf = Data()
            headerDone = true

            guard let hdrStr = String(data: hdrData, encoding: .utf8) else {
                finish(OpenCleanerError.invalidUTF8)
                conn.cancel()
                return
            }

            let (status, headers) = parseHTTPHeaderBlock(hdrStr)
            if status == 0 {
                finish(OpenCleanerError.invalidUTF8)
                conn.cancel()
                return
            }
            if status >= 400 {
                let body = String(decoding: rest, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
                finish(OpenCleanerError.httpError(statusCode: status, body: body.isEmpty ? hdrStr : body))
                conn.cancel()
                return
            }

            httpIsChunked = headers["transfer-encoding"]?.lowercased().contains("chunked") == true
            httpChunkBuf = Data()
            appendBodyBytes(rest)
        } else {
            appendBodyBytes(data)
        }
    }

    private func parseHTTPHeaderBlock(_ headerBlock: String) -> (Int, [String: String]) {
        let lines = headerBlock.components(separatedBy: "\r\n")
        let statusLine = lines.first ?? ""
        let statusParts = statusLine.split(separator: " ")
        let status = Int(statusParts.dropFirst().first ?? "") ?? 0

        var headers: [String: String] = [:]
        for l in lines.dropFirst() {
            guard let idx = l.firstIndex(of: ":") else { continue }
            let k = l[..<idx].trimmingCharacters(in: .whitespaces).lowercased()
            let v = l[l.index(after: idx)...].trimmingCharacters(in: .whitespaces)
            headers[k] = v
        }

        return (status, headers)
    }

    private func appendBodyBytes(_ bytes: Data) {
        do {
            var payload = bytes
            var chunkedDone = false

            if httpIsChunked {
                httpChunkBuf.append(bytes)
                if httpChunkBuf.count > maxChunkBufferBytes {
                    throw OpenCleanerError.invalidUTF8
                }
                let (decoded, done) = try decodeChunkedFromBuffer(&httpChunkBuf, maxBytes: 1 << 20)
                payload = decoded
                chunkedDone = done
            }

            if !payload.isEmpty {
                eventBuf.append(payload)
                if eventBuf.count > maxEventBufferBytes {
                    throw OpenCleanerError.invalidUTF8
                }
                try drainEventsFromBytes()
            }

            if chunkedDone {
                finish(nil)
                conn.cancel()
            }
        } catch {
            finish(error)
            conn.cancel()
        }
    }

    private func drainEventsFromBytes() throws {
        let rawEvents = try extractSSEEventsFromBuffer(&eventBuf)
        for raw in rawEvents {
            if let evt = parseSSEDataEvent(raw) {
                yield(evt)
            }
        }
    }
}

private func connect(_ conn: NWConnection, on queue: DispatchQueue) async throws {
    let once = Once()
    try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
        conn.stateUpdateHandler = { state in
            switch state {
            case .ready:
                once.run {
                    conn.stateUpdateHandler = nil
                    cont.resume(returning: ())
                }
            case .failed(let err):
                once.run {
                    conn.stateUpdateHandler = nil
                    cont.resume(throwing: err)
                }
            default:
                break
            }
        }
        conn.start(queue: queue)
    }
}

private func send(_ conn: NWConnection, data: Data) async throws {
    try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
        conn.send(content: data, completion: .contentProcessed { err in
            if let err {
                cont.resume(throwing: err)
            } else {
                cont.resume(returning: ())
            }
        })
    }
}

private func receiveAll(_ conn: NWConnection) async throws -> Data {
    var out = Data()

    while true {
        let (chunk, isComplete) = try await withCheckedThrowingContinuation { (cont: CheckedContinuation<(Data, Bool), Error>) in
            conn.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { data, _, isComplete, err in
                if let err {
                    cont.resume(throwing: err)
                    return
                }
                cont.resume(returning: (data ?? Data(), isComplete))
            }
        }

        out.append(chunk)
        if isComplete {
            return out
        }
    }
}

private func parseHTTPResponse(_ raw: Data) throws -> OpenCleanerDaemonClient.HTTPResponse {
    let marker = Data("\r\n\r\n".utf8)
    guard let headerEnd = raw.range(of: marker) else {
        throw OpenCleanerError.invalidUTF8
    }

    let headerData = raw.subdata(in: raw.startIndex..<headerEnd.lowerBound)
    var bodyData = raw.subdata(in: headerEnd.upperBound..<raw.endIndex)

    guard let headerStr = String(data: headerData, encoding: .utf8) else {
        throw OpenCleanerError.invalidUTF8
    }

    let lines = headerStr.components(separatedBy: "\r\n")
    guard let statusLine = lines.first, !statusLine.isEmpty else {
        throw OpenCleanerError.invalidUTF8
    }

    let statusParts = statusLine.split(separator: " ")
    let statusCode = Int(statusParts.dropFirst().first ?? "") ?? 0

    var headers: [String: String] = [:]
    for l in lines.dropFirst() {
        guard let idx = l.firstIndex(of: ":") else { continue }
        let k = l[..<idx].trimmingCharacters(in: .whitespaces).lowercased()
        let v = l[l.index(after: idx)...].trimmingCharacters(in: .whitespaces)
        headers[k] = v
    }

    if let te = headers["transfer-encoding"], te.lowercased().contains("chunked") {
        bodyData = try decodeChunkedBody(bodyData)
    } else if let cl = headers["content-length"], let n = Int(cl), bodyData.count >= n {
        bodyData = Data(bodyData.prefix(n))
    }

    return .init(statusCode: statusCode, headers: headers, body: bodyData)
}

private func decodeChunkedBody(_ data: Data) throws -> Data {
    var i = data.startIndex
    var out = Data()
    let crlf = Data("\r\n".utf8)

    func readLine() -> String? {
        guard let range = data.range(of: crlf, options: [], in: i..<data.endIndex) else { return nil }
        let lineData = data.subdata(in: i..<range.lowerBound)
        i = range.upperBound
        return String(data: lineData, encoding: .utf8)
    }

    while true {
        guard let line = readLine() else {
            throw OpenCleanerError.invalidUTF8
        }
        let sizeTokenRaw = line.split(separator: ";", maxSplits: 1, omittingEmptySubsequences: true).first ?? ""
        let sizeToken = String(sizeTokenRaw).trimmingCharacters(in: .whitespaces)
        guard let size = Int(sizeToken, radix: 16) else {
            throw OpenCleanerError.invalidUTF8
        }
        if size == 0 {
            // Consume optional trailers until the empty line terminator.
            while let t = readLine(), !t.isEmpty {
                _ = t
            }
            break
        }
        if data.distance(from: i, to: data.endIndex) < size + 2 {
            throw OpenCleanerError.invalidUTF8
        }
        out.append(data.subdata(in: i..<data.index(i, offsetBy: size)))
        i = data.index(i, offsetBy: size)

        let end = data.index(i, offsetBy: 2)
        guard data.subdata(in: i..<end) == crlf else {
            throw OpenCleanerError.invalidUTF8
        }
        i = end
    }

    return out
}

internal func decodeChunkedFromBuffer(_ buffer: inout Data, maxBytes: Int) throws -> (Data, Bool) {
    let crlf = Data("\r\n".utf8)
    let endMarker = Data("\r\n\r\n".utf8)
    var out = Data()

    while true {
        guard let lineEnd = buffer.range(of: crlf) else { break }
        let lineData = buffer.subdata(in: buffer.startIndex..<lineEnd.lowerBound)
        guard let lineStr = String(data: lineData, encoding: .utf8) else {
            throw OpenCleanerError.invalidUTF8
        }

        let sizeTokenRaw = lineStr.split(separator: ";", maxSplits: 1, omittingEmptySubsequences: true).first ?? ""
        let sizeToken = String(sizeTokenRaw).trimmingCharacters(in: .whitespaces)
        guard let size = Int(sizeToken, radix: 16) else {
            throw OpenCleanerError.invalidUTF8
        }

        let afterLine = lineEnd.upperBound
        if size == 0 {
            // Final chunk: `0\r\n` + (optional trailers ending with blank line) + `\r\n`.
            if buffer.count >= afterLine + 2 {
                let maybeCRLF = buffer.subdata(in: afterLine..<(afterLine + 2))
                if maybeCRLF == crlf {
                    buffer.removeSubrange(buffer.startIndex..<(afterLine + 2))
                    return (out, true)
                }
            }
            guard let trailerEnd = buffer.range(of: endMarker, options: [], in: afterLine..<buffer.endIndex) else { break }
            buffer.removeSubrange(buffer.startIndex..<trailerEnd.upperBound)
            return (out, true)
        }

        let chunkEnd = afterLine + size
        let needed = chunkEnd + 2
        if buffer.count < needed { break }

        out.append(buffer.subdata(in: afterLine..<chunkEnd))
        if out.count > maxBytes {
            throw OpenCleanerError.invalidUTF8
        }

        let trailer = buffer.subdata(in: chunkEnd..<needed)
        if trailer != crlf {
            throw OpenCleanerError.invalidUTF8
        }

        buffer.removeSubrange(buffer.startIndex..<needed)
    }

    return (out, false)
}

private let sseDelimCRLF = Data("\r\n\r\n".utf8)
private let sseDelimLF = Data("\n\n".utf8)

internal func extractSSEEventsFromBuffer(_ buffer: inout Data) throws -> [String] {
    var out: [String] = []

    while true {
        let r1 = buffer.range(of: sseDelimCRLF)
        let r2 = buffer.range(of: sseDelimLF)

        let delim: Range<Data.Index>?
        if let r1, let r2 {
            delim = (r1.lowerBound <= r2.lowerBound) ? r1 : r2
        } else {
            delim = r1 ?? r2
        }

        guard let delim else { return out }

        let eventBytes = buffer.subdata(in: buffer.startIndex..<delim.lowerBound)
        buffer.removeSubrange(buffer.startIndex..<delim.upperBound)

        guard let raw = String(data: eventBytes, encoding: .utf8) else {
            throw OpenCleanerError.invalidUTF8
        }

        out.append(raw.replacingOccurrences(of: "\r\n", with: "\n"))
    }
}

internal func parseSSEDataEvent(_ rawEvent: String) -> ProgressEvent? {
    // Accept multi-line `data:` events; ignore pings/comments.
    let lines = rawEvent.split(separator: "\n", omittingEmptySubsequences: false)
    var dataLines: [String] = []

    for lineSub in lines {
        let line = String(lineSub)
        if line.hasPrefix(":") { continue }

        if line.hasPrefix("data:") {
            var value = line.dropFirst("data:".count)
            if value.first == " " { value = value.dropFirst() } // only one optional space
            dataLines.append(String(value))
        }
    }

    guard !dataLines.isEmpty else { return nil }
    let json = dataLines.joined(separator: "\n")
    guard let d = json.data(using: .utf8) else { return nil }
    let dec = JSONDecoder()
    return try? dec.decode(ProgressEvent.self, from: d)
}
