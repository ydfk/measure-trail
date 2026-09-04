import XCTest
@testable import MeasureTrail

final class APIContractTests: XCTestCase {
    func testMeasurementChangeDecodesHealthKitFields() throws {
        let payload = """
        {
          "id": "measurement-1",
          "recordedOn": "2025-09-26",
          "weightG": 76120,
          "waistMm": 810,
          "note": "",
          "source": "healthkit",
          "healthkitUuid": "sample-weight:sample-waist",
          "version": 2,
          "deletedAt": null,
          "updatedAt": "2025-09-26T08:00:00.000Z"
        }
        """.data(using: .utf8)!

        let change = try JSONDecoder().decode(APIClient.MeasurementChange.self, from: payload)

        XCTAssertEqual(change.source, "healthkit")
        XCTAssertEqual(change.healthkitUuid, "sample-weight:sample-waist")
        XCTAssertNil(change.deletedAt)
        XCTAssertEqual(change.weightG, 76_120)
    }

    func testMeasurementChangeAllowsManualRecordWithoutHealthKitUUID() throws {
        let payload = """
        {
          "id": "measurement-2",
          "recordedOn": "2025-09-27",
          "weightG": 76000,
          "waistMm": null,
          "note": "晨起",
          "source": "manual",
          "version": 3,
          "deletedAt": "2025-09-27T08:00:00.000Z",
          "updatedAt": "2025-09-27T08:00:00.000Z"
        }
        """.data(using: .utf8)!

        let change = try JSONDecoder().decode(APIClient.MeasurementChange.self, from: payload)

        XCTAssertEqual(change.source, "manual")
        XCTAssertNil(change.healthkitUuid)
        XCTAssertNotNil(change.deletedAt)
    }

    func testCreateMeasurementMapsConflictResponseToConflictError() {
        let body = #"{"detail":"记录已在其他设备更新"}"#.data(using: .utf8)!
        let client = APIClient(baseURL: URL(string: "https://measuretrail.example.com")!)

        let error = client.responseError(body, statusCode: 409, fallback: "记录尚未同步。")

        guard case let .conflict(message) = error else {
            return XCTFail("409 应映射为冲突错误，实际为 \(error)")
        }
        XCTAssertEqual(message, "记录已在其他设备更新")
    }
}
