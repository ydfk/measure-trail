import AuthenticationServices
import SwiftUI

struct AuthenticationView: View {
    @Environment(AppModel.self) private var appModel
    @State private var mode: Mode = .signIn
    @State private var email = ""
    @State private var password = ""
    @State private var message: String?
    @State private var isSubmitting = false
    @State private var appleNonce: String?
    @State private var showingPasswordReset = false
    @State private var showingServerSettings = false

    enum Mode: String, CaseIterable, Identifiable { case signIn, signUp; var id: Self { self } }

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                Spacer()
                Image(systemName: "point.3.connected.trianglepath.dotted")
                    .font(.system(size: 54, weight: .medium))
                    .foregroundStyle(MeasureTrailStyle.ink)
                    .accessibilityHidden(true)
                VStack(spacing: 8) {
                    Text("量迹").font(.largeTitle.weight(.bold))
                    Text("记录变化，留住属于你的轨迹。")
                        .foregroundStyle(.secondary)
                }
                Picker("操作", selection: $mode) {
                    Text("登录").tag(Mode.signIn)
                    Text("注册").tag(Mode.signUp)
                }
                .pickerStyle(.segmented)
                .accessibilityLabel("账号操作")
                VStack(spacing: 14) {
                    TextField("邮箱", text: $email).textContentType(.emailAddress).keyboardType(.emailAddress).textInputAutocapitalization(.never).autocorrectionDisabled().accessibilityIdentifier("authentication-email")
                    SecureField("密码（至少 12 位）", text: $password).textContentType(mode == .signIn ? .password : .newPassword)
                }
                .textFieldStyle(.roundedBorder)
                .measureTrailActionSurface()
                if let message { Text(message).font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center) }
                Button {
                    showingServerSettings = true
                } label: {
                    Label(AppConfiguration.configuredAPIBaseURL == nil ? "设置量迹服务器" : "更改服务器", systemImage: "server.rack")
                }
                .font(.footnote)
                Button(mode == .signIn ? "登录" : "创建账号") { Task { await submit() } }
                    .buttonStyle(.borderedProminent)
                    .tint(MeasureTrailStyle.ink)
                    .controlSize(.large)
                    .disabled(email.isEmpty || password.count < 12 || isSubmitting || AppConfiguration.configuredAPIBaseURL == nil)
                    .accessibilityIdentifier("authentication-submit")
                SignInWithAppleButton(.continue) { request in
                    request.requestedScopes = [.email]
                    request.nonce = appleNonce
                } onCompletion: { result in
                    Task { await completeAppleLogin(result) }
                }
                    .frame(height: 50)
                    .accessibilityLabel("使用 Apple 登录")
                    .disabled(appleNonce == nil || isSubmitting || AppConfiguration.configuredAPIBaseURL == nil)
                Button("忘记密码？") { showingPasswordReset = true }.font(.footnote)
                Spacer()
            }
            .padding()
            .task {
                if AppConfiguration.configuredAPIBaseURL == nil { message = "请先设置并验证你的量迹服务器地址。" }
                await prepareAppleNonce()
            }
            .sheet(isPresented: $showingPasswordReset) { PasswordResetSheet(email: email) }
            .sheet(isPresented: $showingServerSettings, onDismiss: {
                appleNonce = nil
                Task { await prepareAppleNonce() }
            }) { ServerSettingsView() }
        }
    }

    private func submit() async {
        guard AppConfiguration.configuredAPIBaseURL != nil else { message = "请先设置并验证你的量迹服务器地址。"; return }
        isSubmitting = true
        defer { isSubmitting = false }
        do {
            if mode == .signIn {
                let session = try await APIClient().login(email: email, password: password)
                try TokenStore().save(accessToken: session.accessToken, refreshToken: session.refreshToken)
                appModel.didAuthenticate()
            } else {
                try await APIClient().register(email: email, password: password)
                message = "账号已创建。请查收验证邮件后再登录。"
            }
        } catch {
            message = error.localizedDescription
        }
    }

    private func prepareAppleNonce() async {
        guard appleNonce == nil, AppConfiguration.configuredAPIBaseURL != nil else { return }
        do { appleNonce = try await APIClient().newAppleNonce() }
        catch { message = "暂时无法准备 Apple 登录。" }
    }

    private func completeAppleLogin(_ result: Result<ASAuthorization, Error>) async {
        guard case .success(let authorization) = result, let credential = authorization.credential as? ASAuthorizationAppleIDCredential, let identityData = credential.identityToken, let identityToken = String(data: identityData, encoding: .utf8), let authorizationData = credential.authorizationCode, let authorizationCode = String(data: authorizationData, encoding: .utf8), let nonce = appleNonce else {
            message = "Apple 登录未完成。"
            await prepareAppleNonce()
            return
        }
        isSubmitting = true
        appleNonce = nil
        defer { isSubmitting = false }
        do {
            let session = try await APIClient().loginWithApple(identityToken: identityToken, authorizationCode: authorizationCode, nonce: nonce)
            try TokenStore().save(accessToken: session.accessToken, refreshToken: session.refreshToken)
            appModel.didAuthenticate()
        } catch { message = error.localizedDescription }
        await prepareAppleNonce()
    }
}

struct ServerSettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var address = UserDefaults.standard.string(forKey: AppConfiguration.apiBaseURLKey) ?? ""
    @State private var message: String?
    @State private var isValidating = false

    var body: some View {
        NavigationStack {
            Form {
                Section("量迹服务器") {
                    TextField("https://measuretrail.example.com", text: $address)
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .accessibilityIdentifier("server-address")
                    Text("请输入部署量迹服务的 HTTPS 基础地址，不要附加 /api 路径。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                Section {
                    Button("连接并保存") { Task { await validateAndSave() } }
                        .disabled(address.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isValidating)
                }
                if let message { Section { Label(message, systemImage: "exclamationmark.circle").foregroundStyle(.secondary) } }
            }
            .navigationTitle("服务器连接")
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } } }
        }
    }

    private func validateAndSave() async {
        guard let url = AppConfiguration.validatedAPIBaseURL(address) else {
            message = "请输入有效的 HTTPS 基础地址，且不要包含路径、查询参数或账号信息。"
            return
        }
        isValidating = true
        defer { isValidating = false }
        do {
            try await APIClient(baseURL: url).health()
            AppConfiguration.saveAPIBaseURL(url)
            dismiss()
        } catch {
            message = "无法连接服务器：\(error.localizedDescription)"
        }
    }
}

private struct PasswordResetSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var email: String
    @State private var token = ""
    @State private var password = ""
    @State private var confirmation = ""
    @State private var message: String?
    @State private var isSubmitting = false

    init(email: String) { _email = State(initialValue: email) }

    var body: some View {
        NavigationStack {
            Form {
                Section("申请重置") {
                    TextField("邮箱", text: $email).textContentType(.emailAddress).keyboardType(.emailAddress).textInputAutocapitalization(.never).autocorrectionDisabled()
                    Button("发送重置邮件") { Task { await requestReset() } }.disabled(email.isEmpty || isSubmitting)
                    Text("无论该邮箱是否已注册，都会显示相同的提交结果，以保护账号隐私。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                Section("设置新密码") {
                    TextField("邮件中的重置令牌", text: $token).textInputAutocapitalization(.never).autocorrectionDisabled()
                    SecureField("新密码（至少 12 位）", text: $password).textContentType(.newPassword)
                    SecureField("再次输入新密码", text: $confirmation).textContentType(.newPassword)
                    Button("重置密码") { Task { await completeReset() } }
                        .disabled(token.count < 20 || password.count < 12 || password != confirmation || isSubmitting)
                }
                if let message { Section { Label(message, systemImage: "envelope.badge").foregroundStyle(.secondary) } }
            }
            .navigationTitle("重置密码")
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("完成") { dismiss() } } }
        }
    }

    private func requestReset() async {
        isSubmitting = true
        defer { isSubmitting = false }
        do {
            try await APIClient().requestPasswordReset(email: email)
            message = "如果该邮箱可用，重置邮件已发送。请复制邮件中的令牌后设置新密码。"
        } catch {
            message = error.localizedDescription
        }
    }

    private func completeReset() async {
        guard password == confirmation else { message = "两次输入的密码不一致。"; return }
        isSubmitting = true
        defer { isSubmitting = false }
        do {
            try await APIClient().resetPassword(token: token, password: password)
            password = ""
            confirmation = ""
            token = ""
            message = "密码已重置。现在可以返回登录。"
        } catch {
            message = error.localizedDescription
        }
    }
}
