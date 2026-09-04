import Foundation

protocol SyncTransport: Sendable {
    func measurement(id: String, accessToken: String) async throws -> APIClient.MeasurementChange
    func measurement(recordedOn: String, accessToken: String) async throws -> APIClient.MeasurementChange
    func listMeasurementChanges(cursor: String?, accessToken: String) async throws -> APIClient.MeasurementChangePage
    func upsertMeasurement(date: String, weightG: Int, waistMM: Int?, note: String, mutationID: UUID, accessToken: String) async throws -> APIClient.MeasurementResponse
    func updateMeasurement(id: String, weightG: Int, waistMM: Int?, note: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws -> APIClient.MeasurementResponse
    func deleteMeasurement(id: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws
}

extension APIClient: SyncTransport {}

actor SyncEngine {
    struct Request: Sendable {
        let mutationID: UUID
        let recordedOn: Date
        let weightG: Int
        let waistMM: Int?
        let note: String
        let operation: String
        let serverID: String?
        let expectedVersion: Int
    }

    struct Result: Sendable {
        let completed: Set<UUID>
        let upserts: [UUID: APIClient.MeasurementResponse]
        let conflicts: Set<UUID>
    }

    private let transport: any SyncTransport

    init(transport: any SyncTransport = APIClient()) {
        self.transport = transport
    }

    func pullChanges(cursor: String?, accessToken: String) async throws -> APIClient.MeasurementChangePage {
        try await transport.listMeasurementChanges(cursor: cursor, accessToken: accessToken)
    }

    func synchronize(_ requests: [Request], accessToken: String) async -> Result {
        var completed = Set<UUID>()
        var upserts = [UUID: APIClient.MeasurementResponse]()
        var conflicts = Set<UUID>()
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withFullDate]
        for request in requests {
            do {
                if request.operation == "upsertMeasurement" {
                    if let serverID = request.serverID {
                        upserts[request.mutationID] = try await transport.updateMeasurement(id: serverID, weightG: request.weightG, waistMM: request.waistMM, note: request.note, expectedVersion: request.expectedVersion, mutationID: request.mutationID, accessToken: accessToken)
                    } else {
                        upserts[request.mutationID] = try await transport.upsertMeasurement(date: formatter.string(from: request.recordedOn), weightG: request.weightG, waistMM: request.waistMM, note: request.note, mutationID: request.mutationID, accessToken: accessToken)
                    }
                } else if let serverID = request.serverID {
                    try await transport.deleteMeasurement(id: serverID, expectedVersion: request.expectedVersion, mutationID: request.mutationID, accessToken: accessToken)
                } else { continue }
                completed.insert(request.mutationID)
            } catch APIClient.APIError.conflict {
                conflicts.insert(request.mutationID)
            } catch {
                break
            }
        }
        return Result(completed: completed, upserts: upserts, conflicts: conflicts)
    }
}
