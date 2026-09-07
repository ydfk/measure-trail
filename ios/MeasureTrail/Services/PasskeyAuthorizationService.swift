@preconcurrency import AuthenticationServices
import Foundation
import UIKit

@MainActor
final class PasskeyAuthorizationService: NSObject {
    static let shared = PasskeyAuthorizationService()

    private var continuation: CheckedContinuation<ASAuthorization, Error>?

    func register(options: APIClient.PasskeyRegistrationOptions) async throws -> APIClient.RegistrationCredential {
        let publicKey = options.options.publicKey
        try validateRelyingParty(publicKey.rp.id)
        guard let challenge = Data(base64URLEncoded: publicKey.challenge),
              let userID = Data(base64URLEncoded: publicKey.user.id) else {
            throw PasskeyAuthorizationError.invalidOptions
        }
        let provider = ASAuthorizationPlatformPublicKeyCredentialProvider(relyingPartyIdentifier: publicKey.rp.id)
        let request = provider.createCredentialRegistrationRequest(challenge: challenge, name: publicKey.user.name, userID: userID)
        request.excludedCredentials = publicKey.excludeCredentials?.compactMap { descriptor in
            Data(base64URLEncoded: descriptor.id).map(ASAuthorizationPlatformPublicKeyCredentialDescriptor.init)
        } ?? []
        let authorization = try await perform(request)
        guard let credential = authorization.credential as? ASAuthorizationPlatformPublicKeyCredentialRegistration,
              let attestationObject = credential.rawAttestationObject else {
            throw PasskeyAuthorizationError.invalidCredential
        }
        let credentialID = credential.credentialID.base64URLEncodedString
        return APIClient.RegistrationCredential(
            id: credentialID,
            rawId: credentialID,
            response: .init(
                clientDataJSON: credential.rawClientDataJSON.base64URLEncodedString,
                attestationObject: attestationObject.base64URLEncodedString
            )
        )
    }

    func authenticate(options: APIClient.PasskeyLoginOptions) async throws -> APIClient.AssertionCredential {
        let publicKey = options.options.publicKey
        try validateRelyingParty(publicKey.rpId)
        guard let challenge = Data(base64URLEncoded: publicKey.challenge) else {
            throw PasskeyAuthorizationError.invalidOptions
        }
        let provider = ASAuthorizationPlatformPublicKeyCredentialProvider(relyingPartyIdentifier: publicKey.rpId)
        let authorization = try await perform(provider.createCredentialAssertionRequest(challenge: challenge))
        guard let credential = authorization.credential as? ASAuthorizationPlatformPublicKeyCredentialAssertion else {
            throw PasskeyAuthorizationError.invalidCredential
        }
        let credentialID = credential.credentialID.base64URLEncodedString
        return APIClient.AssertionCredential(
            id: credentialID,
            rawId: credentialID,
            response: .init(
                clientDataJSON: credential.rawClientDataJSON.base64URLEncodedString,
                authenticatorData: credential.rawAuthenticatorData.base64URLEncodedString,
                signature: credential.signature.base64URLEncodedString,
                userHandle: credential.userID.base64URLEncodedString
            )
        )
    }

    private func validateRelyingParty(_ relyingParty: String) throws {
        guard relyingParty == AppConfiguration.passkeyRelyingPartyID else {
            throw PasskeyAuthorizationError.relyingPartyMismatch
        }
    }

    private func perform(_ request: ASAuthorizationRequest) async throws -> ASAuthorization {
        guard continuation == nil else { throw PasskeyAuthorizationError.requestInProgress }
        return try await withCheckedThrowingContinuation { continuation in
            self.continuation = continuation
            let controller = ASAuthorizationController(authorizationRequests: [request])
            controller.delegate = self
            controller.presentationContextProvider = self
            controller.performRequests()
        }
    }
}

extension PasskeyAuthorizationService: ASAuthorizationControllerDelegate {
    func authorizationController(controller: ASAuthorizationController, didCompleteWithAuthorization authorization: ASAuthorization) {
        continuation?.resume(returning: authorization)
        continuation = nil
    }

    func authorizationController(controller: ASAuthorizationController, didCompleteWithError error: Error) {
        continuation?.resume(throwing: error)
        continuation = nil
    }
}

extension PasskeyAuthorizationService: ASAuthorizationControllerPresentationContextProviding {
    func presentationAnchor(for controller: ASAuthorizationController) -> ASPresentationAnchor {
        guard let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else {
            preconditionFailure("Passkey 请求需要可见窗口")
        }
        return scene.windows.first(where: \.isKeyWindow) ?? UIWindow(windowScene: scene)
    }
}

enum PasskeyAuthorizationError: LocalizedError {
    case invalidOptions
    case invalidCredential
    case relyingPartyMismatch
    case requestInProgress

    var errorDescription: String? {
        switch self {
        case .invalidOptions: "服务器返回的 Passkey 选项无效。"
        case .invalidCredential: "系统没有返回有效的 Passkey 凭据。"
        case .relyingPartyMismatch: "Passkey 域名与当前 App 配置不一致。"
        case .requestInProgress: "已有一个 Passkey 请求正在进行。"
        }
    }
}
