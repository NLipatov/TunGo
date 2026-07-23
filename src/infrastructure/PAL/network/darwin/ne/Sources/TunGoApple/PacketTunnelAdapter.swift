import CTunGo
import Foundation
@preconcurrency import NetworkExtension

// Mutable state is confined to stateQueue. Lifecycle calls are serialized
// separately so stopTunnel can interrupt a blocking readiness wait.
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
        case stopping
    }

    private weak var provider: NEPacketTunnelProvider?
    private let backend = TunGoBackend()
    private let stateQueue = DispatchQueue(label: "io.tungo.apple.packet-tunnel.state")
    private let lifecycleQueue = DispatchQueue(label: "io.tungo.apple.packet-tunnel.lifecycle")
    private let waitQueue = DispatchQueue(label: "io.tungo.apple.packet-tunnel.wait")
    private var state = State.stopped
    private var generation: UInt64 = 0
    private var networkSettingsPending = false
    private var pendingStartCompletion: CompletionBox<Error?>?
    private var pendingStartFailure: Error?
    private var pendingStopCompletion: CompletionBox<Error?>?
    private var pendingStopResult: Result<Void, Error>?

    init(provider: NEPacketTunnelProvider) {
        self.provider = provider
    }

    func start(completion: @escaping (Error?) -> Void) {
        let completion = CompletionBox(completion)
        stateQueue.async {
            guard self.state == .stopped, let provider = self.provider else {
                completion(TunGoAppleError.invalidState)
                return
            }
            self.generation &+= 1
            let generation = self.generation
            self.state = .starting
            self.pendingStartCompletion = completion
            self.pendingStartFailure = nil

            self.lifecycleQueue.async {
                do {
                    let networkSettings = try self.backend.networkSettings()
                    let packetTunnelSettings =
                        try networkSettings.makeNEPacketTunnelNetworkSettings()
                    self.stateQueue.async {
                        self.apply(
                            packetTunnelSettings,
                            startupTimeoutMilliseconds:
                                networkSettings.startupTimeoutMilliseconds,
                            provider: provider,
                            generation: generation
                        )
                    }
                } catch {
                    self.stateQueue.async {
                        self.failBeforeBackendStart(error, generation: generation)
                    }
                }
            }
        }
    }

    func stop(completion: @escaping (Error?) -> Void) {
        let completion = CompletionBox(completion)
        stateQueue.async {
            switch self.state {
            case .stopped:
                completion(nil)
            case .stopping:
                completion(TunGoAppleError.invalidState)
            case .starting, .running:
                self.generation &+= 1
                self.state = .stopping
                self.pendingStartFailure = TunGoAppleError.startupCancelled
                self.pendingStopCompletion = completion
                self.pendingStopResult = nil
                self.stopBackend()
            }
        }
    }

    private func apply(
        _ networkSettings: NEPacketTunnelNetworkSettings,
        startupTimeoutMilliseconds: Int64,
        provider: NEPacketTunnelProvider,
        generation: UInt64
    ) {
        guard self.generation == generation, state == .starting else { return }
        networkSettingsPending = true
        provider.setTunnelNetworkSettings(networkSettings) { error in
            self.stateQueue.async {
                self.networkSettingsPending = false
                guard self.generation == generation, self.state == .starting else {
                    self.finishStoppingIfPossible()
                    return
                }
                if let error {
                    self.failBeforeBackendStart(error, generation: generation)
                    return
                }
                self.startBackend(
                    startupTimeoutMilliseconds: startupTimeoutMilliseconds,
                    generation: generation
                )
            }
        }
    }

    private func startBackend(
        startupTimeoutMilliseconds: Int64,
        generation: UInt64
    ) {
        lifecycleQueue.async {
            do {
                let fileDescriptor = tungo_find_utun_fd()
                guard fileDescriptor >= 0 else {
                    throw TunGoAppleError.tunnelFileDescriptorNotFound
                }
                try self.backend.start(tunnelFileDescriptor: fileDescriptor)
                self.waitQueue.async {
                    do {
                        try self.backend.waitUntilReady(
                            timeoutMilliseconds: startupTimeoutMilliseconds
                        )
                        self.stateQueue.async {
                            guard self.generation == generation,
                                  self.state == .starting else { return }
                            self.state = .running
                            let completion = self.pendingStartCompletion
                            self.pendingStartCompletion = nil
                            self.pendingStartFailure = nil
                            completion?(nil)
                        }
                    } catch {
                        self.stateQueue.async {
                            self.failAfterBackendStart(error, generation: generation)
                        }
                    }
                }
            } catch {
                self.stateQueue.async {
                    self.failAfterBackendStart(error, generation: generation)
                }
            }
        }
    }

    private func failBeforeBackendStart(_ error: Error, generation: UInt64) {
        guard self.generation == generation, state == .starting else { return }
        state = .stopped
        let completion = pendingStartCompletion
        pendingStartCompletion = nil
        pendingStartFailure = nil
        completion?(error)
    }

    private func failAfterBackendStart(_ error: Error, generation: UInt64) {
        guard self.generation == generation, state == .starting else { return }
        self.generation &+= 1
        state = .stopping
        pendingStartFailure = error
        pendingStopResult = nil
        stopBackend()
    }

    private func stopBackend() {
        lifecycleQueue.async {
            let result = Result { try self.backend.stop() }
            self.stateQueue.async {
                self.pendingStopResult = result
                self.finishStoppingIfPossible()
            }
        }
    }

    private func finishStoppingIfPossible() {
        guard state == .stopping,
              !networkSettingsPending,
              let stopResult = pendingStopResult else { return }

        let startCompletion = pendingStartCompletion
        let startFailure = pendingStartFailure
        let stopCompletion = pendingStopCompletion
        pendingStartCompletion = nil
        pendingStartFailure = nil
        pendingStopCompletion = nil
        pendingStopResult = nil
        state = .stopped

        if let startCompletion {
            startCompletion(startFailure ?? TunGoAppleError.startupCancelled)
        }
        switch stopResult {
        case .success:
            stopCompletion?(nil)
        case .failure(let error):
            stopCompletion?(error)
        }
    }
}
