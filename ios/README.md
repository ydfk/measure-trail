# MeasureTrail iOS

`MeasureTrail.xcodeproj` is an iOS 26 SwiftUI application. It uses the approved Icon Composer document, system navigation and accessibility semantics, and keeps session tokens in Keychain.

API addresses live in the MeasureTrail target Build Settings inside `MeasureTrail.xcodeproj/project.pbxproj`. Debug simulators use `http://localhost:21000`; Debug devices and Release builds use `https://measure-trail.ydfk.site`. `Configuration/Info.plist` embeds the value and `App/AppConfiguration.swift` validates it. Override `MEASURETRAIL_API_BASE_URL` at build time for a test deployment, or in the Debug scheme environment for local development. Release builds require HTTPS and ignore runtime overrides and legacy UserDefaults addresses.

The app signs in with username and password or Passkey; the username field starts empty. Public registration, email input, email password recovery, and Sign in with Apple are not available in the app. The dashboard shows the latest note, history exposes waist and note values with filtering, and account settings can change the username or password. See [development configuration](../docs/development.md) for local network and default-account setup.
