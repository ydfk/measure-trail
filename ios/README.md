# MeasureTrail iOS

`MeasureTrail.xcodeproj` is an iOS 26 SwiftUI application. It uses the approved Icon Composer document, system navigation and accessibility semantics, and keeps session tokens in Keychain.

For local development, set the `apiBaseURL` UserDefaults value to a reachable HTTPS deployment before logging in. The app intentionally has no built-in production server address and does not bypass failed authentication.
