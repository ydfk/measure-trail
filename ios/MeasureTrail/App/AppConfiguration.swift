import Foundation

enum AppConfiguration {
    static var apiBaseURL: URL? {
        #if DEBUG
        resolveAPIBaseURL(
            environmentOverride: ProcessInfo.processInfo.environment["MEASURETRAIL_API_BASE_URL"],
            bundledValue: Bundle.main.object(forInfoDictionaryKey: "MeasureTrailAPIBaseURL") as? String,
            isDevelopment: true
        )
        #else
        resolveAPIBaseURL(
            environmentOverride: nil,
            bundledValue: Bundle.main.object(forInfoDictionaryKey: "MeasureTrailAPIBaseURL") as? String,
            isDevelopment: false
        )
        #endif
    }

    static func resolveAPIBaseURL(environmentOverride: String?, bundledValue: String?, isDevelopment: Bool) -> URL? {
        // 正式包只信任打包地址，忽略历史手填地址及运行时环境覆盖。
        let value = isDevelopment ? (environmentOverride ?? bundledValue ?? "http://localhost:21000") : bundledValue
        guard let value else { return nil }
        return validatedAPIBaseURL(value, allowLocalHTTP: isDevelopment)
    }

    static func validatedAPIBaseURL(_ value: String, allowLocalHTTP: Bool = false) -> URL? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard var components = URLComponents(string: trimmed),
              let host = components.host, !host.isEmpty,
              components.user == nil, components.password == nil,
              components.query == nil, components.fragment == nil,
              components.path.isEmpty || components.path == "/" else { return nil }
        let isLocal = ["localhost", "127.0.0.1", "::1", "[::1]"].contains(host.lowercased()) || host.lowercased().hasSuffix(".local")
        guard components.scheme?.lowercased() == "https" ||
                (allowLocalHTTP && isLocal && components.scheme?.lowercased() == "http") else { return nil }
        components.path = ""
        return components.url
    }
}
