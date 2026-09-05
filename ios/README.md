# MeasureTrail iOS

`MeasureTrail.xcodeproj` is an iOS 26 SwiftUI application. It uses the approved Icon Composer document, system navigation and accessibility semantics, and keeps session tokens in Keychain.

Debug simulator builds use `http://localhost:21000`; device and Release builds use `https://measure-api.ydfk.site`. No server entry is required in the app. Override `MEASURETRAIL_API_BASE_URL` at build time for a test deployment, or in the Debug scheme environment for local development. Release builds require HTTPS and ignore runtime overrides and legacy UserDefaults addresses.

Public registration is temporarily hidden in the app and disabled by default on the backend. Existing email and Apple accounts can still sign in. See [development configuration](../docs/development.md) for local network setup and [deployment](../docs/deployment.md) for the server registration switch.
