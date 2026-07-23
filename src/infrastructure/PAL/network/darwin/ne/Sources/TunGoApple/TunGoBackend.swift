import CTunGo
import Foundation

struct TunGoBackend: Sendable {
    func networkSettings() throws -> NetworkSettings {
        var output: UnsafeMutablePointer<CChar>?
        let errorPointer = tungo_network_settings(&output)
        try Self.throwBackendError(errorPointer)
        guard let output else {
            throw TunGoAppleError.backend("The Go backend returned no network settings.")
        }
        defer { tungo_free(UnsafeMutableRawPointer(output)) }
        return try JSONDecoder().decode(
            NetworkSettings.self,
            from: Data(bytes: output, count: strlen(output))
        )
    }

    func start(tunnelFileDescriptor: Int32) throws {
        try Self.throwBackendError(tungo_start(tunnelFileDescriptor))
    }

    func waitUntilReady(timeoutMilliseconds: Int64) throws {
        try Self.throwBackendError(tungo_wait_ready(timeoutMilliseconds))
    }

    func stop() throws {
        try Self.throwBackendError(tungo_stop())
    }

    private static func throwBackendError(_ pointer: UnsafeMutablePointer<CChar>?) throws {
        guard let pointer else { return }
        defer { tungo_free(UnsafeMutableRawPointer(pointer)) }
        throw TunGoAppleError.backend(String(cString: pointer))
    }
}
