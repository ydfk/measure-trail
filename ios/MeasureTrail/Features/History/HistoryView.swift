import SwiftData
import SwiftUI

struct HistoryView: View {
    @Environment(\.modelContext) private var modelContext
    @Query(sort: \CachedMeasurement.recordedOn, order: .reverse) private var measurements: [CachedMeasurement]
    @State private var showingRecord = false

    private var visibleMeasurements: [CachedMeasurement] { measurements.filter { !$0.isDeleted || $0.syncState == "conflict" } }

    var body: some View {
        NavigationStack {
            Group {
                if visibleMeasurements.isEmpty {
                    ContentUnavailableView("还没有历史记录", systemImage: "list.bullet.rectangle", description: Text("每次记录都会在这里沉淀为清晰的时间线。"))
                } else {
                    List {
                        ForEach(visibleMeasurements) { measurement in
                            NavigationLink { MeasurementDetailView(measurement: measurement) } label: { MeasurementRow(measurement: measurement) }
                        }
                        .onDelete(perform: delete)
                    }
                }
            }
            .navigationTitle("历史")
            .toolbar { ToolbarItem(placement: .primaryAction) { Button { showingRecord = true } label: { Label("补记", systemImage: "plus") } } }
            .sheet(isPresented: $showingRecord) { RecordSheet() }
        }
    }

    private func delete(at offsets: IndexSet) {
        for index in offsets {
            let measurement = visibleMeasurements[index]
            if measurement.syncState == "pending" {
                let measurementID = measurement.id
                let descriptor = FetchDescriptor<PendingMutation>(predicate: #Predicate { $0.measurementID == measurementID })
                for mutation in (try? modelContext.fetch(descriptor)) ?? [] { modelContext.delete(mutation) }
                modelContext.delete(measurement)
            } else {
                measurement.isDeleted = true
                measurement.syncState = "pendingDelete"
                modelContext.insert(PendingMutation(operation: "deleteMeasurement", measurementID: measurement.id, recordedOn: measurement.recordedOn, expectedVersion: measurement.version))
            }
        }
        try? modelContext.save()
    }
}

private struct MeasurementRow: View {
    let measurement: CachedMeasurement
    var body: some View {
        HStack {
            VStack(alignment: .leading) { Text(measurement.recordedOn, format: .dateTime.year().month().day()).font(.headline); Text(syncDescription).font(.caption).foregroundStyle(measurement.syncState == "conflict" ? .orange : .secondary) }
            Spacer()
            Text(WeightUnit.display(measurement.weightG)).monospacedDigit().font(.title3.weight(.semibold))
        }
        .accessibilityElement(children: .combine)
    }

    private var syncDescription: String {
        switch measurement.syncState {
        case "synced": "已同步"
        case "conflict": "需要处理同步冲突"
        default: "等待同步"
        }
    }
}

private struct MeasurementDetailView: View {
    let measurement: CachedMeasurement
    @Environment(\.modelContext) private var modelContext
    @State private var showingEditor = false
    @State private var showingConflictResolution = false

    private var conflict: SyncConflict? {
        let measurementID = measurement.id
        let descriptor = FetchDescriptor<SyncConflict>(predicate: #Predicate { $0.measurementID == measurementID })
        return try? modelContext.fetch(descriptor).first
    }

    var body: some View {
        Form {
            if conflict != nil {
                Section("同步冲突") {
                    Text("另一台设备已更新此记录。请决定保留哪一份内容后再继续同步。")
                    Button("处理冲突") { showingConflictResolution = true }
                }
            }
            Section("体重") { Text(WeightUnit.display(measurement.weightG)) }
            if let waist = measurement.waistMM { Section("腰围") { Text("\(Double(waist) / 10, format: .number.precision(.fractionLength(1))) cm") } }
            if !measurement.note.isEmpty { Section("备注") { Text(measurement.note) } }
        }
        .navigationTitle(Text(measurement.recordedOn, format: .dateTime.year().month().day()))
        .toolbar { ToolbarItem(placement: .primaryAction) { Button("编辑") { showingEditor = true }.disabled(conflict != nil) } }
        .sheet(isPresented: $showingEditor) { RecordSheet(measurement: measurement) }
        .sheet(isPresented: $showingConflictResolution) {
            if let conflict { ConflictResolutionSheet(measurement: measurement, conflict: conflict) }
        }
    }
}

private struct ConflictResolutionSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.modelContext) private var modelContext
    let measurement: CachedMeasurement
    let conflict: SyncConflict

    var body: some View {
        NavigationStack {
            List {
                Section("本机修改") { measurementSummary(weightG: measurement.weightG, waistMM: measurement.waistMM, note: measurement.note, deleted: measurement.isDeleted) }
                Section("云端版本") { measurementSummary(weightG: conflict.remoteWeightG, waistMM: conflict.remoteWaistMM, note: conflict.remoteNote, deleted: conflict.remoteDeletedAt != nil) }
                Section {
                    Button("采用云端版本") { acceptRemote() }
                    Button("保留本机修改", role: .destructive) { keepLocal() }
                } footer: {
                    Text("保留本机修改会以云端的最新版本为基准重新同步，不会无提示覆盖其他设备的数据。")
                }
            }
            .navigationTitle("处理同步冲突")
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("稍后处理") { dismiss() } } }
        }
    }

    @ViewBuilder
    private func measurementSummary(weightG: Int, waistMM: Int?, note: String, deleted: Bool) -> some View {
        if deleted {
            Text("此版本已删除")
        } else {
            Text(WeightUnit.display(weightG))
            if let waistMM { Text("腰围 \(Double(waistMM) / 10, format: .number.precision(.fractionLength(1))) cm") }
            if !note.isEmpty { Text(note) }
        }
    }

    private func acceptRemote() {
        removePendingMutations()
        switch SyncConflictResolution.actionForAcceptingRemote(remoteWasDeleted: conflict.remoteDeletedAt != nil) {
        case .removeLocalMeasurement:
            modelContext.delete(measurement)
        case .applyRemoteMeasurement:
            measurement.weightG = conflict.remoteWeightG
            measurement.waistMM = conflict.remoteWaistMM
            measurement.note = conflict.remoteNote
            measurement.version = conflict.remoteVersion
            measurement.isDeleted = false
            measurement.syncState = "synced"
        default:
            assertionFailure("采用云端版本的冲突决策无效。")
            return
        }
        modelContext.delete(conflict)
        try? modelContext.save()
        dismiss()
    }

    private func keepLocal() {
        removePendingMutations()
        switch SyncConflictResolution.actionForKeepingLocal(operation: conflict.operation, remoteWasDeleted: conflict.remoteDeletedAt != nil, remoteVersion: conflict.remoteVersion) {
        case .removeLocalMeasurement:
            modelContext.delete(measurement)
        case let .queueDelete(expectedVersion):
            measurement.version = expectedVersion
            measurement.isDeleted = true
            measurement.syncState = "pendingDelete"
            modelContext.insert(PendingMutation(operation: "deleteMeasurement", measurementID: measurement.id, recordedOn: measurement.recordedOn, expectedVersion: expectedVersion))
        case let .queueUpsert(expectedVersion):
            if let expectedVersion {
                measurement.version = expectedVersion
            } else {
                measurement.serverID = nil
                measurement.version = 0
            }
            measurement.isDeleted = false
            measurement.syncState = "pending"
            modelContext.insert(PendingMutation(operation: "upsertMeasurement", measurementID: measurement.id, recordedOn: measurement.recordedOn, expectedVersion: expectedVersion ?? 0))
        default:
            assertionFailure("保留本机修改的冲突决策无效。")
            return
        }
        modelContext.delete(conflict)
        try? modelContext.save()
        Task { await SyncCoordinator.shared.synchronize(context: modelContext) }
        dismiss()
    }

    private func removePendingMutations() {
        let measurementID = measurement.id
        let descriptor = FetchDescriptor<PendingMutation>(predicate: #Predicate { $0.measurementID == measurementID })
        for item in (try? modelContext.fetch(descriptor)) ?? [] { modelContext.delete(item) }
    }
}
