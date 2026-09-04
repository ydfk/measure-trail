import SwiftData
import XCTest
@testable import MeasureTrail

final class SyncConflictResolutionTests: XCTestCase {
    func testAcceptingRemoteDeletesLocalWhenRemoteWasDeleted() {
        XCTAssertEqual(SyncConflictResolution.actionForAcceptingRemote(remoteWasDeleted: true), .removeLocalMeasurement)
    }

    func testAcceptingRemoteAppliesLiveRemoteMeasurement() {
        XCTAssertEqual(SyncConflictResolution.actionForAcceptingRemote(remoteWasDeleted: false), .applyRemoteMeasurement)
    }

    func testKeepingLocalDeleteQueuesDeleteAgainstRemoteVersion() {
        XCTAssertEqual(SyncConflictResolution.actionForKeepingLocal(operation: "deleteMeasurement", remoteWasDeleted: false, remoteVersion: 8), .queueDelete(expectedVersion: 8))
    }

    func testKeepingLocalUpdateAfterRemoteDeletionCreatesFreshMeasurement() {
        XCTAssertEqual(SyncConflictResolution.actionForKeepingLocal(operation: "upsertMeasurement", remoteWasDeleted: true, remoteVersion: 8), .queueUpsert(expectedVersion: nil))
    }

    func testKeepingLocalUpdateUsesLatestRemoteVersion() {
        XCTAssertEqual(SyncConflictResolution.actionForKeepingLocal(operation: "upsertMeasurement", remoteWasDeleted: false, remoteVersion: 8), .queueUpsert(expectedVersion: 8))
    }

    @MainActor
    func testCapturingConflictStoresRemoteVersionAndMarksLocalMeasurement() async throws {
        let container = try ModelContainer(for: CachedMeasurement.self, PendingMutation.self, SyncConflict.self, configurations: ModelConfiguration(isStoredInMemoryOnly: true))
        let context = ModelContext(container)
        let local = CachedMeasurement(recordedOn: Date(timeIntervalSince1970: 0), weightG: 76_000, waistMM: 810, note: "本机备注", version: 2, serverID: "remote-id")
        let mutation = PendingMutation(operation: "upsertMeasurement", measurementID: local.id, recordedOn: local.recordedOn, expectedVersion: 2)
        context.insert(local)
        context.insert(mutation)

        let transport = ConflictTransportStub(remote: remoteMeasurement())
        let coordinator = SyncCoordinator(engine: SyncEngine(transport: transport), transport: transport)
        let request = SyncEngine.Request(mutationID: mutation.id, recordedOn: local.recordedOn, weightG: local.weightG, waistMM: local.waistMM, note: local.note, operation: mutation.operation, serverID: local.serverID, expectedVersion: mutation.expectedVersion)

        try await coordinator.captureConflicts([mutation.id], mutations: [mutation], requests: [request], cached: [local], context: context, accessToken: "access-token")
        try context.save()

        let conflicts = try context.fetch(FetchDescriptor<SyncConflict>())
        XCTAssertEqual(local.syncState, "conflict")
        XCTAssertEqual(conflicts.count, 1)
        XCTAssertEqual(conflicts[0].measurementID, local.id)
        XCTAssertEqual(conflicts[0].remoteWeightG, 75_600)
        XCTAssertEqual(conflicts[0].remoteVersion, 3)
    }

    @MainActor
    func testCapturingCreateConflictFindsRemoteMeasurementByDate() async throws {
        let container = try ModelContainer(for: CachedMeasurement.self, PendingMutation.self, SyncConflict.self, configurations: ModelConfiguration(isStoredInMemoryOnly: true))
        let context = ModelContext(container)
        let local = CachedMeasurement(recordedOn: Date(timeIntervalSince1970: 0), weightG: 76_000, waistMM: nil, note: "离线记录")
        let mutation = PendingMutation(operation: "upsertMeasurement", measurementID: local.id, recordedOn: local.recordedOn)
        context.insert(local)
        context.insert(mutation)

        let transport = ConflictTransportStub(remote: remoteMeasurement())
        let coordinator = SyncCoordinator(engine: SyncEngine(transport: transport), transport: transport)
        let request = SyncEngine.Request(mutationID: mutation.id, recordedOn: local.recordedOn, weightG: local.weightG, waistMM: local.waistMM, note: local.note, operation: mutation.operation, serverID: nil, expectedVersion: 0)

        try await coordinator.captureConflicts([mutation.id], mutations: [mutation], requests: [request], cached: [local], context: context, accessToken: "access-token")
        try context.save()

        let conflicts = try context.fetch(FetchDescriptor<SyncConflict>())
        XCTAssertEqual(local.serverID, "remote-id")
        XCTAssertEqual(local.syncState, "conflict")
        XCTAssertEqual(conflicts.first?.remoteVersion, 3)
    }

    private func remoteMeasurement() -> APIClient.MeasurementChange {
        APIClient.MeasurementChange(id: "remote-id", recordedOn: "2026-09-03", weightG: 75_600, waistMM: 800, note: "云端备注", source: "manual", healthkitUuid: nil, version: 3, deletedAt: nil, updatedAt: "2026-09-03T08:00:00.000Z")
    }
}

private actor ConflictTransportStub: SyncTransport {
    let remote: APIClient.MeasurementChange

    init(remote: APIClient.MeasurementChange) {
        self.remote = remote
    }

    func measurement(id: String, accessToken: String) async throws -> APIClient.MeasurementChange { remote }

    func measurement(recordedOn: String, accessToken: String) async throws -> APIClient.MeasurementChange { remote }

    func listMeasurementChanges(cursor: String?, accessToken: String) async throws -> APIClient.MeasurementChangePage {
        APIClient.MeasurementChangePage(measurements: [], nextCursor: nil, hasMore: false)
    }

    func upsertMeasurement(date: String, weightG: Int, waistMM: Int?, note: String, mutationID: UUID, accessToken: String) async throws -> APIClient.MeasurementResponse {
        throw APIClient.APIError.rejected("此测试不写入远端记录。")
    }

    func updateMeasurement(id: String, weightG: Int, waistMM: Int?, note: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws -> APIClient.MeasurementResponse {
        throw APIClient.APIError.rejected("此测试不更新远端记录。")
    }

    func deleteMeasurement(id: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws {
        throw APIClient.APIError.rejected("此测试不删除远端记录。")
    }
}
