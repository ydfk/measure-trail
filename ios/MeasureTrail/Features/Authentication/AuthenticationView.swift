import AuthenticationServices
import SwiftUI

struct AuthenticationView: View {
    @Environment(AppModel.self) private var appModel
    @Environment(\.colorScheme) private var colorScheme
    @FocusState private var focusedField: Field?
    @State private var username = "admin"
    @State private var password = ""
    @State private var message: String?
    @State private var appleMessage: String?
    @State private var isSubmitting = false
    @State private var appleNonce: String?

    private enum Field { case username, password }
    private var canSubmit: Bool {
        username.trimmingCharacters(in: .whitespacesAndNewlines).count >= 3 && password.count >= 6 && !isSubmitting
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
                Text("用户名").font(.subheadline.weight(.medium))
                TextField("输入用户名", text: $username, prompt: Text("输入用户名").foregroundColor(Color(uiColor: .secondaryLabel)))
                    .textContentType(.username)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .focused($focusedField, equals: .username)
                    .submitLabel(.next)
                    .onSubmit { focusedField = .password }
                    .accessibilityLabel("用户名")
                    .accessibilityIdentifier("authentication-username")
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
            passkeyLoginButton
            SignInWithAppleButton(.signIn) { request in
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

    private var passkeyLoginButton: some View {
        Button { Task { await loginWithPasskey() } } label: {
            Label("使用 Passkey 登录", systemImage: "person.badge.key.fill")
                .font(.headline)
                .frame(maxWidth: .infinity, minHeight: 36)
        }
        .buttonStyle(.bordered)
        .buttonBorderShape(.roundedRectangle(radius: 16))
        .controlSize(.large)
        .disabled(isSubmitting)
        .accessibilityIdentifier("authentication-passkey")
    }

    private func submit() async {
        guard canSubmit else { return }
        focusedField = nil
        isSubmitting = true
        message = nil
        defer { isSubmitting = false }
        do {
            let session = try await APIClient().login(username: username.trimmingCharacters(in: .whitespacesAndNewlines), password: password)
            try TokenStore().save(accessToken: session.accessToken, refreshToken: session.refreshToken)
            appModel.didAuthenticate()
        } catch { message = error.localizedDescription }
    }

    private func loginWithPasskey() async {
        focusedField = nil
        isSubmitting = true
        message = nil
        defer { isSubmitting = false }
        do {
            let client = APIClient()
            let options = try await client.beginPasskeyLogin()
            let credential = try await PasskeyAuthorizationService.shared.authenticate(options: options)
            let session = try await client.finishPasskeyLogin(sessionID: options.sessionId, credential: credential)
            try TokenStore().save(accessToken: session.accessToken, refreshToken: session.refreshToken)
            appModel.didAuthenticate()
        } catch let error as ASAuthorizationError where error.code == .canceled {
            return
        } catch {
            message = error.localizedDescription
        }
    }

    private func prepareAppleNonce() async {
        guard appleNonce == nil else { return }
        appleMessage = nil
        do { appleNonce = try await APIClient().newAppleNonce() }
        catch { appleMessage = "Apple 登录暂不可用，可使用用户名登录。" }
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
