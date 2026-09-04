import Foundation
import SwiftData

@MainActor
final class SyncCoordinator {
    static let shared = SyncCoordinator()
    private let engine: SyncEngine
    private let transport: any SyncTransport

    init(engine: SyncEngine = SyncEngine(), transport: any SyncTransport = APIClient()) {
        self.engine = engine
        self.transport = transport
    }

    func synchronize(context: ModelContext) async {
        guard let session = TokenStore().session() else { return }
        let mutations = (try? context.fetch(FetchDescriptor<PendingMutation>(sortBy: [SortDescriptor(\PendingMutation.createdAt)]))) ?? []
        let cached = (try? context.fetch(FetchDescriptor<CachedMeasurement>())) ?? []
        let requests = mutations.compactMap { mutation -> SyncEngine.Request? in
            guard let measurement = cached.first(where: { $0.id == mutation.measurementID }) else { return nil }
            return SyncEngine.Request(mutationID: mutation.id, recordedOn: measurement.recordedOn, weightG: measurement.weightG, waistMM: measurement.waistMM, note: measurement.note, operation: mutation.operation, serverID: measurement.serverID, expectedVersion: mutation.expectedVersion)
        }
        let result = await engine.synchronize(requests, accessToken: session.accessToken)
        for mutation in mutations where result.completed.contains(mutation.id) { context.delete(mutation) }
        for request in requests where result.completed.contains(request.mutationID) {
            guard let measurement = cached.first(where: { $0.id == mutations.first(where: { $0.id == request.mutationID })?.measurementID }) else { continue }
            if let remote = result.upserts[request.mutationID] { measurement.serverID = remote.id; measurement.version = remote.version }
        }
        let synchronizedDates = Set(requests.filter { result.completed.contains($0.mutationID) && $0.operation == "upsertMeasurement" }.map(\.recordedOn))
        for measurement in cached where synchronizedDates.contains(measurement.recordedOn) { measurement.syncState = "synced" }
        do {
            try await captureConflicts(result.conflicts, mutations: mutations, requests: requests, cached: cached, context: context, accessToken: session.accessToken)
            try context.save()
            try await pullChanges(context: context, accessToken: session.accessToken)
        } catch {
            return
        }
    }

    func captureConflicts(_ mutationIDs: Set<UUID>, mutations: [PendingMutation], requests: [SyncEngine.Request], cached: [CachedMeasurement], context: ModelContext, accessToken: String) async throws {
        for mutationID in mutationIDs {
            guard let request = requests.first(where: { $0.mutationID == mutationID }),
                  let mutation = mutations.first(where: { $0.id == mutationID }),
                  let local = cached.first(where: { $0.id == mutation.measurementID }) else { continue }
            let remote: APIClient.MeasurementChange
            if let serverID = request.serverID {
                remote = try await transport.measurement(id: serverID, accessToken: accessToken)
            } else {
                remote = try await transport.measurement(recordedOn: dateString(request.recordedOn), accessToken: accessToken)
                local.serverID = remote.id
            }
            let remoteDeletedAt = remote.deletedAt.flatMap(timestamp)
            let measurementID = local.id
            let descriptor = FetchDescriptor<SyncConflict>(predicate: #Predicate { $0.measurementID == measurementID })
            if let conflict = try context.fetch(descriptor).first {
                conflict.operation = mutation.operation
                conflict.remoteWeightG = remote.weightG
                conflict.remoteWaistMM = remote.waistMM
                conflict.remoteNote = remote.note
                conflict.remoteVersion = remote.version
                conflict.remoteDeletedAt = remoteDeletedAt
            } else {
                context.insert(SyncConflict(measurementID: local.id, operation: mutation.operation, remoteWeightG: remote.weightG, remoteWaistMM: remote.waistMM, remoteNote: remote.note, remoteVersion: remote.version, remoteDeletedAt: remoteDeletedAt))
            }
            local.syncState = "conflict"
        }
    }

    private func pullChanges(context: ModelContext, accessToken: String) async throws {
        let checkpoint = try checkpoint(context: context)
        var cursor = checkpoint.measurementCursor
        while true {
            let page = try await engine.pullChanges(cursor: cursor.isEmpty ? nil : cursor, accessToken: accessToken)
            guard !page.hasMore || page.nextCursor != cursor else { throw SyncError.stalledCursor }
            try apply(page.measurements, context: context)
            if let nextCursor = page.nextCursor { checkpoint.measurementCursor = nextCursor }
            try context.save()
            cursor = checkpoint.measurementCursor
            if !page.hasMore { return }
        }
    }

    private func checkpoint(context: ModelContext) throws -> SyncCheckpoint {
        let descriptor = FetchDescriptor<SyncCheckpoint>(predicate: #Predicate { $0.scope == "account" })
        if let existing = try context.fetch(descriptor).first { return existing }
        let created = SyncCheckpoint()
        context.insert(created)
        return created
    }

    private func apply(_ changes: [APIClient.MeasurementChange], context: ModelContext) throws {
        let cached = try context.fetch(FetchDescriptor<CachedMeasurement>())
        let pending = try context.fetch(FetchDescriptor<PendingMutation>())
        let pendingMeasurementIDs = Set(pending.map(\.measurementID))
        for change in changes {
            if let local = cached.first(where: { $0.serverID == change.id }) {
                guard SyncMergePolicy.shouldApplyRemoteChange(localVersion: local.version, hasPendingLocalMutation: pendingMeasurementIDs.contains(local.id), remoteVersion: change.version) else { continue }
                if change.deletedAt != nil {
                    context.delete(local)
                    continue
                }
                try update(local, from: change)
            } else if change.deletedAt == nil {
                context.insert(try measurement(from: change))
            }
        }
    }

    private func measurement(from change: APIClient.MeasurementChange) throws -> CachedMeasurement {
        guard let recordedOn = localDate(change.recordedOn), let updatedAt = timestamp(change.updatedAt) else { throw SyncError.invalidChange }
        let measurement = CachedMeasurement(recordedOn: recordedOn, weightG: change.weightG, waistMM: change.waistMM, note: change.note, source: change.source, healthKitUUID: change.healthkitUuid, version: change.version, serverID: change.id, syncState: "synced")
        measurement.updatedAt = updatedAt
        return measurement
    }

    private func update(_ measurement: CachedMeasurement, from change: APIClient.MeasurementChange) throws {
        guard let updatedAt = timestamp(change.updatedAt) else { throw SyncError.invalidChange }
        measurement.weightG = change.weightG
        measurement.waistMM = change.waistMM
        measurement.note = change.note
        measurement.source = change.source
        measurement.healthKitUUID = change.healthkitUuid
        measurement.version = change.version
        measurement.updatedAt = updatedAt
        measurement.syncState = "synced"
    }

    private func localDate(_ value: String) -> Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withFullDate]
        return formatter.date(from: value)
    }

    private func dateString(_ value: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withFullDate]
        return formatter.string(from: value)
    }

    private func timestamp(_ value: String) -> Date? {
        ISO8601DateFormatter().date(from: value)
    }
}

private enum SyncError: Error { case invalidChange, stalledCursor }
