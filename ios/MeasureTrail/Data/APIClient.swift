import Foundation

struct APIClient: Sendable {
    struct SessionResponse: Decodable {
        let accessToken: String
        let refreshToken: String
    }

    enum APIError: LocalizedError, Sendable {
        case invalidEndpoint
        case conflict(String)
        case rejected(String)

        var errorDescription: String? {
            switch self {
            case .invalidEndpoint: "服务地址无效。"
            case .conflict(let message): message
            case .rejected(let message): message
            }
        }
    }

    let baseURL: URL?

    init(baseURL: URL? = AppConfiguration.apiBaseURL) {
        self.baseURL = baseURL
    }

    func health() async throws {
        guard let baseURL, let url = URL(string: "/api/health", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        let (data, httpResponse) = try await execute(URLRequest(url: url))
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "服务器健康检查未通过。") }
        let response = try JSONDecoder().decode(HealthResponse.self, from: data)
        guard response.status == "ok" else { throw APIError.rejected("服务器未就绪。") }
    }

    func login(username: String, password: String) async throws -> SessionResponse {
        try await send(path: "/api/v1/auth/login", body: Credentials(username: username, password: password, deviceLabel: "iPhone"))
    }

    func refresh(refreshToken: String) async throws -> SessionResponse {
        struct Payload: Encodable { let refreshToken: String; let deviceLabel: String }
        return try await send(path: "/api/v1/auth/refresh", body: Payload(refreshToken: refreshToken, deviceLabel: "iPhone"))
    }

    func logout(refreshToken: String) async throws {
        struct Payload: Encodable { let refreshToken: String; let deviceLabel: String }
        let _: EmptyResponse = try await send(path: "/api/v1/auth/logout", body: Payload(refreshToken: refreshToken, deviceLabel: "iPhone"))
    }

    struct MeasurementResponse: Decodable, Sendable { let id: String; let version: Int }

    struct ProfileResponse: Codable, Sendable {
        let heightMM: Int?
        let targetWeightG: Int?
        let preferredUnit: String
        let timezone: String
    }

    struct SessionInfo: Decodable, Identifiable, Sendable {
        let id: String
        let deviceLabel: String
        let createdAt: String
        let expiresAt: String
    }

    struct AccountCredentials: Decodable, Sendable {
        let username: String
    }

    struct MeasurementChange: Decodable, Sendable {
        let id: String
        let recordedOn: String
        let weightG: Int
        let waistMM: Int?
        let note: String
        let source: String
        let healthkitUuid: String?
        let version: Int
        let deletedAt: String?
        let updatedAt: String
    }

    struct MeasurementChangePage: Decodable, Sendable {
        let measurements: [MeasurementChange]
        let nextCursor: String?
        let hasMore: Bool
    }

    func upsertMeasurement(date: String, weightG: Int, waistMM: Int?, note: String, mutationID: UUID, accessToken: String) async throws -> MeasurementResponse {
        struct Payload: Encodable { let weightG: Int; let waistMM: Int?; let note: String; let clientMutationId: String }
        guard let baseURL, let url = URL(string: "/api/v1/measurements/by-date/\(date)", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "PUT"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        request.httpBody = try JSONEncoder().encode(Payload(weightG: weightG, waistMM: waistMM, note: note, clientMutationId: mutationID.uuidString))
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "记录尚未同步。") }
        return try JSONDecoder().decode(MeasurementResponse.self, from: data)
    }

    func importHealthKitMeasurement(recordedOn: String, weightG: Int, waistMM: Int?, healthKitUUID: String, mutationID: UUID, accessToken: String) async throws -> MeasurementChange {
        struct Payload: Encodable { let recordedOn: String; let weightG: Int; let waistMM: Int?; let healthkitUuid: String; let clientMutationId: String }
        return try await sendAuthorized(path: "/api/v1/measurements/healthkit", body: Payload(recordedOn: recordedOn, weightG: weightG, waistMM: waistMM, healthkitUuid: healthKitUUID, clientMutationId: mutationID.uuidString), accessToken: accessToken)
    }

    func deleteMeasurement(id: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws {
        guard let baseURL, let url = URL(string: "/api/v1/measurements/\(id)", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        struct Payload: Encodable { let expectedVersion: Int; let clientMutationId: String }
        request.httpBody = try JSONEncoder().encode(Payload(expectedVersion: expectedVersion, clientMutationId: mutationID.uuidString))
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "删除尚未同步。") }
    }

    func updateMeasurement(id: String, weightG: Int, waistMM: Int?, note: String, expectedVersion: Int, mutationID: UUID, accessToken: String) async throws -> MeasurementResponse {
        struct Payload: Encodable { let weightG: Int; let waistMM: Int?; let note: String; let expectedVersion: Int; let clientMutationId: String }
        guard let baseURL, let url = URL(string: "/api/v1/measurements/\(id)", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "PATCH"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        request.httpBody = try JSONEncoder().encode(Payload(weightG: weightG, waistMM: waistMM, note: note, expectedVersion: expectedVersion, clientMutationId: mutationID.uuidString))
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "编辑尚未同步。") }
        return try JSONDecoder().decode(MeasurementResponse.self, from: data)
    }

    func measurement(id: String, accessToken: String) async throws -> MeasurementChange {
        guard let baseURL, let url = URL(string: "/api/v1/measurements/\(id)", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "无法读取云端记录。") }
        return try JSONDecoder().decode(MeasurementChange.self, from: data)
    }

    func measurement(recordedOn: String, accessToken: String) async throws -> MeasurementChange {
        guard let baseURL, let endpoint = URL(string: "/api/v1/measurements", relativeTo: baseURL), var components = URLComponents(url: endpoint, resolvingAgainstBaseURL: true) else { throw APIError.invalidEndpoint }
        components.queryItems = [
            URLQueryItem(name: "from", value: recordedOn),
            URLQueryItem(name: "to", value: recordedOn),
            URLQueryItem(name: "limit", value: "1"),
        ]
        guard let url = components.url else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "无法读取云端记录。") }
        struct Response: Decodable { let measurements: [MeasurementChange] }
        guard let measurement = try JSONDecoder().decode(Response.self, from: data).measurements.first else {
            throw APIError.rejected("云端记录不存在。")
        }
        return measurement
    }

    func listMeasurementChanges(cursor: String?, accessToken: String) async throws -> MeasurementChangePage {
        guard let baseURL, let endpoint = URL(string: "/api/v1/measurement-changes", relativeTo: baseURL), var components = URLComponents(url: endpoint, resolvingAgainstBaseURL: true) else { throw APIError.invalidEndpoint }
        var queryItems = [URLQueryItem(name: "limit", value: "100")]
        if let cursor, !cursor.isEmpty { queryItems.append(URLQueryItem(name: "cursor", value: cursor)) }
        components.queryItems = queryItems
        guard let url = components.url else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else {
            throw APIError.rejected((try? JSONDecoder().decode(Problem.self, from: data))?.detail ?? "同步数据暂不可用。")
        }
        return try JSONDecoder().decode(MeasurementChangePage.self, from: data)
    }

    func profile(accessToken: String) async throws -> ProfileResponse {
        guard let baseURL, let url = URL(string: "/api/v1/profile", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else {
            throw APIError.rejected((try? JSONDecoder().decode(Problem.self, from: data))?.detail ?? "无法读取资料。")
        }
        return try JSONDecoder().decode(ProfileResponse.self, from: data)
    }

    func sessions(accessToken: String) async throws -> [SessionInfo] {
        struct Response: Decodable { let sessions: [SessionInfo] }
        guard let baseURL, let url = URL(string: "/api/v1/auth/sessions", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "无法读取活跃会话。") }
        return try JSONDecoder().decode(Response.self, from: data).sessions
    }

    func revokeSession(id: String, accessToken: String) async throws {
        guard let baseURL, let url = URL(string: "/api/v1/auth/sessions/\(id)", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "会话撤销未完成。") }
    }

    func accountCredentials(accessToken: String) async throws -> AccountCredentials {
        guard let baseURL, let url = URL(string: "/api/v1/account/credentials", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "无法读取登录信息。") }
        return try JSONDecoder().decode(AccountCredentials.self, from: data)
    }

    func updateAccountCredentials(currentPassword: String, username: String?, password: String?, accessToken: String) async throws {
        struct Payload: Encodable {
            let currentPassword: String
            let username: String?
            let password: String?
        }
        guard let baseURL, let url = URL(string: "/api/v1/account/credentials", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "PATCH"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        request.httpBody = try JSONEncoder().encode(Payload(currentPassword: currentPassword, username: username, password: password))
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw responseError(data, statusCode: httpResponse.statusCode, fallback: "登录信息未保存。") }
    }

    func updateProfile(heightMM: Int?, targetWeightG: Int?, preferredUnit: String, timezone: String, accessToken: String) async throws -> ProfileResponse {
        struct Payload: Encodable { let heightMM: Int?; let targetWeightG: Int?; let preferredUnit: String; let timezone: String }
        guard let baseURL, let url = URL(string: "/api/v1/profile", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "PATCH"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        request.httpBody = try JSONEncoder().encode(Payload(heightMM: heightMM, targetWeightG: targetWeightG, preferredUnit: preferredUnit, timezone: timezone))
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else {
            throw APIError.rejected((try? JSONDecoder().decode(Problem.self, from: data))?.detail ?? "资料未保存。")
        }
        return try JSONDecoder().decode(ProfileResponse.self, from: data)
    }

    func deleteAccount(accessToken: String) async throws {
        guard let baseURL, let url = URL(string: "/api/v1/account", relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "DELETE"
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, httpResponse) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(httpResponse.statusCode) else { throw APIError.rejected((try? JSONDecoder().decode(Problem.self, from: data))?.detail ?? "账号删除未完成。") }
    }

    private func send<Body: Encodable, Response: Decodable>(path: String, body: Body) async throws -> Response {
        guard let baseURL, let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(body)
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let httpResponse = response as? HTTPURLResponse else { throw APIError.rejected("服务响应无效。") }
        guard (200...299).contains(httpResponse.statusCode) else {
            let problem = try? JSONDecoder().decode(Problem.self, from: data)
            throw APIError.rejected(problem?.detail ?? "请求未完成（\(httpResponse.statusCode)）。")
        }
        if Response.self == EmptyResponse.self { return EmptyResponse() as! Response }
        return try JSONDecoder().decode(Response.self, from: data)
    }

    private func sendAuthorized<Body: Encodable, Response: Decodable>(path: String, body: Body, accessToken: String) async throws -> Response {
        guard let baseURL, let url = URL(string: path, relativeTo: baseURL) else { throw APIError.invalidEndpoint }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        request.httpBody = try JSONEncoder().encode(body)
        let (data, response) = try await authorizedData(for: request, accessToken: accessToken)
        guard (200...299).contains(response.statusCode) else { throw responseError(data, statusCode: response.statusCode, fallback: "HealthKit 记录尚未同步。") }
        return try JSONDecoder().decode(Response.self, from: data)
    }

    func authorizedData(for request: URLRequest, accessToken: String) async throws -> (Data, HTTPURLResponse) {
        let first = try await execute(request)
        guard first.1.statusCode == 401 else { return first }
        var retry = request
        retry.setValue("Bearer \(try await SessionRefresher.shared.accessToken(replacing: accessToken))", forHTTPHeaderField: "Authorization")
        return try await execute(retry)
    }

    func execute(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let httpResponse = response as? HTTPURLResponse else { throw APIError.rejected("服务响应无效。") }
        return (data, httpResponse)
    }

    func responseError(_ data: Data, statusCode: Int, fallback: String) -> APIError {
        let problem = try? JSONDecoder().decode(Problem.self, from: data)
        let message: String
        if statusCode == 422 {
            let fields = problem?.errors?.compactMap(\.location).filter { !$0.isEmpty }.joined(separator: "、") ?? ""
            message = "请求字段校验失败（HTTP 422）" + (fields.isEmpty ? "。" : "：\(fields)。")
                + (problem?.errors?.compactMap(\.message).joined(separator: "；") ?? "")
        } else {
            message = problem?.detail ?? fallback
        }
        return statusCode == 409 ? .conflict(message) : .rejected(message)
    }
}

private struct Credentials: Encodable { let username: String; let password: String; let deviceLabel: String }
private struct EmptyResponse: Decodable {}
private struct HealthResponse: Decodable { let status: String }
private struct Problem: Decodable {
    let detail: String?
    let errors: [FieldError]?

    struct FieldError: Decodable {
        let message: String?
        let location: String?
    }
}
