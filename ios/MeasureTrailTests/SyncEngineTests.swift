import XCTest
@testable import MeasureTrail

final class SyncEngineTests: XCTestCase {
    func testRetryableFailureStopsOutboxAndLeavesLaterMutationsPending() async {
        let first = UUID()
        let second = UUID()
        let transport = SyncTransportStub(updateOutcomes: [.retryableFailure, .success(id: "server-second", version: 2)])
        let engine = SyncEngine(transport: transport)

        let result = await engine.synchronize([request(mutationID: first), request(mutationID: second)], accessToken: "access-token")

        XCTAssertEqual(result.completed, [])
        XCTAssertEqual(result.conflicts, [])
        let calls = await transport.updateCallCount()
        XCTAssertEqual(calls, 1)
    }

    func testConflictDoesNotBlockLaterOutboxMutation() async {
        let first = UUID()
        let second = UUID()
        let transport = SyncTransportStub(updateOutcomes: [.conflict, .success(id: "server-second", version: 2)])
        let engine = SyncEngine(transport: transport)

        let result = await engine.synchronize([request(mutationID: first), request(mutationID: second)], accessToken: "access-token")

        XCTAssertEqual(result.completed, [second])
        XCTAssertEqual(result.conflicts, [first])
        XCTAssertEqual(result.upserts[second]?.id, "server-second")
        let calls = await transport.updateCallCount()
        XCTAssertEqual(calls, 2)
    }

    private func request(mutationID: UUID) -> SyncEngine.Request {
        SyncEngine.Request(mutationID: mutationID, recordedOn: Date(timeIntervalSince1970: 0), weightG: 76_000, waistMM: nil, note: "", operation: "upsertMeasurement", serverID: "server-id", expectedVersion: 1)
    }
}

private actor SyncTransportStub: SyncTransport {
    enum UpdateOutcome: Sendable {
        case success(id: String, version: Int)
        case conflict
        case retryableFailure
    }

    private var updateOutcomes: [UpdateOutcome]
    private var calls = 0

    init(updateOutcomes: [UpdateOutcome]) {
        self.updateOutcomes = updateOutcomes
    }

    func measurement(id: String, accessToken: String) async throws -> APIClient.MeasurementChange {
        throw APIClient.APIError.rejected("此测试不读取单条记录。")
    }

    func measurement(recordedOn: String, accessToken: String) async throws -> APIClient.MeasurementChange {
        throw APIClient.APIError.rejected("此测试不按日期读取记录。")
    }

    func listMeasurementChanges(cursor: String?, accessToken: String) async throws -> APIClient.MeasurementChangePage {
        APIClient.MeasurementChangePage(measurements: [], nextCursor: nil, hasMore: false)
    }

    func upsertMeasurement(date: String, weightG: Int, waistMM: Int?, note: String, mutationID: UUID, accessToken: String) async throws -> APIClient.MeasurementResponse {
        try nextUpdateResult()
    }

    func updateMeasurement(id: String, weightG: Int, waistMM: Int?, note: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws -> APIClient.MeasurementResponse {
        try nextUpdateResult()
    }

    func deleteMeasurement(id: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws {
        throw APIClient.APIError.rejected("此测试不发送删除请求。")
    }

    func updateCallCount() -> Int { calls }

    private func nextUpdateResult() throws -> APIClient.MeasurementResponse {
        calls += 1
        guard !updateOutcomes.isEmpty else { throw APIClient.APIError.rejected("测试结果耗尽。") }
        switch updateOutcomes.removeFirst() {
        case let .success(id, version): return APIClient.MeasurementResponse(id: id, version: version)
        case .conflict: throw APIClient.APIError.conflict("记录已冲突。")
        case .retryableFailure: throw APIClient.APIError.rejected("网络暂不可用。")
        }
    }
}
