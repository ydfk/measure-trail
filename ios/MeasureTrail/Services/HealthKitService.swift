import Foundation
import HealthKit
import Observation
import SwiftData

@Observable @MainActor final class HealthKitService {
    enum Status: Equatable { case unavailable, notRequested, requested, syncing, imported(Int), denied, error(String) }

    static let shared = HealthKitService()

    private static let originMetadataKey = "com.ydfk.measuretrail.origin"
    private static let originMetadataValue = "measuretrail"
    private static let readAuthorizationRequestedKey = "healthKitReadAuthorizationRequested"
    private static let manualWriteEnabledKey = "healthKitManualWriteEnabled"
    private let store = HKHealthStore()
    private(set) var status: Status
    private(set) var hasRequestedReadAuthorization: Bool
    private(set) var isManualWriteEnabled: Bool

    init() {
        let hasRequestedReadAuthorization = UserDefaults.standard.bool(forKey: Self.readAuthorizationRequestedKey)
        self.hasRequestedReadAuthorization = hasRequestedReadAuthorization
        isManualWriteEnabled = UserDefaults.standard.bool(forKey: Self.manualWriteEnabledKey)
        status = HKHealthStore.isHealthDataAvailable()
            ? (hasRequestedReadAuthorization ? .requested : .notRequested)
            : .unavailable
    }

    func requestAuthorization() async {
        guard HKHealthStore.isHealthDataAvailable(), let bodyMass = HKObjectType.quantityType(forIdentifier: .bodyMass), let waist = HKObjectType.quantityType(forIdentifier: .waistCircumference) else { status = .unavailable; return }
        do {
            try await store.requestAuthorization(toShare: [], read: [bodyMass, waist])
            UserDefaults.standard.set(true, forKey: Self.readAuthorizationRequestedKey)
            hasRequestedReadAuthorization = true
            status = .requested
        } catch {
            status = .error("无法请求健康数据权限。")
        }
    }

    func enableManualWrite() async {
        guard HKHealthStore.isHealthDataAvailable(), let bodyMass = HKObjectType.quantityType(forIdentifier: .bodyMass), let waist = HKObjectType.quantityType(forIdentifier: .waistCircumference) else { status = .unavailable; return }
        do {
            try await store.requestAuthorization(toShare: [bodyMass, waist], read: [bodyMass, waist])
            UserDefaults.standard.set(true, forKey: Self.readAuthorizationRequestedKey)
            UserDefaults.standard.set(true, forKey: Self.manualWriteEnabledKey)
            hasRequestedReadAuthorization = true
            isManualWriteEnabled = true
            status = .requested
        } catch {
            status = .error("无法请求 HealthKit 写入权限。")
        }
    }

    func disableManualWrite() {
        UserDefaults.standard.set(false, forKey: Self.manualWriteEnabledKey)
        isManualWriteEnabled = false
    }

    func clearAccountState() {
        UserDefaults.standard.removeObject(forKey: Self.readAuthorizationRequestedKey)
        UserDefaults.standard.removeObject(forKey: Self.manualWriteEnabledKey)
        UserDefaults.standard.removeObject(forKey: "healthKitAnchor.bodyMass")
        UserDefaults.standard.removeObject(forKey: "healthKitAnchor.waist")
        isManualWriteEnabled = false
        hasRequestedReadAuthorization = false
        status = HKHealthStore.isHealthDataAvailable() ? .notRequested : .unavailable
    }

    func writeManualMeasurement(_ measurement: CachedMeasurement) async {
        guard isManualWriteEnabled, HKHealthStore.isHealthDataAvailable(), let bodyMass = HKObjectType.quantityType(forIdentifier: .bodyMass), let waist = HKObjectType.quantityType(forIdentifier: .waistCircumference) else { return }
        let sampleDate = Calendar.current.date(bySettingHour: 12, minute: 0, second: 0, of: measurement.recordedOn) ?? measurement.recordedOn
        let baseIdentifier = measurement.id.uuidString
        do {
            let bodyIdentifier = baseIdentifier + ".bodyMass"
            try await deleteOwnSamples(of: bodyMass, externalIdentifier: bodyIdentifier)
            var samples: [HKQuantitySample] = [HKQuantitySample(type: bodyMass, quantity: HKQuantity(unit: .gram(), doubleValue: Double(measurement.weightG)), start: sampleDate, end: sampleDate, metadata: metadata(externalIdentifier: bodyIdentifier))]
            let waistIdentifier = baseIdentifier + ".waistCircumference"
            try await deleteOwnSamples(of: waist, externalIdentifier: waistIdentifier)
            if let waistMM = measurement.waistMM {
                samples.append(HKQuantitySample(type: waist, quantity: HKQuantity(unit: .meter(), doubleValue: Double(waistMM) / 1_000), start: sampleDate, end: sampleDate, metadata: metadata(externalIdentifier: waistIdentifier)))
            }
            try await store.save(samples)
        } catch {
            status = .error("手工记录未写入 HealthKit。")
        }
    }

    func synchronize(context: ModelContext) async {
        guard HKHealthStore.isHealthDataAvailable(), let bodyMass = HKObjectType.quantityType(forIdentifier: .bodyMass), let waist = HKObjectType.quantityType(forIdentifier: .waistCircumference) else { status = .unavailable; return }
        guard hasRequestedReadAuthorization else { status = .notRequested; return }
        guard let session = TokenStore().session() else { status = .error("本地登录状态已失效。"); return }
        status = .syncing
        do {
            let bodyChanges = try await anchoredSamples(for: bodyMass, key: "bodyMass")
            let waistChanges = try await anchoredSamples(for: waist, key: "waist")
            let affectedDays = Set((bodyChanges.samples + waistChanges.samples)
                .filter { !Self.isMeasureTrailSample($0) }
                .map { Calendar.current.startOfDay(for: $0.endDate) })
            var importedCount = 0
            for day in affectedDays {
                guard let bodySample = try await latestSample(for: bodyMass, on: day) else { continue }
                let waistSample = try await latestSample(for: waist, on: day)
                let weightG = Int(bodySample.quantity.doubleValue(for: .gram()).rounded())
                let waistMM = waistSample.map { Int(($0.quantity.doubleValue(for: .meter()) * 1_000).rounded()) }
                let sampleKey = [bodySample.uuid.uuidString, waistSample?.uuid.uuidString ?? "none"].joined(separator: ":")
                let remote = try await APIClient().importHealthKitMeasurement(recordedOn: dateString(day), weightG: weightG, waistMM: waistMM, healthKitUUID: sampleKey, mutationID: UUID(), accessToken: session.accessToken)
                try apply(remote, context: context)
                importedCount += 1
            }
            saveAnchor(bodyChanges.anchor, key: "bodyMass")
            saveAnchor(waistChanges.anchor, key: "waist")
            try context.save()
            status = .imported(importedCount)
        } catch {
            status = .error("HealthKit 同步暂未完成。")
        }
    }

    private func anchoredSamples(for type: HKQuantityType, key: String) async throws -> (samples: [HKQuantitySample], anchor: HKQueryAnchor?) {
        let anchor = loadAnchor(key: key)
        return try await withCheckedThrowingContinuation { continuation in
            let query = HKAnchoredObjectQuery(type: type, predicate: nil, anchor: anchor, limit: HKObjectQueryNoLimit) { _, samples, _, nextAnchor, error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(returning: (samples?.compactMap { $0 as? HKQuantitySample } ?? [], nextAnchor))
                }
            }
            store.execute(query)
        }
    }

    private func latestSample(for type: HKQuantityType, on day: Date) async throws -> HKQuantitySample? {
        let nextDay = Calendar.current.date(byAdding: .day, value: 1, to: day)!
        let predicate = HKQuery.predicateForSamples(withStart: day, end: nextDay, options: .strictStartDate)
        return try await withCheckedThrowingContinuation { continuation in
            let query = HKSampleQuery(sampleType: type, predicate: predicate, limit: 100, sortDescriptors: [NSSortDescriptor(key: HKSampleSortIdentifierEndDate, ascending: false)]) { _, samples, error in
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(returning: samples?.compactMap { $0 as? HKQuantitySample }.first(where: { !Self.isMeasureTrailSample($0) }))
                }
            }
            store.execute(query)
        }
    }

    private func apply(_ remote: APIClient.MeasurementChange, context: ModelContext) throws {
        guard let recordedOn = parseDate(remote.recordedOn), let updatedAt = ISO8601DateFormatter().date(from: remote.updatedAt) else { return }
        let cached = try context.fetch(FetchDescriptor<CachedMeasurement>())
        if let local = cached.first(where: { $0.serverID == remote.id }) {
            guard local.syncState == "synced", !local.isDeleted else { return }
            local.recordedOn = recordedOn
            local.weightG = remote.weightG
            local.waistMM = remote.waistMM
            local.note = remote.note
            local.source = remote.source
            local.healthKitUUID = remote.healthkitUuid
            local.version = remote.version
            local.updatedAt = updatedAt
            return
        }
        context.insert(CachedMeasurement(recordedOn: recordedOn, weightG: remote.weightG, waistMM: remote.waistMM, note: remote.note, source: remote.source, healthKitUUID: remote.healthkitUuid, version: remote.version, serverID: remote.id, syncState: "synced"))
    }

    private func loadAnchor(key: String) -> HKQueryAnchor? {
        guard let data = UserDefaults.standard.data(forKey: "healthKitAnchor.\(key)") else { return nil }
        return try? NSKeyedUnarchiver.unarchivedObject(ofClass: HKQueryAnchor.self, from: data)
    }

    private func saveAnchor(_ anchor: HKQueryAnchor?, key: String) {
        guard let anchor, let data = try? NSKeyedArchiver.archivedData(withRootObject: anchor, requiringSecureCoding: true) else { return }
        UserDefaults.standard.set(data, forKey: "healthKitAnchor.\(key)")
    }

    private func deleteOwnSamples(of type: HKQuantityType, externalIdentifier: String) async throws {
        let predicate = HKQuery.predicateForObjects(withMetadataKey: HKMetadataKeyExternalUUID, allowedValues: [externalIdentifier])
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            store.deleteObjects(of: type, predicate: predicate) { success, _, error in
                if let error {
                    continuation.resume(throwing: error)
                } else if success {
                    continuation.resume()
                } else {
                    continuation.resume(throwing: HealthKitWriteError.deletionFailed)
                }
            }
        }
    }

    private func metadata(externalIdentifier: String) -> [String: Any] {
        [
            HKMetadataKeyExternalUUID: externalIdentifier,
            HKMetadataKeyWasUserEntered: true,
            HKMetadataKeyTimeZone: TimeZone.current.identifier,
            Self.originMetadataKey: Self.originMetadataValue,
        ]
    }

    private func dateString(_ date: Date) -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: date)
    }

    private func parseDate(_ value: String) -> Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withFullDate]
        return formatter.date(from: value)
    }

    private nonisolated static func isMeasureTrailSample(_ sample: HKSample) -> Bool {
        sample.metadata?["com.ydfk.measuretrail.origin"] as? String == "measuretrail"
    }
}

private enum HealthKitWriteError: Error { case deletionFailed }
