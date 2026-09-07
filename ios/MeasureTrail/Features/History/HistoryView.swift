import SwiftData
import SwiftUI

struct HistoryView: View {
    @Environment(\.modelContext) private var modelContext
    @Query(sort: \CachedMeasurement.recordedOn, order: .reverse) private var measurements: [CachedMeasurement]
    @State private var showingRecord = false
    @State private var selectedFilter = HistoryFilter.all
    @State private var searchText = ""

    private var visibleMeasurements: [CachedMeasurement] { measurements.filter { !$0.isDeleted || $0.syncState == "conflict" } }
    private var filteredMeasurements: [CachedMeasurement] {
        visibleMeasurements.filter { measurement in
            selectedFilter.matches(recordedOn: measurement.recordedOn, waistMM: measurement.waistMM, note: measurement.note) && matchesSearch(measurement)
        }
    }

    var body: some View {
        NavigationStack {
            Group {
                if visibleMeasurements.isEmpty {
                    ContentUnavailableView("还没有历史记录", systemImage: "list.bullet.rectangle", description: Text("每次记录都会在这里沉淀为清晰的时间线。"))
                } else if filteredMeasurements.isEmpty {
                    ContentUnavailableView {
                        Label("没有符合条件的记录", systemImage: "line.3.horizontal.decrease.circle")
                    } description: {
                        Text("可以更换筛选条件或清除搜索内容。")
                    } actions: {
                        Button("清除筛选") {
                            selectedFilter = .all
                            searchText = ""
                        }
                    }
                } else {
                    List {
                        ForEach(filteredMeasurements) { measurement in
                            NavigationLink { MeasurementDetailView(measurement: measurement) } label: { MeasurementRow(measurement: measurement) }
                        }
                        .onDelete(perform: delete)
                    }
                }
            }
            .navigationTitle("历史")
            .searchable(text: $searchText, prompt: "搜索备注、日期或数值")
            .toolbar {
                ToolbarItemGroup(placement: .primaryAction) {
                    Menu {
                        Picker("筛选记录", selection: $selectedFilter) {
                            ForEach(HistoryFilter.allCases) { filter in
                                Label(filter.title, systemImage: filter.symbol).tag(filter)
                            }
                        }
                    } label: {
                        Image(systemName: selectedFilter == .all ? "line.3.horizontal.decrease.circle" : "line.3.horizontal.decrease.circle.fill")
                    }
                    .accessibilityLabel("筛选历史记录")
                    Button { showingRecord = true } label: { Label("补记", systemImage: "plus") }
                }
            }
            .sheet(isPresented: $showingRecord) { RecordSheet() }
        }
    }

    private func delete(at offsets: IndexSet) {
        for index in offsets {
            let measurement = filteredMeasurements[index]
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

    private func matchesSearch(_ measurement: CachedMeasurement) -> Bool {
        let query = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty else { return true }
        let values = [
            measurement.note,
            measurement.recordedOn.formatted(date: .numeric, time: .omitted),
            WeightUnit.display(measurement.weightG),
            measurement.waistMM.map { "\(Double($0) / 10) cm" } ?? "",
        ]
        return values.contains { $0.localizedCaseInsensitiveContains(query) }
    }
}

enum HistoryFilter: String, CaseIterable, Identifiable {
    case all
    case recent30Days
    case withWaist
    case withNote

    var id: Self { self }

    var title: String {
        switch self {
        case .all: "全部记录"
        case .recent30Days: "近 30 天"
        case .withWaist: "有腰围"
        case .withNote: "有备注"
        }
    }

    var symbol: String {
        switch self {
        case .all: "list.bullet"
        case .recent30Days: "calendar"
        case .withWaist: "ruler"
        case .withNote: "note.text"
        }
    }

    func matches(recordedOn: Date, waistMM: Int?, note: String, referenceDate: Date = .now, calendar: Calendar = .current) -> Bool {
        switch self {
        case .all:
            true
        case .recent30Days:
            recordedOn >= (calendar.date(byAdding: .day, value: -29, to: calendar.startOfDay(for: referenceDate)) ?? .distantPast)
        case .withWaist:
            waistMM != nil
        case .withNote:
            !note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        }
    }
}

private struct MeasurementRow: View {
    let measurement: CachedMeasurement
    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            HStack(alignment: .firstTextBaseline) {
                Text(measurement.recordedOn, format: .dateTime.year().month().day()).font(.headline)
                Spacer()
                Text(WeightUnit.display(measurement.weightG)).monospacedDigit().font(.title3.weight(.semibold))
            }
            if let waistMM = measurement.waistMM {
                Label("腰围 \(Double(waistMM) / 10, format: .number.precision(.fractionLength(1))) cm", systemImage: "ruler")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            if !measurement.note.isEmpty {
                Text(measurement.note)
                    .font(.subheadline)
                    .foregroundStyle(.primary)
                    .lineLimit(2)
            }
            Text(syncDescription)
                .font(.caption)
                .foregroundStyle(measurement.syncState == "conflict" ? .orange : .secondary)
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
