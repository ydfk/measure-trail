# MeasureTrail / 量迹

<!-- README-I18N:START -->

**English** | [汉语](./README_zh.md)

<!-- README-I18N:END -->

MeasureTrail is a Chinese-first, privacy-conscious weight-tracking product being rebuilt as a modern native iOS app.

> [!WARNING]
> The project is under active implementation and is not release-ready. The committed plan remains the source of truth for scope and acceptance.

## Status

| Area | Current state |
| --- | --- |
| Product identity | MeasureTrail / 量迹 selected |
| Repository | Local Git repository initialized; no remote configured yet |
| iOS app | SwiftUI iOS 26 app builds for the simulator; authentication, offline cache/outbox, profile, recording, trends, conflict handling, export, and server connection setup are implemented |
| Backend | Go API, SQLite migrations, authentication, data APIs, and static Docker configuration are implemented |
| Web app | Reserved for a future phase; not implemented |

## Scope

- Record a daily weight, with optional waist measurement and note.
- Provide trends, goals, BMI, history, and mobile-appropriate insights.
- Support account registration, email/password authentication, Sign in with Apple, and future multi-client sync.
- Offer optional HealthKit integration, with the user retaining control over reading and writing health data.
- Exclude CSV import and reminders from the current product scope.

## Repository layout

```text
measure-trail/
├── backend/       # Go API, SQLite migrations, and Docker deployment files
├── ios/           # Native SwiftUI application for iOS 26
├── web/           # Future web client placeholder
├── docs/          # Product plan and development conventions
├── .env.example   # Configuration boundary only; contains no usable secrets
├── README.md
└── README_zh.md
```

## Documentation

- [Implementation plan](./docs/plan.md)
- [Development conventions](./docs/development.md)
- [API contract](./docs/api-contract.md)
- [Privacy policy draft](./docs/privacy.md)
- [App Store release checklist](./docs/app-store-checklist.md)

## Legacy data

The supplied `slimtrack.db` stays in the repository root only as local migration input. It is ignored by Git and must not be moved, modified, or committed. Its structure and migration path will be validated in the dedicated migration phase.

## Development

The iOS app asks for a self-hosted HTTPS service address before login and verifies `/api/health` before saving it. Simulator builds and 26 XCTest/UI tests are passing, including local conflict capture for both stale edits and an offline new device's same-day record, complete local cache cleanup on sign-out and account deletion, resolution rules, the HTTPS connection gate, dashboard record-count rendering, and automatic sync when network connectivity returns. Docker smoke validation is complete; final HTTPS deployment, HealthKit read/write, Sign in with Apple on device, cross-device conflict and remaining end-to-end UI tests, and App Store release gates remain open.
