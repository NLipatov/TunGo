import Foundation
import NetworkExtension

open class PacketTunnelProvider: NEPacketTunnelProvider {
    private lazy var adapter = PacketTunnelAdapter(provider: self)

    public override func startTunnel(
        options: [String: NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        adapter.start(completion: completionHandler)
    }

    public override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        adapter.stop { _ in completionHandler() }
    }
}
