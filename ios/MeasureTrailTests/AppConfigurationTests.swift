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
    func testBuildEnvironmentSelectsServerWithoutUserDefaults() {
        XCTAssertEqual(AppConfiguration.resolveAPIBaseURL(environmentOverride: nil, bundledValue: "http://localhost:21000", isDevelopment: true)?.absoluteString, "http://localhost:21000")
        XCTAssertEqual(AppConfiguration.resolveAPIBaseURL(environmentOverride: "https://staging.example.com", bundledValue: "http://localhost:21000", isDevelopment: true)?.host, "staging.example.com")
        XCTAssertEqual(AppConfiguration.resolveAPIBaseURL(environmentOverride: "https://unexpected.example.com", bundledValue: "https://measure-trail.ydfk.site/", isDevelopment: false)?.absoluteString, "https://measure-trail.ydfk.site")
    }

    func testReleaseRejectsMissingAndInsecureServer() {
        XCTAssertNil(AppConfiguration.resolveAPIBaseURL(environmentOverride: "https://override.example.com", bundledValue: nil, isDevelopment: false))
        XCTAssertNil(AppConfiguration.resolveAPIBaseURL(environmentOverride: nil, bundledValue: "http://localhost:21000", isDevelopment: false))
    }

    func testDevelopmentHTTPIsLimitedToLocalHosts() {
        XCTAssertNotNil(AppConfiguration.validatedAPIBaseURL("http://localhost:21000", allowLocalHTTP: true))
        XCTAssertNotNil(AppConfiguration.validatedAPIBaseURL("http://dev-machine.local:21000", allowLocalHTTP: true))
        XCTAssertNil(AppConfiguration.validatedAPIBaseURL("http://public.example.com", allowLocalHTTP: true))
    }

    func testBundleContainsServerAndLaunchScreen() {
        XCTAssertNotNil(AppConfiguration.apiBaseURL)
        XCTAssertEqual(AppConfiguration.passkeyRelyingPartyID, "measure-trail.ydfk.site")
        XCTAssertNotNil(Bundle.main.object(forInfoDictionaryKey: "UILaunchScreen"))
    }
}
