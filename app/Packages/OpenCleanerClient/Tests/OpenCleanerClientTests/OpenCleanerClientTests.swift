import Foundation
import Testing

@testable import OpenCleanerClient

@Test func chunkedDecoder_handlesFragmentationAndTrailers() throws {
    var buf = Data("4\r\nWi".utf8)

    let (d1, done1) = try decodeChunkedFromBuffer(&buf, maxBytes: 1 << 20)
    #expect(d1.isEmpty)
    #expect(done1 == false)

    buf.append(Data("ki\r\n5\r\npedia\r\n0\r\nX: y\r\n\r\n".utf8))

    let (d2, done2) = try decodeChunkedFromBuffer(&buf, maxBytes: 1 << 20)
    #expect(String(data: d2, encoding: .utf8) == "Wikipedia")
    #expect(done2 == true)
    #expect(buf.isEmpty)
}

@Test func sseParser_decodesMultilineDataEvent() throws {
    let raw = "data: {\"type\":\"scanning\",\n" +
        "data:  \"progress\":0.2,\n" +
        "data:  \"message\":\"starting\"}"

    let evt = parseSSEDataEvent(raw)
    #expect(evt?.type == "scanning")
    #expect(evt?.progress == 0.2)
    #expect(evt?.message == "starting")
}

@Test func sseFramer_handlesUTF8SplitAcrossAppends() throws {
    // Split inside a multi-byte UTF-8 scalar (é) to ensure we don't corrupt decoding.
    let frame = "data: {\"type\":\"scanning\",\"message\":\"café\"}\n\n"
    let bytes = Array(frame.utf8)

    guard let c3Index = bytes.firstIndex(of: 0xC3) else {
        throw OpenCleanerError.invalidUTF8
    }
    let split = c3Index + 1

    var buf = Data(bytes[0..<split])
    var events = try extractSSEEventsFromBuffer(&buf)
    #expect(events.isEmpty)

    buf.append(Data(bytes[split..<bytes.count]))
    events = try extractSSEEventsFromBuffer(&buf)

    #expect(events.count == 1)
    let evt = parseSSEDataEvent(events[0])
    #expect(evt?.message == "café")
}

private final class FakeProgressClient: OpenCleanerClientProtocol, @unchecked Sendable {
    private let lock = NSLock()
    private var calls = 0

    func status() async throws -> DaemonStatus { throw OpenCleanerError.invalidUTF8 }
    func scan() async throws -> ScanResult { throw OpenCleanerError.invalidUTF8 }
    func clean(request: CleanRequest) async throws -> CleanResult { throw OpenCleanerError.invalidUTF8 }

    func progressStream() -> AsyncThrowingStream<ProgressEvent, Error> {
        lock.lock()
        calls += 1
        let n = calls
        lock.unlock()

        return AsyncThrowingStream { continuation in
            continuation.yield(ProgressEvent(type: n == 1 ? "one" : "two"))
            continuation.finish()
        }
    }
}

@Test func progressStream_reconnectsAfterCompletion() async throws {
    let client = FakeProgressClient()
    let policy = SSEReconnectPolicy(
        initialDelaySeconds: 0,
        maxDelaySeconds: 0,
        multiplier: 1,
        jitterFraction: 0,
        maxAttempts: 3,
        reconnectOnCompletion: true
    )

    var got: [String] = []
    for try await evt in client.progressStream(reconnect: policy) {
        got.append(evt.type)
        if got.count == 2 { break }
    }

    #expect(got == ["one", "two"])
}
