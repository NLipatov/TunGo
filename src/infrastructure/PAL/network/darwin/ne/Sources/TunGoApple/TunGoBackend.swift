import CTunGo
import Foundation

final class TunGoBackend: Sendable {
    private let handle: tungo_controller_handle_t

    init() {
        handle = tungo_controller_create()
        precondition(handle != 0, "The Go backend returned an invalid controller handle.")
    }

    deinit {
        tungo_controller_destroy(handle)
    }

    static func networkSettings() throws -> PacketTunnelSettings {
        var output: UnsafeMutablePointer<CChar>?
        let errorPointer = tungo_network_settings(&output)
        try Self.throwBackendError(errorPointer)
        guard let output else {
            throw TunGoAppleError.backend("The Go backend returned no network settings.")
        }
        defer { tungo_free(UnsafeMutableRawPointer(output)) }
        return try JSONDecoder().decode(
            PacketTunnelSettings.self,
            from: Data(bytes: output, count: strlen(output))
        )
    }

    func start(tunnelFileDescriptor: Int32) throws {
        try Self.throwBackendError(tungo_start(handle, tunnelFileDescriptor))
    }

    func waitUntilReady(timeoutMilliseconds: Int64) throws {
        try Self.throwBackendError(tungo_wait_ready(handle, timeoutMilliseconds))
    }

    func stop() throws {
        try Self.throwBackendError(tungo_stop(handle))
    }

    private static func throwBackendError(_ pointer: UnsafeMutablePointer<CChar>?) throws {
        guard let pointer else { return }
        defer { tungo_free(UnsafeMutableRawPointer(pointer)) }
        throw TunGoAppleError.backend(String(cString: pointer))
    }
}
