import SwiftUI

struct AccountCredentialsView: View {
    @Environment(AppModel.self) private var appModel
    @State private var originalUsername = ""
    @State private var username = ""
    @State private var currentPassword = ""
    @State private var newPassword = ""
    @State private var passwordConfirmation = ""
    @State private var message: String?
    @State private var isLoading = true
    @State private var isSaving = false
    @State private var showingSuccess = false

    private var normalizedUsername: String { username.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
    private var usernameChanged: Bool { normalizedUsername != originalUsername.lowercased() }
    private var passwordChanged: Bool { !newPassword.isEmpty }
    private var usernameIsValid: Bool {
        normalizedUsername.count >= 3 && normalizedUsername.count <= 32 && normalizedUsername.allSatisfy {
            $0.isLetter || $0.isNumber || $0 == "." || $0 == "_" || $0 == "-"
        }
    }
    private var canSave: Bool {
        !isLoading && !isSaving && currentPassword.count >= 6 &&
            usernameIsValid &&
            (usernameChanged || passwordChanged) &&
            (!passwordChanged || (newPassword.count >= 6 && newPassword.count <= 128 && newPassword == passwordConfirmation))
    }

    var body: some View {
        Form {
            Section("用户名") {
                TextField("用户名", text: $username)
                    .textContentType(.username)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("account-username")
                Text("用户名为 3 至 32 个字符，可使用字母、数字、点、下划线和连字符。")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            Section {
                SecureField("当前密码", text: $currentPassword)
                    .textContentType(.password)
                    .accessibilityIdentifier("account-current-password")
                SecureField("新密码（留空则不修改）", text: $newPassword)
                    .textContentType(.newPassword)
                    .accessibilityIdentifier("account-new-password")
                SecureField("再次输入新密码", text: $passwordConfirmation)
                    .textContentType(.newPassword)
                    .disabled(newPassword.isEmpty)
                    .accessibilityIdentifier("account-confirm-password")
                if passwordChanged && newPassword != passwordConfirmation {
                    Text("两次输入的新密码不一致。")
                        .font(.footnote)
                        .foregroundStyle(.red)
                }
            } header: {
                Text("修改密码")
            } footer: {
                Text("保存后所有登录设备的 Refresh Token 会失效，你需要使用新凭证重新登录。")
            }
            if let message {
                Section {
                    Label(message, systemImage: "exclamationmark.circle")
                        .foregroundStyle(.red)
                }
            }
        }
        .navigationTitle("用户名与密码")
        .disabled(isLoading || isSaving)
        .overlay {
            if isLoading { ProgressView("正在读取账号…") }
        }
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button(isSaving ? "正在保存…" : "保存") { Task { await save() } }
                    .disabled(!canSave)
                    .accessibilityIdentifier("account-save")
            }
        }
        .task { await load() }
        .alert("登录信息已更新", isPresented: $showingSuccess) {
            Button("重新登录") { appModel.signOut() }
        } message: {
            Text("请使用更新后的用户名和密码登录。")
        }
    }

    private func load() async {
        guard let session = TokenStore().session() else {
            message = "本地登录状态已失效。"
            isLoading = false
            return
        }
        defer { isLoading = false }
        do {
            let credentials = try await APIClient().accountCredentials(accessToken: session.accessToken)
            originalUsername = credentials.username
            username = credentials.username
        } catch {
            message = error.localizedDescription
        }
    }

    private func save() async {
        guard canSave, let session = TokenStore().session() else { return }
        isSaving = true
        message = nil
        defer { isSaving = false }
        do {
            try await APIClient().updateAccountCredentials(
                currentPassword: currentPassword,
                username: usernameChanged ? normalizedUsername : nil,
                password: passwordChanged ? newPassword : nil,
                accessToken: session.accessToken
            )
            showingSuccess = true
        } catch {
            message = error.localizedDescription
        }
    }
}
