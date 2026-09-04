import Foundation

enum SyncConflictResolution {
    enum Action: Equatable {
        case removeLocalMeasurement
        case applyRemoteMeasurement
        case queueDelete(expectedVersion: Int)
        case queueUpsert(expectedVersion: Int?)
    }

    static func actionForAcceptingRemote(remoteWasDeleted: Bool) -> Action {
        remoteWasDeleted ? .removeLocalMeasurement : .applyRemoteMeasurement
    }

    static func actionForKeepingLocal(operation: String, remoteWasDeleted: Bool, remoteVersion: Int) -> Action {
        if operation == "deleteMeasurement" {
            return remoteWasDeleted ? .removeLocalMeasurement : .queueDelete(expectedVersion: remoteVersion)
        }
        return .queueUpsert(expectedVersion: remoteWasDeleted ? nil : remoteVersion)
    }
}
