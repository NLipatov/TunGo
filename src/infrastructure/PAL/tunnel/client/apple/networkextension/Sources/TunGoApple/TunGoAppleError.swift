import Foundation

enum TunGoAppleError: LocalizedError {
    case backend(String)
    case invalidPrefixLength(Int)
    case tunnelFileDescriptorNotFound
    case invalidState

    var errorDescription: String? {
        switch self {
        case .backend(let message):
            return message
        case .invalidPrefixLength(let length):
            return "Invalid network prefix length: \(length)."
        case .tunnelFileDescriptorNotFound:
            return "Could not locate the NetworkExtension UTUN file descriptor."
        case .invalidState:
            return "The TunGo backend is not in a valid state for this operation."
        }
    }
}
