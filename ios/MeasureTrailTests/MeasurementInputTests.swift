import XCTest
@testable import MeasureTrail

final class MeasurementInputTests: XCTestCase {
    override func tearDown() {
        WeightUnit.save("kg")
        super.tearDown()
    }

    func testJinInputConvertsToGrams() throws {
        WeightUnit.save("jin")

        let input = try MeasurementInput.make(weightText: "152.4", waistText: "78.5", note: "  晨起  ")

        XCTAssertEqual(input.weightG, 76_200)
        XCTAssertEqual(input.waistMM, 785)
        XCTAssertEqual(input.note, "晨起")
    }

    func testKilogramInputConvertsToGrams() throws {
        WeightUnit.save("kg")

        let input = try MeasurementInput.make(weightText: "76.2", waistText: "", note: "")

        XCTAssertEqual(input.weightG, 76_200)
        XCTAssertNil(input.waistMM)
    }

    func testInvalidInputIsRejected() {
        WeightUnit.save("kg")

        XCTAssertThrowsError(try MeasurementInput.make(weightText: "2", waistText: "", note: ""))
        XCTAssertThrowsError(try MeasurementInput.make(weightText: "76", waistText: "9", note: ""))
        XCTAssertThrowsError(try MeasurementInput.make(weightText: "76", waistText: "", note: String(repeating: "a", count: 501)))
    }

    func testDashboardRecordCountUsesActualCount() {
        XCTAssertEqual(DashboardCopy.recordCount(3), "共 3 条记录。趋势只描述变化，不作医疗判断。")
    }
}
