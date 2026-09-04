import Foundation
import SwiftData

@Model
final class CachedMeasurement {
    @Attribute(.unique) var id: UUID
    var recordedOn: Date
    var weightG: Int
    var waistMM: Int?
    var note: String
    var source: String
    var healthKitUUID: String?
    var version: Int
    var serverID: String?
    var updatedAt: Date
    var syncState: String
    var isDeleted: Bool

    init(recordedOn: Date, weightG: Int, waistMM: Int?, note: String, source: String = "manual", healthKitUUID: String? = nil, version: Int = 0, serverID: String? = nil, syncState: String = "pending", isDeleted: Bool = false) {
        id = UUID()
        self.recordedOn = recordedOn
        self.weightG = weightG
        self.waistMM = waistMM
        self.note = note
        self.source = source
        self.healthKitUUID = healthKitUUID
        self.version = version
        self.serverID = serverID
        updatedAt = .now
        self.syncState = syncState
        self.isDeleted = isDeleted
    }
}

@Model
final class PendingMutation {
    @Attribute(.unique) var id: UUID
    var operation: String
    var measurementID: UUID
    var expectedVersion: Int
    var recordedOn: Date
    var createdAt: Date

    init(operation: String, measurementID: UUID, recordedOn: Date, expectedVersion: Int = 0) {
        id = UUID()
        self.operation = operation
        self.measurementID = measurementID
        self.expectedVersion = expectedVersion
        self.recordedOn = recordedOn
        createdAt = .now
    }
}

@Model
final class SyncConflict {
    @Attribute(.unique) var measurementID: UUID
    var operation: String
    var remoteWeightG: Int
    var remoteWaistMM: Int?
    var remoteNote: String
    var remoteVersion: Int
    var remoteDeletedAt: Date?
    var createdAt: Date

    init(measurementID: UUID, operation: String, remoteWeightG: Int, remoteWaistMM: Int?, remoteNote: String, remoteVersion: Int, remoteDeletedAt: Date?) {
        self.measurementID = measurementID
        self.operation = operation
        self.remoteWeightG = remoteWeightG
        self.remoteWaistMM = remoteWaistMM
        self.remoteNote = remoteNote
        self.remoteVersion = remoteVersion
        self.remoteDeletedAt = remoteDeletedAt
        createdAt = .now
    }
}

@Model
final class SyncCheckpoint {
    @Attribute(.unique) var scope: String
    var measurementCursor: String

    init(scope: String = "account", measurementCursor: String = "") {
        self.scope = scope
        self.measurementCursor = measurementCursor
    }
}
