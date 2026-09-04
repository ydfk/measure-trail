import SwiftData

@MainActor
enum LocalDataCleaner {
    static func clear(context: ModelContext) throws {
        for item in try context.fetch(FetchDescriptor<CachedMeasurement>()) { context.delete(item) }
        for item in try context.fetch(FetchDescriptor<PendingMutation>()) { context.delete(item) }
        for item in try context.fetch(FetchDescriptor<SyncConflict>()) { context.delete(item) }
        for item in try context.fetch(FetchDescriptor<SyncCheckpoint>()) { context.delete(item) }
        try context.save()
        HealthKitService.shared.clearAccountState()
    }
}
