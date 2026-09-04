import SwiftUI

struct SessionsView: View {
    @State private var sessions = [APIClient.SessionInfo]()
    @State private var message: String?
    @State private var sessionToRevoke: APIClient.SessionInfo?

    var body: some View {
        List {
            Section {
                Text("这里显示仍可用于刷新登录状态的设备会话。撤销后，该设备下次刷新时需要重新登录。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if sessions.isEmpty && message == nil {
                ContentUnavailableView("没有活跃会话", systemImage: "laptopcomputer.and.iphone")
            } else {
                Section("活跃会话") {
                    ForEach(sessions) { session in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(session.deviceLabel.isEmpty ? "未命名设备" : session.deviceLabel)
                            Text("登录于 \(dateText(session.createdAt))，到期于 \(dateText(session.expiresAt))")
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                        .swipeActions {
                            Button("撤销", role: .destructive) { sessionToRevoke = session }
                        }
                    }
                }
            }
            if let message { Section { Label(message, systemImage: "exclamationmark.circle").foregroundStyle(.secondary) } }
        }
        .navigationTitle("登录设备")
        .task { await loadSessions() }
        .refreshable { await loadSessions() }
        .alert("撤销此设备登录？", isPresented: Binding(get: { sessionToRevoke != nil }, set: { if !$0 { sessionToRevoke = nil } })) {
            Button("撤销", role: .destructive) { Task { await revokeSelectedSession() } }
            Button("取消", role: .cancel) { sessionToRevoke = nil }
        } message: {
            Text("该设备下次刷新登录状态时需要重新登录。")
        }
    }

    private func loadSessions() async {
        guard let session = TokenStore().session() else { message = "本地登录状态已失效。"; return }
        do {
            sessions = try await APIClient().sessions(accessToken: session.accessToken)
            message = nil
        } catch {
            message = error.localizedDescription
        }
    }

    private func revokeSelectedSession() async {
        guard let target = sessionToRevoke, let session = TokenStore().session() else { return }
        do {
            try await APIClient().revokeSession(id: target.id, accessToken: session.accessToken)
            sessions.removeAll { $0.id == target.id }
        } catch {
            message = error.localizedDescription
        }
        sessionToRevoke = nil
    }

    private func dateText(_ value: String) -> String {
        guard let date = ISO8601DateFormatter().date(from: value) else { return value }
        return date.formatted(date: .abbreviated, time: .shortened)
    }
}
