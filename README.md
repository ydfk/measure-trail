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
| Repository | GitHub and Gitea remotes configured |
| iOS app | SwiftUI iOS 26 app builds for the simulator; authentication, offline cache/outbox, dashboard notes, history filters, credential changes, trends, conflict handling, and automatic server configuration are implemented |
| Backend | Go API, SQLite migrations, authentication, data APIs, and static Docker configuration are implemented |
| Web app | Reserved for a future phase; not implemented |

## Scope

- Record a daily weight, with optional waist measurement and note.
- Provide trends, goals, BMI, history, and mobile-appropriate insights.
- Use username/password authentication with a bootstrapped default account, optional Sign in with Apple for linked identities, and future multi-client sync. Public registration and email-based recovery are disabled.
- Offer optional HealthKit integration, with the user retaining control over reading and writing health data.
- Exclude CSV import and reminders from the current product scope.

## Repository layout

```text
measure-trail/
├── backend/       # Go API, SQLite migrations, and Docker deployment files
├── ios/           # Native SwiftUI application for iOS 26
├── web/           # Future web client placeholder
├── docs/          # Product plan and development conventions
├── .env.example   # Minimal local-development configuration; contains no usable secrets
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

The iOS app selects its server automatically: Debug simulators use `http://localhost:21000`, while devices and Release builds use `https://measure-trail.ydfk.site`; `MEASURETRAIL_API_BASE_URL` lives in the Xcode target Build Settings. The backend bootstraps `admin` / `111111` unless first-run credentials are supplied through environment variables, and users can change them from account settings. A single Go process serves both `/api` and the future Vue/React static build copied into the Docker image. A production Compose example and secure environment generator are included. Final HTTPS deployment, HealthKit read/write, Sign in with Apple on device, cross-device conflict, and App Store release gates remain open.
