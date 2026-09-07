import SwiftUI
import SwiftData

struct MainTabView: View {
    @Environment(AppModel.self) private var appModel
    @Environment(\.modelContext) private var modelContext
    @State private var showingRecord = false
    @State private var connectivity = ConnectivityMonitor()

    var body: some View {
        TabView {
            DashboardView(showingRecord: $showingRecord).tabItem { Label("概览", systemImage: "chart.line.uptrend.xyaxis") }
            HistoryView().tabItem { Label("历史", systemImage: "calendar") }
            InsightsView().tabItem { Label("趋势", systemImage: "waveform.path.ecg") }
            SettingsView().tabItem { Label("我的", systemImage: "person.crop.circle") }
        }
        .tint(MeasureTrailStyle.accent)
        .sheet(isPresented: $showingRecord) { RecordSheet() }
        .task(id: connectivity.isOnline) {
            guard connectivity.isOnline else { return }
            await SyncCoordinator.shared.synchronize(context: modelContext)
        }
    }
}

private struct DashboardView: View {
    @Binding var showingRecord: Bool
    @Query(sort: \CachedMeasurement.recordedOn) private var measurements: [CachedMeasurement]

    private var activeMeasurements: [CachedMeasurement] { measurements.filter { !$0.isDeleted } }
    private var latest: CachedMeasurement? { activeMeasurements.last }
    private var previous: CachedMeasurement? { activeMeasurements.dropLast().last }
    private var monthStart: Date { Calendar.current.date(byAdding: .day, value: -29, to: .now)! }
    private var monthFirst: CachedMeasurement? { activeMeasurements.first(where: { $0.recordedOn >= monthStart }) }
    private var todayRecord: CachedMeasurement? { activeMeasurements.last(where: { Calendar.current.isDateInToday($0.recordedOn) }) }

    var body: some View {
        NavigationStack {
            Group {
                if let latest {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 16) {
                            Text(todayRecord == nil ? "今天还没有记录" : "今天已记录")
                                .font(.subheadline.weight(.medium))
                                .foregroundStyle(.secondary)
                            VStack(alignment: .leading, spacing: 8) {
                                Text("最近体重").font(.subheadline).foregroundStyle(.secondary)
                                Text(weight(latest)).font(.system(size: 48, weight: .semibold, design: .rounded)).monospacedDigit()
                                Text(latest.recordedOn, format: .dateTime.year().month().day())
                                    .font(.callout)
                                    .foregroundStyle(.secondary)
                                Divider().padding(.vertical, 4)
                                Label {
                                    Text(latest.note.isEmpty ? "这条记录没有备注" : latest.note)
                                        .foregroundStyle(latest.note.isEmpty ? .secondary : .primary)
                                        .lineLimit(3)
                                } icon: {
                                    Image(systemName: "note.text")
                                        .foregroundStyle(MeasureTrailStyle.blue)
                                }
                                .font(.callout)
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .measureTrailActionSurface()

                            HStack(spacing: 12) {
                                DashboardMetric(title: "较上次", value: change(from: previous, to: latest), detail: previous == nil ? "需要两条记录" : "与上一条相比")
                                DashboardMetric(title: "近 30 天", value: change(from: monthFirst, to: latest), detail: monthFirst == nil ? "暂无对比基线" : "从首条至今")
                            }
                            Text(DashboardCopy.recordCount(activeMeasurements.count))
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                            Button(todayRecord == nil ? "记录今天" : "更新今天") { showingRecord = true }
                                .buttonStyle(.borderedProminent)
                                .tint(MeasureTrailStyle.accent)
                                .frame(maxWidth: .infinity)
                        }
                        .padding()
                    }
                } else {
                    ContentUnavailableView {
                        Label("从第一条记录开始", systemImage: "plus.circle")
                    } description: { Text("量迹会在你的节奏里呈现趋势，而不是催促你改变。") } actions: {
                        Button("记录今天") { showingRecord = true }.buttonStyle(.borderedProminent).tint(MeasureTrailStyle.accent)
                    }
                }
            }
            .navigationTitle("概览")
        }
    }

    private func weight(_ measurement: CachedMeasurement) -> String { WeightUnit.display(measurement.weightG) }
    private func change(from first: CachedMeasurement?, to last: CachedMeasurement) -> String {
        guard let first else { return "—" }
        let delta = WeightUnit.value(fromGrams: last.weightG - first.weightG)
        return "\(delta >= 0 ? "+" : "")\(delta.formatted(.number.precision(.fractionLength(1)))) \(WeightUnit.label)"
    }
}

private struct DashboardMetric: View {
    let title: String
    let value: String
    let detail: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.title3.weight(.semibold)).monospacedDigit()
            Text(detail).font(.caption2).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(.quaternary, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
    }
}

private struct SettingsView: View {
    @Environment(AppModel.self) private var appModel
    @Environment(\.modelContext) private var modelContext
    @State private var healthKit = HealthKitService.shared
    @State private var showingDeleteConfirmation = false
    @State private var showingHealthKitWriteConfirmation = false
    @State private var accountMessage: String?
    var body: some View {
        NavigationStack {
            List {
                Section("资料") {
                    NavigationLink { ProfileSettingsView() } label: { Label("资料与目标", systemImage: "target") }
                }
                Section("账号") {
                    NavigationLink { AccountCredentialsView() } label: { Label("用户名与密码", systemImage: "person.badge.key") }
                    NavigationLink { PasskeySettingsView() } label: { Label("Passkey", systemImage: "person.badge.key.fill") }
                    NavigationLink { SessionsView() } label: { Label("登录设备", systemImage: "laptopcomputer.and.iphone") }
                    Button("退出登录", role: .destructive) { Task { await signOut() } }
                    Button("删除账号", role: .destructive) { showingDeleteConfirmation = true }
                }
                Section("HealthKit") {
                    Text("导入会读取健康 App 中当天最新的体重和腰围；手工记录优先，备注、目标和量迹账户状态不会写入健康数据。你可单独选择是否将之后的手工记录写入健康 App。").font(.footnote).foregroundStyle(.secondary)
                    if healthKit.hasRequestedReadAuthorization {
                        Label("已请求 HealthKit 读取权限；授权由健康 App 管理。", systemImage: "checkmark.circle")
                            .foregroundStyle(.secondary)
                    } else {
                        Button("连接 HealthKit") { Task { await healthKit.requestAuthorization() } }
                    }
                    Button("从 HealthKit 导入记录") { Task { await healthKit.synchronize(context: modelContext) } }
                        .disabled(!healthKit.hasRequestedReadAuthorization || healthKit.status == .syncing)
                    Button(healthKit.isManualWriteEnabled ? "停止写入之后的手工记录" : "启用手工记录写入 HealthKit") {
                        if healthKit.isManualWriteEnabled {
                            healthKit.disableManualWrite()
                        } else {
                            showingHealthKitWriteConfirmation = true
                        }
                    }
                    switch healthKit.status {
                    case .requested: EmptyView()
                    case .syncing: Label("正在从 HealthKit 导入记录。", systemImage: "arrow.triangle.2.circlepath").foregroundStyle(.secondary)
                    case .imported(let count): Label(count == 0 ? "未发现可导入的 HealthKit 记录。请确认健康 App 的读取权限。" : "已从 HealthKit 导入 \(count) 天记录。", systemImage: count == 0 ? "info.circle" : "checkmark.circle").foregroundStyle(.secondary)
                    case .denied: Label("权限未授予，手动记录仍可使用。", systemImage: "exclamationmark.circle").foregroundStyle(.secondary)
                    case .unavailable: Label("此设备不支持 HealthKit。", systemImage: "xmark.circle").foregroundStyle(.secondary)
                    case .error(let message): Label(message, systemImage: "exclamationmark.circle").foregroundStyle(.secondary)
                    case .notRequested: EmptyView()
                    }
                }
                Section("隐私") { Label("记录会安全上传到量迹账户", systemImage: "lock") }
                if let accountMessage { Section { Label(accountMessage, systemImage: "exclamationmark.circle").foregroundStyle(.secondary) } }
            }.navigationTitle("我的")
        }
        .alert("永久删除账号？", isPresented: $showingDeleteConfirmation) {
            Button("删除账号", role: .destructive) { Task { await deleteAccount() } }
            Button("取消", role: .cancel) {}
        } message: { Text("所有已上传到量迹账户的记录、会话和本机缓存将被删除，此操作不可恢复。") }
        .alert("写入手工记录到健康 App？", isPresented: $showingHealthKitWriteConfirmation) {
            Button("启用") { Task { await healthKit.enableManualWrite() } }
            Button("取消", role: .cancel) {}
        } message: { Text("启用后，之后保存的手工体重和腰围会写入健康 App。每次编辑会更新量迹创建的对应样本；备注和目标不会写入。") }
    }

    private func deleteAccount() async {
        guard let session = TokenStore().session() else { accountMessage = "本地登录状态已失效。"; return }
        do {
            try await APIClient().deleteAccount(accessToken: session.accessToken)
            try LocalDataCleaner.clear(context: modelContext)
            appModel.signOut()
        } catch { accountMessage = error.localizedDescription }
    }

    private func signOut() async {
        let refreshToken = TokenStore().session()?.refreshToken
        do {
            try LocalDataCleaner.clear(context: modelContext)
            appModel.signOut()
        } catch {
            accountMessage = "无法清理本机数据，请重试退出。"
            return
        }
        if let refreshToken { try? await APIClient().logout(refreshToken: refreshToken) }
    }
}
