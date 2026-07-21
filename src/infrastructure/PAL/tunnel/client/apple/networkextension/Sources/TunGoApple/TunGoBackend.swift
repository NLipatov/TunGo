import CTunGo
import Foundation

final class TunGoBackend {
    enum State: Int32 {
        case starting = 1
        case running = 2
        case stopped = 3
        case failed = 4
    }

    struct Status {
        let state: State
        let detail: String?
    }

    private(set) var handle: UInt64?

    func plan() throws -> TunnelPlan {
        var output: UnsafeMutablePointer<CChar>?
        let errorPointer = tungo_plan(&output)
        try Self.throwBackendError(errorPointer)
        guard let output else {
            throw TunGoAppleError.backend("The Go backend returned no tunnel plan.")
        }
        defer { tungo_free(UnsafeMutableRawPointer(output)) }
        return try JSONDecoder().decode(TunnelPlan.self, from: Data(bytes: output, count: strlen(output)))
    }

    func start(tunnelFileDescriptor: Int32) throws {
        guard handle == nil else { throw TunGoAppleError.invalidState }
        var newHandle: UInt64 = 0
        let errorPointer = tungo_start(tunnelFileDescriptor, &newHandle)
        try Self.throwBackendError(errorPointer)
        handle = newHandle
    }

    func waitUntilReady(timeoutMilliseconds: Int64) throws {
        guard let handle else { throw TunGoAppleError.invalidState }
        try Self.throwBackendError(tungo_wait_ready(handle, timeoutMilliseconds))
    }

    func stop() throws {
        guard let handle else { return }
        try Self.throwBackendError(tungo_stop(handle))
        self.handle = nil
    }

    func pause() throws {
        guard let handle else { throw TunGoAppleError.invalidState }
        try Self.throwBackendError(tungo_pause(handle))
    }

    func restart() throws {
        guard let handle else { throw TunGoAppleError.invalidState }
        try Self.throwBackendError(tungo_restart(handle))
    }

    func status() throws -> Status {
        guard let handle else { return Status(state: .stopped, detail: nil) }
        var rawState: Int32 = 0
        var detailPointer: UnsafeMutablePointer<CChar>?
        try Self.throwBackendError(tungo_status(handle, &rawState, &detailPointer))
        let detail = detailPointer.map { String(cString: $0) }
        if let detailPointer {
            tungo_free(UnsafeMutableRawPointer(detailPointer))
        }
        guard let state = State(rawValue: rawState) else {
            throw TunGoAppleError.backend("The Go backend returned unknown state \(rawState).")
        }
        return Status(state: state, detail: detail)
    }

    private static func throwBackendError(_ pointer: UnsafeMutablePointer<CChar>?) throws {
        guard let pointer else { return }
        defer { tungo_free(UnsafeMutableRawPointer(pointer)) }
        throw TunGoAppleError.backend(String(cString: pointer))
    }
}
