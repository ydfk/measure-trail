import Charts
import SwiftData
import SwiftUI

struct InsightsView: View {
    enum Metric: String, CaseIterable, Identifiable { case weight = "体重", waist = "腰围"; var id: Self { self } }

    enum Range: Hashable, Identifiable {
        case days(Int)
        case all
        case custom

        static let presets: [Range] = [.days(7), .days(14), .days(30), .days(60), .days(90), .days(180), .days(365), .all]

        var id: String {
            switch self {
            case let .days(days): "days-\(days)"
            case .all: "all"
            case .custom: "custom"
            }
        }

        var title: String {
            switch self {
            case let .days(days): "\(days) 天"
            case .all: "全部"
            case .custom: "自定义日期"
            }
        }
    }

    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @Environment(\.verticalSizeClass) private var verticalSizeClass
    @Query(sort: \CachedMeasurement.recordedOn) private var measurements: [CachedMeasurement]
    @State private var metric: Metric = .weight
    @State private var range: Range = .days(30)
    @State private var customStart = Calendar.current.date(byAdding: .day, value: -29, to: .now) ?? .now
    @State private var customEnd = Date.now
    @State private var selectedDate: Date?
    @State private var showingCustomRange = false

    private var usesExpandedLayout: Bool { horizontalSizeClass == .regular || verticalSizeClass == .compact }
    private var points: [CachedMeasurement] {
        measurements.filter {
            !$0.isDeleted && $0.recordedOn >= dateInterval.start && $0.recordedOn < dateInterval.end && (metric == .weight || $0.waistMM != nil)
        }
    }

    private var dateInterval: DateInterval {
        let calendar = Calendar.current
        let end = calendar.date(byAdding: .day, value: 1, to: calendar.startOfDay(for: range == .custom ? customEnd : .now)) ?? .distantFuture
        switch range {
        case let .days(days):
            let start = calendar.date(byAdding: .day, value: -(days - 1), to: calendar.startOfDay(for: .now)) ?? .distantPast
            return DateInterval(start: start, end: end)
        case .all:
            return DateInterval(start: .distantPast, end: end)
        case .custom:
            return DateInterval(start: calendar.startOfDay(for: customStart), end: end)
        }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                if usesExpandedLayout, !points.isEmpty {
                    HStack(alignment: .top, spacing: 28) {
                        VStack(alignment: .leading, spacing: 18) {
                            controls
                            selectionAndSummary
                        }
                        .frame(maxWidth: 260, alignment: .leading)
                        trendChart
                            .frame(maxWidth: .infinity)
                    }
                    .padding()
                } else {
                    VStack(alignment: .leading, spacing: 20) {
                        controls
                        if points.isEmpty {
                            emptyState
                        } else {
                            trendChart
                            selectionAndSummary
                        }
                    }
                    .padding()
                }
            }
            .navigationTitle("趋势")
        }
        .sheet(isPresented: $showingCustomRange) { customRangeSheet }
        .onChange(of: metric) { _, _ in selectedDate = nil }
        .onChange(of: range) { _, _ in selectedDate = nil }
    }

    private var controls: some View {
        VStack(alignment: .leading, spacing: 12) {
            Picker("指标", selection: $metric) {
                ForEach(Metric.allCases) { Text($0.rawValue).tag($0) }
            }
            .pickerStyle(.segmented)

            Menu {
                Section("预设范围") {
                    ForEach(Range.presets) { option in
                        Button {
                            range = option
                        } label: {
                            if range == option {
                                Label(option.title, systemImage: "checkmark")
                            } else {
                                Text(option.title)
                            }
                        }
                    }
                }
                Button("自定义日期…", systemImage: "calendar") { showingCustomRange = true }
            } label: {
                Label(rangeLabel, systemImage: "calendar")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.bordered)
            .controlSize(.large)
            .accessibilityLabel("趋势时间范围：\(rangeLabel)")
        }
    }

    private var rangeLabel: String {
        guard range == .custom else { return range.title }
        return "\(customStart.formatted(.dateTime.month().day())) 至 \(customEnd.formatted(.dateTime.month().day()))"
    }

    private var emptyState: some View {
        ContentUnavailableView(
            metric == .waist ? "还没有腰围趋势" : "还没有趋势",
            systemImage: "chart.line.uptrend.xyaxis",
            description: Text(metric == .waist ? "腰围会在你记录后显示；缺失数据不会被当作 0。" : "在这个时间范围内记录体重后，会在这里呈现真实的变化。")
        )
        .frame(minHeight: usesExpandedLayout ? 320 : 280)
        .frame(maxWidth: .infinity)
    }

    private var trendChart: some View {
        Chart(points) { point in
            LineMark(x: .value("日期", point.recordedOn), y: .value(metric.rawValue, value(of: point)))
                .foregroundStyle(MeasureTrailStyle.blue)
                .interpolationMethod(.catmullRom)
            PointMark(x: .value("日期", point.recordedOn), y: .value(metric.rawValue, value(of: point)))
                .foregroundStyle(MeasureTrailStyle.ink)
        }
        .chartXSelection(value: $selectedDate)
        .chartYAxisLabel(metric == .weight ? WeightUnit.label : "cm")
        .frame(height: usesExpandedLayout ? 340 : 260)
        .accessibilityLabel("\(rangeLabel)\(metric.rawValue)趋势图")
    }

    private var selectionAndSummary: some View {
        VStack(alignment: .leading, spacing: 10) {
            if let selected = closestPoint {
                Label("\(selected.recordedOn, format: .dateTime.year().month().day())：\(displayValue(selected))", systemImage: "calendar")
                    .font(.callout)
                    .accessibilityLabel("选中记录：\(selected.recordedOn.formatted(date: .long, time: .omitted))，\(displayValue(selected))")
            }
            Text(summary)
                .font(.callout)
                .foregroundStyle(.secondary)
        }
    }

    private var customRangeSheet: some View {
        NavigationStack {
            Form {
                DatePicker("开始日期", selection: $customStart, in: ...customEnd, displayedComponents: .date)
                DatePicker("结束日期", selection: $customEnd, in: customStart...Date.now, displayedComponents: .date)
            }
            .navigationTitle("自定义趋势范围")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { showingCustomRange = false } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("完成") {
                        range = .custom
                        showingCustomRange = false
                    }
                }
            }
        }
    }

    private var closestPoint: CachedMeasurement? {
        guard let selectedDate else { return points.last }
        return points.min { abs($0.recordedOn.timeIntervalSince(selectedDate)) < abs($1.recordedOn.timeIntervalSince(selectedDate)) }
    }

    private func value(of measurement: CachedMeasurement) -> Double {
        metric == .weight ? WeightUnit.value(fromGrams: measurement.weightG) : Double(measurement.waistMM ?? 0) / 10
    }

    private func displayValue(_ measurement: CachedMeasurement) -> String {
        "\(value(of: measurement).formatted(.number.precision(.fractionLength(1)))) \(metric == .weight ? WeightUnit.label : "cm")"
    }

    private var summary: String {
        guard let first = points.first, let last = points.last else { return "" }
        let change = value(of: last) - value(of: first)
        let direction = change == 0 ? "保持不变" : (change > 0 ? "增加" : "减少")
        return "此范围内记录的\(metric.rawValue)较第一条\(direction) \(abs(change).formatted(.number.precision(.fractionLength(1)))) \(metric == .weight ? WeightUnit.label : "cm")。"
    }
}
