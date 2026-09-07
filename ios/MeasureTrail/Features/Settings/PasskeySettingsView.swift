import AuthenticationServices
import SwiftUI

struct PasskeySettingsView: View {
    @State private var passkeys: [APIClient.PasskeyItem] = []
    @State private var editor: PasskeyEditor?
    @State private var pendingDeletion: APIClient.PasskeyItem?
    @State private var message: String?
    @State private var isWorking = false

    var body: some View {
        List {
            Section {
                if passkeys.isEmpty && !isWorking {
                    ContentUnavailableView("还没有 Passkey", systemImage: "person.badge.key", description: Text("添加后可用 Face ID 或设备密码登录量迹。"))
                } else {
                    ForEach(passkeys) { passkey in
                        Button {
                            editor = .rename(passkey)
                        } label: {
                            HStack(spacing: 14) {
                                Image(systemName: "person.badge.key.fill")
                                    .foregroundStyle(MeasureTrailStyle.blue)
                                    .frame(width: 28)
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(passkey.name).foregroundStyle(.primary)
                                    Text(detail(for: passkey)).font(.caption).foregroundStyle(.secondary)
                                }
                                Spacer()
                                Image(systemName: "chevron.right").font(.caption.weight(.semibold)).foregroundStyle(.tertiary)
                            }
                        }
                        .swipeActions {
                            Button("删除", role: .destructive) { pendingDeletion = passkey }
                        }
                    }
                }
            } footer: {
                Text("Passkey 保存在你的 Apple 密码与钥匙串中。可以为不同设备添加多个，并随时重命名或删除。")
            }

            if let message {
                Section {
                    Label(message, systemImage: "exclamationmark.circle")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .navigationTitle("Passkey")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { editor = .add } label: { Label("添加 Passkey", systemImage: "plus") }
                    .disabled(isWorking)
            }
        }
        .overlay { if isWorking { ProgressView().controlSize(.large) } }
        .task { await load() }
        .refreshable { await load() }
        .sheet(item: $editor) { editor in
            PasskeyNameEditor(title: editor.title, initialName: editor.name) { name in
                switch editor {
                case .add: Task { await add(name: name) }
                case .rename(let passkey): Task { await rename(passkey, name: name) }
                }
            }
        }
        .confirmationDialog("删除这个 Passkey？", isPresented: Binding(
            get: { pendingDeletion != nil },
            set: { if !$0 { pendingDeletion = nil } }
        ), titleVisibility: .visible) {
            Button("删除 Passkey", role: .destructive) {
                if let passkey = pendingDeletion { Task { await delete(passkey) } }
                pendingDeletion = nil
            }
            Button("取消", role: .cancel) { pendingDeletion = nil }
        } message: {
            Text("删除后，这个 Passkey 将不能再用于量迹登录。")
        }
    }

    private func load() async {
        guard let token = TokenStore().session()?.accessToken else { message = "登录状态已失效。"; return }
        isWorking = true
        defer { isWorking = false }
        do {
            passkeys = try await APIClient().passkeys(accessToken: token)
            message = nil
        } catch { message = error.localizedDescription }
    }

    private func add(name: String) async {
        guard let token = TokenStore().session()?.accessToken else { message = "登录状态已失效。"; return }
        isWorking = true
        message = nil
        defer { isWorking = false }
        do {
            let client = APIClient()
            let options = try await client.beginPasskeyRegistration(name: name, accessToken: token)
            let credential = try await PasskeyAuthorizationService.shared.register(options: options)
            let passkey = try await client.finishPasskeyRegistration(sessionID: options.sessionId, credential: credential, accessToken: token)
            passkeys.append(passkey)
        } catch let error as ASAuthorizationError where error.code == .canceled {
            return
        } catch { message = PasskeyErrorMessage.make(from: error) }
    }

    private func rename(_ passkey: APIClient.PasskeyItem, name: String) async {
        guard let token = TokenStore().session()?.accessToken else { message = "登录状态已失效。"; return }
        isWorking = true
        defer { isWorking = false }
        do {
            let updated = try await APIClient().renamePasskey(id: passkey.id, name: name, accessToken: token)
            if let index = passkeys.firstIndex(where: { $0.id == passkey.id }) { passkeys[index] = updated }
            message = nil
        } catch { message = error.localizedDescription }
    }

    private func delete(_ passkey: APIClient.PasskeyItem) async {
        guard let token = TokenStore().session()?.accessToken else { message = "登录状态已失效。"; return }
        isWorking = true
        defer { isWorking = false }
        do {
            try await APIClient().deletePasskey(id: passkey.id, accessToken: token)
            passkeys.removeAll { $0.id == passkey.id }
            message = nil
        } catch { message = error.localizedDescription }
    }

    private func detail(for passkey: APIClient.PasskeyItem) -> String {
        guard let value = passkey.lastUsedAt else { return "尚未用于登录" }
        return "最近使用：\(formatted(value))"
    }

    private func formatted(_ value: String) -> String {
        guard let date = try? Date(value, strategy: .iso8601) else { return value }
        return date.formatted(date: .abbreviated, time: .shortened)
    }
}

private enum PasskeyEditor: Identifiable {
    case add
    case rename(APIClient.PasskeyItem)

    var id: String {
        switch self {
        case .add: "add"
        case .rename(let passkey): passkey.id
        }
    }

    var title: String {
        switch self {
        case .add: "添加 Passkey"
        case .rename: "重命名 Passkey"
        }
    }

    var name: String {
        switch self {
        case .add: "我的 iPhone"
        case .rename(let passkey): passkey.name
        }
    }
}

private struct PasskeyNameEditor: View {
    @Environment(\.dismiss) private var dismiss
    let title: String
    let onSave: (String) -> Void
    @State private var name: String

    init(title: String, initialName: String, onSave: @escaping (String) -> Void) {
        self.title = title
        self.onSave = onSave
        _name = State(initialValue: initialName)
    }

    var body: some View {
        NavigationStack {
            Form {
                TextField("Passkey 名称", text: $name)
                    .textInputAutocapitalization(.sentences)
                Text("使用容易识别的名称，例如“我的 iPhone”或“工作 Mac”。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("继续") {
                        onSave(name.trimmingCharacters(in: .whitespacesAndNewlines))
                        dismiss()
                    }
                    .disabled(name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || name.count > 128)
                }
            }
        }
        .presentationDetents([.medium])
    }
}
