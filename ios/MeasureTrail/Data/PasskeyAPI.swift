import Foundation

extension APIClient {
    struct PasskeyItem: Decodable, Identifiable, Sendable {
        let id: String
        let name: String
        let createdAt: String
        let lastUsedAt: String?
    }

    struct PasskeyRegistrationOptions: Decodable, Sendable {
        let sessionId: String
        let options: CreationOptions

        struct CreationOptions: Decodable, Sendable {
            let publicKey: PublicKey
        }

        struct PublicKey: Decodable, Sendable {
            let challenge: String
            let rp: RelyingParty
            let user: User
            let excludeCredentials: [CredentialDescriptor]?
        }

        struct RelyingParty: Decodable, Sendable { let id: String }
        struct User: Decodable, Sendable {
            let id: String
            let name: String
        }
    }

    struct PasskeyLoginOptions: Decodable, Sendable {
        let sessionId: String
        let options: RequestOptions

        struct RequestOptions: Decodable, Sendable {
            let publicKey: PublicKey
        }

        struct PublicKey: Decodable, Sendable {
            let challenge: String
            let rpId: String
        }
    }

    struct CredentialDescriptor: Decodable, Sendable {
        let id: String
    }

    struct RegistrationCredential: Encodable, Sendable {
        let id: String
        let rawId: String
        let type = "public-key"
        let response: Response

        struct Response: Encodable, Sendable {
            let clientDataJSON: String
            let attestationObject: String
        }
    }

    struct AssertionCredential: Encodable, Sendable {
        let id: String
        let rawId: String
        let type = "public-key"
        let response: Response

        struct Response: Encodable, Sendable {
            let clientDataJSON: String
            let authenticatorData: String
            let signature: String
            let userHandle: String
        }
    }

    func beginPasskeyLogin() async throws -> PasskeyLoginOptions {
        try await passkeyRequest(path: "/api/v1/auth/passkey/login/options", method: "POST")
    }

    func finishPasskeyLogin(sessionID: String, credential: AssertionCredential) async throws -> SessionResponse {
        struct Payload: Encodable { let sessionId: String; let credential: AssertionCredential; let deviceLabel: String }
        return try await passkeyRequest(path: "/api/v1/auth/passkey/login/verify", method: "POST", body: Payload(sessionId: sessionID, credential: credential, deviceLabel: "iPhone"))
    }

    func passkeys(accessToken: String) async throws -> [PasskeyItem] {
        struct Response: Decodable { let passkeys: [PasskeyItem] }
        let response: Response = try await authorizedPasskeyRequest(path: "/api/v1/account/passkeys", method: "GET", accessToken: accessToken)
        return response.passkeys
    }

    func beginPasskeyRegistration(name: String, accessToken: String) async throws -> PasskeyRegistrationOptions {
        struct Payload: Encodable { let name: String }
        return try await authorizedPasskeyRequest(path: "/api/v1/account/passkeys/registration/options", method: "POST", body: Payload(name: name), accessToken: accessToken)
    }

    func finishPasskeyRegistration(sessionID: String, credential: RegistrationCredential, accessToken: String) async throws -> PasskeyItem {
        struct Payload: Encodable { let sessionId: String; let credential: RegistrationCredential }
        return try await authorizedPasskeyRequest(path: "/api/v1/account/passkeys/registration/verify", method: "POST", body: Payload(sessionId: sessionID, credential: credential), accessToken: accessToken)
    }

    func renamePasskey(id: String, name: String, accessToken: String) async throws -> PasskeyItem {
        struct Payload: Encodable { let name: String }
        return try await authorizedPasskeyRequest(path: "/api/v1/account/passkeys/\(id)", method: "PATCH", body: Payload(name: name), accessToken: accessToken)
    }

    func deletePasskey(id: String, accessToken: String) async throws {
        let _: PasskeyEmptyResponse = try await authorizedPasskeyRequest(path: "/api/v1/account/passkeys/\(id)", method: "DELETE", accessToken: accessToken)
    }

    private func passkeyRequest<Response: Decodable>(path: String, method: String) async throws -> Response {
        try await passkeyRequest(path: path, method: method, body: Optional<PasskeyEmptyRequest>.none)
    }

    private func passkeyRequest<Body: Encodable, Response: Decodable>(path: String, method: String, body: Body?) async throws -> Response {
        guard let baseURL, let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = method
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONEncoder().encode(body)
        }
        let (data, response) = try await execute(request)
        guard (200...299).contains(response.statusCode) else { throw responseError(data, statusCode: response.statusCode, fallback: "Passkey 请求未完成。") }
        return try decodePasskeyResponse(data)
    }

    private func authorizedPasskeyRequest<Response: Decodable>(path: String, method: String, accessToken: String) async throws -> Response {
        try await authorizedPasskeyRequest(path: path, method: method, body: Optional<PasskeyEmptyRequest>.none, accessToken: accessToken)
    }

    private func authorizedPasskeyRequest<Body: Encodable, Response: Decodable>(path: String, method: String, body: Body?, accessToken: String) async throws -> Response {
        guard let baseURL, let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONEncoder().encode(body)
        }
        let (data, response) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(response.statusCode) else { throw responseError(data, statusCode: response.statusCode, fallback: "Passkey 请求未完成。") }
        return try decodePasskeyResponse(data)
    }

    private func decodePasskeyResponse<Response: Decodable>(_ data: Data) throws -> Response {
        if Response.self == PasskeyEmptyResponse.self { return PasskeyEmptyResponse() as! Response }
        return try JSONDecoder().decode(Response.self, from: data)
    }
}

private struct PasskeyEmptyRequest: Encodable {}
private struct PasskeyEmptyResponse: Decodable {}

extension Data {
    init?(base64URLEncoded value: String) {
        var base64 = value.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        base64 += String(repeating: "=", count: (4 - base64.count % 4) % 4)
        self.init(base64Encoded: base64)
    }

    var base64URLEncodedString: String {
        base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
    }
}
