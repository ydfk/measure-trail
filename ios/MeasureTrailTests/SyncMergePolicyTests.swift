import XCTest
@testable import MeasureTrail

final class SyncMergePolicyTests: XCTestCase {
    func testAppliesSameOrNewerRemoteVersionWithoutPendingMutation() {
        XCTAssertTrue(SyncMergePolicy.shouldApplyRemoteChange(localVersion: 3, hasPendingLocalMutation: false, remoteVersion: 3))
        XCTAssertTrue(SyncMergePolicy.shouldApplyRemoteChange(localVersion: 3, hasPendingLocalMutation: false, remoteVersion: 4))
    }

    func testPreservesPendingLocalMutation() {
        XCTAssertFalse(SyncMergePolicy.shouldApplyRemoteChange(localVersion: 3, hasPendingLocalMutation: true, remoteVersion: 4))
    }

    func testRejectsOlderRemoteVersion() {
        XCTAssertFalse(SyncMergePolicy.shouldApplyRemoteChange(localVersion: 3, hasPendingLocalMutation: false, remoteVersion: 2))
    }
}
