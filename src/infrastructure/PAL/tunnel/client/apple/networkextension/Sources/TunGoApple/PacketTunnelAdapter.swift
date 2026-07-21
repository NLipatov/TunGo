import CTunGo
import Foundation
import Network
import NetworkExtension

// Mutable state is confined to queue. The unchecked conformance documents that
// synchronization boundary for Network.framework's @Sendable callbacks.
final class PacketTunnelAdapter: @unchecked Sendable {
    private final class CompletionBox<Value>: @unchecked Sendable {
        private let callback: (Value) -> Void

        init(_ callback: @escaping (Value) -> Void) {
            self.callback = callback
        }

        func callAsFunction(_ value: Value) {
            callback(value)
        }
    }

    private enum State {
        case stopped
        case starting
        case running
        case paused
    }

    private weak var provider: NEPacketTunnelProvider?
    private let backend = TunGoBackend()
    private let queue = DispatchQueue(label: "io.tungo.apple.packet-tunnel")
    private var monitor: NWPathMonitor?
    private var state = State.stopped
    private var receivedInitialPath = false
    private var startupTimeoutMilliseconds: Int64 = 6000

    init(provider: NEPacketTunnelProvider) {
        self.provider = provider
    }

    func start(completion: @escaping (Error?) -> Void) {
        let completion = CompletionBox(completion)
        queue.async {
            guard self.state == .stopped, let provider = self.provider else {
                completion(TunGoAppleError.invalidState)
                return
            }
            self.state = .starting
            do {
                let plan = try self.backend.plan()
                self.startupTimeoutMilliseconds = plan.startupTimeoutMilliseconds
                let networkSettings = try plan.networkSettings()
                provider.setTunnelNetworkSettings(networkSettings) { error in
                    self.queue.async {
                        if let error {
                            self.state = .stopped
                            completion(error)
                            return
                        }
                        let fd = tungo_find_utun_fd()
                        guard fd >= 0 else {
                            self.state = .stopped
                            completion(TunGoAppleError.tunnelFileDescriptorNotFound)
                            return
                        }
                        do {
                            try self.backend.start(tunnelFileDescriptor: fd)
                            do {
                                try self.backend.waitUntilReady(
                                    timeoutMilliseconds: plan.startupTimeoutMilliseconds
                                )
                            } catch {
                                try? self.backend.stop()
                                throw error
                            }
                            self.state = .running
                            self.startPathMonitor()
                            completion(nil)
                        } catch {
                            self.state = .stopped
                            completion(error)
                        }
                    }
                }
            } catch {
                self.state = .stopped
                completion(error)
            }
        }
    }

    func stop(completion: @escaping (Error?) -> Void) {
        let completion = CompletionBox(completion)
        queue.async {
            self.monitor?.cancel()
            self.monitor = nil
            self.receivedInitialPath = false
            self.startupTimeoutMilliseconds = 6000
            do {
                try self.backend.stop()
                self.state = .stopped
                completion(nil)
            } catch {
                self.state = .stopped
                completion(error)
            }
        }
    }

    private func startPathMonitor() {
        let monitor = NWPathMonitor()
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            self.queue.async {
                self.handlePathUpdate(path)
            }
        }
        monitor.start(queue: queue)
        self.monitor = monitor
    }

    private func handlePathUpdate(_ path: Network.NWPath) {
        guard state == .running || state == .paused else { return }
        if !receivedInitialPath {
            receivedInitialPath = true
            if path.status != .unsatisfied { return }
        }

        provider?.reasserting = true
        defer { provider?.reasserting = false }
        do {
            if path.status == .unsatisfied {
                try backend.pause()
                state = .paused
            } else if state == .paused {
                try backend.restart()
                try backend.waitUntilReady(timeoutMilliseconds: startupTimeoutMilliseconds)
                state = .running
            } else {
                try backend.restart()
                try backend.waitUntilReady(timeoutMilliseconds: startupTimeoutMilliseconds)
            }
        } catch {
            try? backend.stop()
            state = .stopped
            provider?.cancelTunnelWithError(error)
        }
    }
}
