import Foundation
import NetworkExtension

struct TunnelPlan: Decodable {
    struct IPSettings: Decodable {
        let address: String
        let prefixLength: Int
    }

    struct Route: Decodable {
        let destination: String
        let prefixLength: Int
    }

    let remoteAddress: String
    let mtu: Int
    let startupTimeoutMilliseconds: Int64
    let ipv4: IPSettings?
    let ipv6: IPSettings?
    let dnsServers: [String]
    let includedRoutes: [Route]
    let excludedRoutes: [Route]

    func networkSettings() throws -> NEPacketTunnelNetworkSettings {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: remoteAddress)
        settings.mtu = NSNumber(value: mtu)

        if let ipv4 {
            let interfaceSettings = NEIPv4Settings(
                addresses: [ipv4.address],
                subnetMasks: [try Self.ipv4Mask(prefixLength: ipv4.prefixLength)]
            )
            interfaceSettings.includedRoutes = try includedRoutes.compactMap(Self.ipv4Route)
            interfaceSettings.excludedRoutes = try excludedRoutes.compactMap(Self.ipv4Route)
            settings.ipv4Settings = interfaceSettings
        }

        if let ipv6 {
            let interfaceSettings = NEIPv6Settings(
                addresses: [ipv6.address],
                networkPrefixLengths: [NSNumber(value: ipv6.prefixLength)]
            )
            interfaceSettings.includedRoutes = try includedRoutes.compactMap(Self.ipv6Route)
            interfaceSettings.excludedRoutes = try excludedRoutes.compactMap(Self.ipv6Route)
            settings.ipv6Settings = interfaceSettings
        }

        if !dnsServers.isEmpty {
            settings.dnsSettings = NEDNSSettings(servers: dnsServers)
        }
        return settings
    }

    private static func ipv4Route(_ route: Route) throws -> NEIPv4Route? {
        guard route.destination.contains(".") else { return nil }
        return NEIPv4Route(
            destinationAddress: route.destination,
            subnetMask: try ipv4Mask(prefixLength: route.prefixLength)
        )
    }

    private static func ipv6Route(_ route: Route) throws -> NEIPv6Route? {
        guard route.destination.contains(":") else { return nil }
        guard (0...128).contains(route.prefixLength) else {
            throw TunGoAppleError.invalidPrefixLength(route.prefixLength)
        }
        return NEIPv6Route(
            destinationAddress: route.destination,
            networkPrefixLength: NSNumber(value: route.prefixLength)
        )
    }

    private static func ipv4Mask(prefixLength: Int) throws -> String {
        guard (0...32).contains(prefixLength) else {
            throw TunGoAppleError.invalidPrefixLength(prefixLength)
        }
        let mask: UInt32 = prefixLength == 0 ? 0 : UInt32.max << UInt32(32 - prefixLength)
        return [24, 16, 8, 0]
            .map { String((mask >> UInt32($0)) & 0xff) }
            .joined(separator: ".")
    }
}
