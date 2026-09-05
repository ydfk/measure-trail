import AuthenticationServices
import SwiftUI

struct AuthenticationView: View {
    @Environment(AppModel.self) private var appModel
    @Environment(\.colorScheme) private var colorScheme
    @FocusState private var focusedField: Field?
    @State private var email = ""
    @State private var password = ""
    @State private var message: String?
    @State private var appleMessage: String?
    @State private var isSubmitting = false
    @State private var appleNonce: String?
    @State private var showingPasswordReset = false

    private enum Field { case email, password }
    private var canSubmit: Bool {
        !email.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && password.count >= 12 && !isSubmitting
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 36) {
                    brandHeader
                    VStack(alignment: .leading, spacing: 24) {
                        Text("登录量迹")
                            .font(.title2.bold())
                            .accessibilityAddTraits(.isHeader)
                        credentials
                        if let message {
                            Label(message, systemImage: "exclamationmark.circle")
                                .font(.footnote)
                                .foregroundStyle(.red)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        Button { Task { await submit() } } label: {
                            HStack(spacing: 10) {
                                if isSubmitting { ProgressView().tint(.white) }
                                Text(isSubmitting ? "正在登录…" : "登录")
                                    .font(.headline)
                            }
                            .frame(maxWidth: .infinity, minHeight: 36)
                        }
                        .buttonStyle(.borderedProminent)
                        .buttonBorderShape(.roundedRectangle(radius: 16))
                        .controlSize(.large)
                        .disabled(!canSubmit)
                        .accessibilityIdentifier("authentication-submit")
                    }
                    appleLogin
                }
                .frame(maxWidth: 440)
                .frame(maxWidth: .infinity)
                .padding(.horizontal, 28)
                .padding(.top, 32)
                .padding(.bottom, 32)
            }
            .scrollDismissesKeyboard(.interactively)
            .background(Color(uiColor: .systemBackground).ignoresSafeArea())
            .toolbar(.hidden, for: .navigationBar)
            .tint(colorScheme == .dark ? MeasureTrailStyle.blue : MeasureTrailStyle.ink)
            .task { await prepareAppleNonce() }
            .sheet(isPresented: $showingPasswordReset) { PasswordResetSheet(email: email) }
        }
    }

    private var brandHeader: some View {
        VStack(alignment: .leading, spacing: 20) {
            Image("BrandLogo")
                .resizable()
                .scaledToFit()
                .frame(width: 96, height: 96)
                .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
                .accessibilityLabel("量迹标志")
                .accessibilityIdentifier("authentication-logo")
            VStack(alignment: .leading, spacing: 10) {
                Text("量迹").font(.largeTitle.bold())
                Text("记录变化，留住属于你的轨迹。")
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var credentials: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 8) {
                Text("邮箱").font(.subheadline.weight(.medium))
                TextField("输入邮箱地址", text: $email, prompt: Text("输入邮箱地址").foregroundColor(Color(uiColor: .secondaryLabel)))
                    .textContentType(.username)
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .focused($focusedField, equals: .email)
                    .submitLabel(.next)
                    .onSubmit { focusedField = .password }
                    .accessibilityLabel("邮箱")
                    .accessibilityIdentifier("authentication-email")
                    .loginInputSurface()
            }
            VStack(alignment: .leading, spacing: 8) {
                Text("密码").font(.subheadline.weight(.medium))
                SecureField("输入密码", text: $password, prompt: Text("输入密码").foregroundColor(Color(uiColor: .secondaryLabel)))
                    .textContentType(.password)
                    .focused($focusedField, equals: .password)
                    .submitLabel(.go)
                    .onSubmit { if canSubmit { Task { await submit() } } }
                    .accessibilityLabel("密码")
                    .accessibilityIdentifier("authentication-password")
                    .loginInputSurface()
            }
            Button("忘记密码？") { showingPasswordReset = true }
                .font(.subheadline)
                .frame(minHeight: 44)
                .frame(maxWidth: .infinity, alignment: .trailing)
        }
        .disabled(isSubmitting)
    }

    private var appleLogin: some View {
        VStack(spacing: 16) {
            HStack(spacing: 16) {
                Rectangle().fill(Color(uiColor: .separator)).frame(height: 0.5)
                Text("或").font(.footnote).foregroundStyle(.secondary)
                Rectangle().fill(Color(uiColor: .separator)).frame(height: 0.5)
            }
            SignInWithAppleButton(.signIn) { request in
                request.requestedScopes = [.email]
                request.nonce = appleNonce
            } onCompletion: { result in
                Task { await completeAppleLogin(result) }
            }
            .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
            .id(colorScheme)
            .frame(height: 52)
            .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
            .accessibilityLabel("使用 Apple 登录")
            .disabled(appleNonce == nil || isSubmitting)
            if let appleMessage {
                Text(appleMessage).font(.footnote).foregroundStyle(.secondary)
                Button("重试 Apple 登录") { Task { await prepareAppleNonce() } }
                    .font(.footnote)
                    .frame(minHeight: 44)
                    .disabled(isSubmitting)
            }
        }
    }

    private func submit() async {
        guard canSubmit else { return }
        focusedField = nil
        isSubmitting = true
        message = nil
        defer { isSubmitting = false }
        do {
            let session = try await APIClient().login(email: email.trimmingCharacters(in: .whitespacesAndNewlines), password: password)
            try TokenStore().save(accessToken: session.accessToken, refreshToken: session.refreshToken)
            appModel.didAuthenticate()
        } catch { message = error.localizedDescription }
    }

    private func prepareAppleNonce() async {
        guard appleNonce == nil else { return }
        appleMessage = nil
        do { appleNonce = try await APIClient().newAppleNonce() }
        catch { appleMessage = "Apple 登录暂不可用，可使用邮箱登录。" }
    }

    private func completeAppleLogin(_ result: Result<ASAuthorization, Error>) async {
        guard case .success(let authorization) = result,
              let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
              let identityData = credential.identityToken, let identityToken = String(data: identityData, encoding: .utf8),
              let authorizationData = credential.authorizationCode, let authorizationCode = String(data: authorizationData, encoding: .utf8),
              let nonce = appleNonce else {
            if case .failure(let error) = result, (error as? ASAuthorizationError)?.code == .canceled { return }
            appleMessage = "Apple 登录未完成，请重试。"
            appleNonce = nil
            return
        }
        isSubmitting = true
        message = nil
        appleNonce = nil
        defer { isSubmitting = false }
        do {
            let session = try await APIClient().loginWithApple(identityToken: identityToken, authorizationCode: authorizationCode, nonce: nonce)
            try TokenStore().save(accessToken: session.accessToken, refreshToken: session.refreshToken)
            appModel.didAuthenticate()
        } catch {
            message = error.localizedDescription
            await prepareAppleNonce()
        }
    }
}

private extension View {
    func loginInputSurface() -> some View {
        padding(.horizontal, 16)
            .padding(.vertical, 16)
            .frame(minHeight: 54)
            .background(Color(uiColor: .secondarySystemBackground), in: RoundedRectangle(cornerRadius: 14))
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
