import XCTest

final class OnboardingUITests: XCTestCase {
    @MainActor
    func testInitialOnboardingContinuesToAuthentication() {
        let app = XCUIApplication()
        app.launchArguments = ["-uiTestingResetOnboarding"]
        app.launch()

        XCTAssertTrue(app.staticTexts["欢迎来到量迹"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["继续"].isHittable)

        app.buttons["继续"].tap()

        XCTAssertTrue(app.textFields["authentication-email"].waitForExistence(timeout: 5))
    }

    @MainActor
    func testServerSettingsRejectsNonHTTPSAddressBeforeNetworkRequest() {
        let app = XCUIApplication()
        app.launchArguments = ["-uiTestingResetOnboarding"]
        app.launch()

        app.buttons["继续"].tap()
        XCTAssertTrue(app.buttons["设置量迹服务器"].waitForExistence(timeout: 5))

        app.buttons["设置量迹服务器"].tap()
        let address = app.textFields["server-address"]
        XCTAssertTrue(address.waitForExistence(timeout: 5))
        address.tap()
        address.typeText("http://measuretrail.example.com")
        app.buttons["连接并保存"].tap()

        XCTAssertTrue(app.staticTexts["请输入有效的 HTTPS 基础地址，且不要包含路径、查询参数或账号信息。"].waitForExistence(timeout: 5))
    }

    @MainActor
    func testSignInStaysDisabledUntilHTTPSServiceIsConnected() {
        let app = XCUIApplication()
        app.launchArguments = ["-uiTestingResetOnboarding"]
        app.launch()

        app.buttons["继续"].tap()
        XCTAssertTrue(app.staticTexts["请先设置并验证你的量迹服务器地址。"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["authentication-submit"].isEnabled)
    }
}
