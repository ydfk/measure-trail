import AuthenticationServices
import SwiftUI

struct AuthenticationView: View {
    @Environment(AppModel.self) private var appModel
    @FocusState private var focusedField: Field?
    @State private var username = ""
    @State private var password = ""
    @State private var message: String?
    @State private var isSubmitting = false

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
                    passkeyLoginButton
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
            .tint(MeasureTrailStyle.accent)
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
            message = PasskeyErrorMessage.make(from: error)
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
