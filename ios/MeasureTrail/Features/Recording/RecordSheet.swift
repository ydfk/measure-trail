import SwiftData
import SwiftUI

struct RecordSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.modelContext) private var modelContext
    private let editingMeasurement: CachedMeasurement?
    @State private var recordedOn: Date
    @State private var weight: String
    @State private var waist: String
    @State private var note: String
    @State private var errorMessage: String?

    init(measurement: CachedMeasurement? = nil, initialDate: Date = .now) {
        editingMeasurement = measurement
        let day = Calendar.current.startOfDay(for: measurement?.recordedOn ?? initialDate)
        _recordedOn = State(initialValue: day)
        _weight = State(initialValue: measurement.map { WeightUnit.inputText($0.weightG) } ?? "")
        _waist = State(initialValue: measurement?.waistMM.map { Double($0).formatted(.number.precision(.fractionLength(1))) } ?? "")
        _note = State(initialValue: measurement?.note ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("记录日期") {
                    if editingMeasurement == nil {
                        DatePicker("日期", selection: $recordedOn, in: ...Date(), displayedComponents: .date)
                    } else {
                        Text(recordedOn, format: .dateTime.year().month().day().weekday())
                        Text("如需记录其他日期，请新建一条补记。")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
                Section("数据") {
                    TextField("体重（\(WeightUnit.label)）", text: $weight).keyboardType(.decimalPad)
                    TextField("腰围（cm，可选）", text: $waist).keyboardType(.decimalPad)
                    TextField("备注（可选）", text: $note, axis: .vertical).lineLimit(2...5)
                }
                Section {
                    Text("体重会以克、腰围会以毫米安全保存；同一天再次保存会更新当天记录。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                if let errorMessage {
                    Section { Label(errorMessage, systemImage: "exclamationmark.circle").foregroundStyle(.red) }
                }
            }
            .navigationTitle(editingMeasurement == nil ? "记录" : "编辑记录")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("保存") { save() }.disabled(weight.isEmpty) }
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
            }
        }
    }

    private func save() {
        do {
            let input = try MeasurementInput.make(weightText: weight, waistText: waist, note: note)
            let day = Calendar.current.startOfDay(for: recordedOn)
            let nextDay = Calendar.current.date(byAdding: .day, value: 1, to: day)!
            let descriptor = FetchDescriptor<CachedMeasurement>(predicate: #Predicate { $0.recordedOn >= day && $0.recordedOn < nextDay })
            let existing = try modelContext.fetch(descriptor).first(where: { !$0.isDeleted })
            let measurement = editingMeasurement ?? existing ?? CachedMeasurement(recordedOn: day, weightG: input.weightG, waistMM: input.waistMM, note: input.note)
            measurement.weightG = input.weightG
            measurement.waistMM = input.waistMM
            measurement.note = input.note
            measurement.source = "manual"
            measurement.healthKitUUID = nil
            measurement.updatedAt = .now
            measurement.syncState = "pending"
            measurement.isDeleted = false
            if editingMeasurement == nil && existing == nil { modelContext.insert(measurement) }
            let measurementID = measurement.id
            let pending = FetchDescriptor<PendingMutation>(predicate: #Predicate { $0.measurementID == measurementID })
            for mutation in try modelContext.fetch(pending) { modelContext.delete(mutation) }
            modelContext.insert(PendingMutation(operation: "upsertMeasurement", measurementID: measurement.id, recordedOn: day, expectedVersion: measurement.version))
            try modelContext.save()
            Task { await HealthKitService.shared.writeManualMeasurement(measurement) }
            Task { await SyncCoordinator.shared.synchronize(context: modelContext) }
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
