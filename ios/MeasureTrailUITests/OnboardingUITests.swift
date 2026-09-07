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

        XCTAssertTrue(app.textFields["authentication-username"].waitForExistence(timeout: 5))
    }

    @MainActor
    func testLoginHasBrandAndNoRegistrationOrServerSetup() {
        let app = XCUIApplication()
        app.launchArguments = ["-uiTestingResetOnboarding"]
        app.launch()
        app.buttons["继续"].tap()

        XCTAssertTrue(app.images["authentication-logo"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["注册"].exists)
        XCTAssertFalse(app.segmentedControls["账号操作"].exists)
        XCTAssertFalse(app.buttons["设置量迹服务器"].exists)
        XCTAssertFalse(app.buttons["忘记密码？"].exists)
        XCTAssertFalse(app.staticTexts["邮箱"].exists)
        XCTAssertTrue(app.buttons["authentication-submit"].isHittable)
        XCTAssertFalse(app.buttons["authentication-submit"].isEnabled)

        let username = app.textFields["authentication-username"]
        XCTAssertEqual(username.value as? String, "admin")
        let password = app.secureTextFields["authentication-password"]
        password.tap()
        password.typeText("111111")
        XCTAssertTrue(app.buttons["authentication-submit"].isEnabled)
    }
}
