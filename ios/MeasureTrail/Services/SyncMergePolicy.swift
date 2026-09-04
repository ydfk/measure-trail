import Foundation

enum SyncMergePolicy {
    static func shouldApplyRemoteChange(localVersion: Int, hasPendingLocalMutation: Bool, remoteVersion: Int) -> Bool {
        !hasPendingLocalMutation && remoteVersion >= localVersion
    }
}
