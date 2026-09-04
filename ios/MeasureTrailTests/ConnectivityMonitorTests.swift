import Network
import XCTest
@testable import MeasureTrail

final class ConnectivityMonitorTests: XCTestCase {
    func testOnlySatisfiedPathIsOnline() {
        XCTAssertTrue(NetworkAvailability.isOnline(.satisfied))
        XCTAssertFalse(NetworkAvailability.isOnline(.requiresConnection))
        XCTAssertFalse(NetworkAvailability.isOnline(.unsatisfied))
    }
}
