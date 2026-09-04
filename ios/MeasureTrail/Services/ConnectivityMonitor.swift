import Foundation
import Network
import Observation

enum NetworkAvailability {
    static func isOnline(_ status: NWPath.Status) -> Bool {
        status == .satisfied
    }
}

@Observable
@MainActor
final class ConnectivityMonitor {
    private let monitor = NWPathMonitor()
    private let queue = DispatchQueue(label: "com.ydfk.MeasureTrail.connectivity")
    private(set) var isOnline = false

    init() {
        monitor.pathUpdateHandler = { [weak self] path in
            let isOnline = NetworkAvailability.isOnline(path.status)
            Task { @MainActor [weak self] in
                self?.isOnline = isOnline
            }
        }
        monitor.start(queue: queue)
    }

    deinit {
        monitor.cancel()
    }
}
