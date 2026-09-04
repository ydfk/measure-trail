import XCTest
@testable import MeasureTrail

final class AppConfigurationTests: XCTestCase {
    func testAcceptsHTTPSBaseAddress() {
        let url = AppConfiguration.validatedAPIBaseURL(" https://measuretrail.example.com/ ")

        XCTAssertEqual(url?.scheme, "https")
        XCTAssertEqual(url?.host, "measuretrail.example.com")
    }

    func testRejectsNonBaseOrInsecureAddress() {
        for value in [
            "http://measuretrail.example.com",
            "https://measuretrail.example.com/api",
            "https://measuretrail.example.com?debug=true",
            "https://user:password@measuretrail.example.com",
        ] {
            XCTAssertNil(AppConfiguration.validatedAPIBaseURL(value), value)
        }
    }
}
