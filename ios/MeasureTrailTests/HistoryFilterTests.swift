import XCTest
@testable import MeasureTrail

final class HistoryFilterTests: XCTestCase {
    func testContentFilters() {
        let date = Date(timeIntervalSince1970: 0)

        XCTAssertTrue(HistoryFilter.withWaist.matches(recordedOn: date, waistMM: 810, note: ""))
        XCTAssertFalse(HistoryFilter.withWaist.matches(recordedOn: date, waistMM: nil, note: "晨起"))
        XCTAssertTrue(HistoryFilter.withNote.matches(recordedOn: date, waistMM: nil, note: " 晨起 "))
        XCTAssertFalse(HistoryFilter.withNote.matches(recordedOn: date, waistMM: 810, note: "  "))
    }

    func testRecentThirtyDaysUsesCalendarDays() throws {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = try XCTUnwrap(TimeZone(secondsFromGMT: 0))
        let referenceDate = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 9, day: 7, hour: 18)))
        let firstIncludedDate = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 8, day: 9)))
        let excludedDate = try XCTUnwrap(calendar.date(from: DateComponents(year: 2026, month: 8, day: 8, hour: 23, minute: 59)))

        XCTAssertTrue(HistoryFilter.recent30Days.matches(recordedOn: firstIncludedDate, waistMM: nil, note: "", referenceDate: referenceDate, calendar: calendar))
        XCTAssertFalse(HistoryFilter.recent30Days.matches(recordedOn: excludedDate, waistMM: nil, note: "", referenceDate: referenceDate, calendar: calendar))
    }

    func testHistoryPaginationLoadsThirtyMoreRecordsAtATime() {
        XCTAssertEqual(HistoryPagination.nextVisibleCount(current: 30, total: 95), 60)
        XCTAssertEqual(HistoryPagination.nextVisibleCount(current: 90, total: 95), 95)
    }
}
