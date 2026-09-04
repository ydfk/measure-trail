import Foundation
import Security

struct TokenStore {
    private let service = "com.ydfk.MeasureTrail"
    private let account = "session"

    var hasSession: Bool { load() != nil }

    func session() -> (accessToken: String, refreshToken: String)? {
        guard let data = load(), let value = String(data: data, encoding: .utf8) else { return nil }
        let values = value.split(separator: "\n", maxSplits: 1).map(String.init)
        guard values.count == 2 else { return nil }
        return (values[0], values[1])
    }

    func save(accessToken: String, refreshToken: String) throws {
        let payload = "\(accessToken)\n\(refreshToken)"
        let data = Data(payload.utf8)
        clear()
        let status = SecItemAdd([kSecClass: kSecClassGenericPassword, kSecAttrService: service, kSecAttrAccount: account, kSecValueData: data] as CFDictionary, nil)
        guard status == errSecSuccess else { throw TokenStoreError.unexpectedStatus(status) }
    }

    func clear() { SecItemDelete([kSecClass: kSecClassGenericPassword, kSecAttrService: service, kSecAttrAccount: account] as CFDictionary) }

    private func load() -> Data? {
        var result: CFTypeRef?
        let status = SecItemCopyMatching([kSecClass: kSecClassGenericPassword, kSecAttrService: service, kSecAttrAccount: account, kSecReturnData: true] as CFDictionary, &result)
        return status == errSecSuccess ? result as? Data : nil
    }
}

enum TokenStoreError: Error { case unexpectedStatus(OSStatus) }

actor SessionRefresher {
    static let shared = SessionRefresher()

    func accessToken(replacing expiredToken: String) async throws -> String {
        let store = TokenStore()
        guard let session = store.session() else { throw APIClient.APIError.rejected("登录已失效，请重新登录。") }
        if session.accessToken != expiredToken { return session.accessToken }
        let refreshed = try await APIClient().refresh(refreshToken: session.refreshToken)
        try store.save(accessToken: refreshed.accessToken, refreshToken: refreshed.refreshToken)
        return refreshed.accessToken
    }
}
