import Charts
import SwiftData
import SwiftUI

struct InsightsView: View {
    enum Metric: String, CaseIterable, Identifiable { case weight = "体重", waist = "腰围"; var id: Self { self } }
    enum Range: String, CaseIterable, Identifiable { case week = "7 天", month = "30 天", quarter = "90 天", all = "全部"; var id: Self { self } }
    @Query(sort: \CachedMeasurement.recordedOn) private var measurements: [CachedMeasurement]
    @State private var metric: Metric = .weight
    @State private var range: Range = .month
    @State private var selectedDate: Date?

    private var points: [CachedMeasurement] { measurements.filter { !$0.isDeleted && $0.recordedOn >= startDate && (metric == .weight || $0.waistMM != nil) } }
    private var startDate: Date {
        let calendar = Calendar.current
        switch range {
        case .week: return calendar.date(byAdding: .day, value: -6, to: .now)!
        case .month: return calendar.date(byAdding: .day, value: -29, to: .now)!
        case .quarter: return calendar.date(byAdding: .day, value: -89, to: .now)!
        case .all: return Date.distantPast
        }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    Picker("指标", selection: $metric) { ForEach(Metric.allCases) { Text($0.rawValue).tag($0) } }.pickerStyle(.segmented)
                    Picker("范围", selection: $range) { ForEach(Range.allCases) { Text($0.rawValue).tag($0) } }.pickerStyle(.segmented)
                    if points.isEmpty {
                        ContentUnavailableView(metric == .waist ? "还没有腰围趋势" : "还没有趋势", systemImage: "chart.line.uptrend.xyaxis", description: Text(metric == .waist ? "腰围会在你记录后显示；缺失数据不会被当作 0。" : "记录更多体重后，会在这里呈现真实的变化。"))
                            .frame(minHeight: 280)
                    } else {
                        Chart(points) { point in
                            LineMark(x: .value("日期", point.recordedOn), y: .value(metric.rawValue, value(of: point))).foregroundStyle(MeasureTrailStyle.blue).interpolationMethod(.catmullRom)
                            PointMark(x: .value("日期", point.recordedOn), y: .value(metric.rawValue, value(of: point))).foregroundStyle(MeasureTrailStyle.ink)
                        }
                        .chartXSelection(value: $selectedDate).chartYAxisLabel(metric == .weight ? WeightUnit.label : "cm").frame(height: 260)
                        if let selected = closestPoint { Label("\(selected.recordedOn, format: .dateTime.year().month().day())：\(displayValue(selected))", systemImage: "calendar").font(.callout).accessibilityLabel("选中记录：\(selected.recordedOn.formatted(date: .long, time: .omitted))，\(displayValue(selected))") }
                        Text(summary).font(.callout).foregroundStyle(.secondary)
                    }
                }.padding()
            }.navigationTitle("趋势")
        }
    }

    private var closestPoint: CachedMeasurement? { guard let selectedDate else { return points.last }; return points.min { abs($0.recordedOn.timeIntervalSince(selectedDate)) < abs($1.recordedOn.timeIntervalSince(selectedDate)) } }
    private func value(of measurement: CachedMeasurement) -> Double { metric == .weight ? WeightUnit.value(fromGrams: measurement.weightG) : Double(measurement.waistMM ?? 0) / 10 }
    private func displayValue(_ measurement: CachedMeasurement) -> String { "\(value(of: measurement).formatted(.number.precision(.fractionLength(1)))) \(metric == .weight ? WeightUnit.label : "cm")" }
    private var summary: String { guard let first = points.first, let last = points.last else { return "" }; let change = value(of: last) - value(of: first); let direction = change == 0 ? "保持不变" : (change > 0 ? "增加" : "减少"); return "此范围内记录的\(metric.rawValue)较第一条\(direction) \(abs(change).formatted(.number.precision(.fractionLength(1)))) \(metric == .weight ? WeightUnit.label : "cm")。" }
}
