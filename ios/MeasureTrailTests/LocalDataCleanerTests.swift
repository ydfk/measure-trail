import SwiftData
import XCTest
@testable import MeasureTrail

final class LocalDataCleanerTests: XCTestCase {
    @MainActor
    func testClearRemovesEveryAccountScopedSwiftDataModel() throws {
        let container = try ModelContainer(for: CachedMeasurement.self, PendingMutation.self, SyncConflict.self, SyncCheckpoint.self, configurations: ModelConfiguration(isStoredInMemoryOnly: true))
        let context = ModelContext(container)
        let measurement = CachedMeasurement(recordedOn: .now, weightG: 76_000, waistMM: nil, note: "本地记录")
        context.insert(measurement)
        context.insert(PendingMutation(operation: "upsertMeasurement", measurementID: measurement.id, recordedOn: measurement.recordedOn))
        context.insert(SyncConflict(measurementID: measurement.id, operation: "upsertMeasurement", remoteWeightG: 75_000, remoteWaistMM: nil, remoteNote: "云端记录", remoteVersion: 2, remoteDeletedAt: nil))
        context.insert(SyncCheckpoint())
        try context.save()

        try LocalDataCleaner.clear(context: context)

        XCTAssertTrue(try context.fetch(FetchDescriptor<CachedMeasurement>()).isEmpty)
        XCTAssertTrue(try context.fetch(FetchDescriptor<PendingMutation>()).isEmpty)
        XCTAssertTrue(try context.fetch(FetchDescriptor<SyncConflict>()).isEmpty)
        XCTAssertTrue(try context.fetch(FetchDescriptor<SyncCheckpoint>()).isEmpty)
    }
}
