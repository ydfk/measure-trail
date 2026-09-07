import Foundation
import Testing
@testable import MeasureTrail

struct PasskeyCodingTests {
    @Test func base64URLRoundTripUsesWebAuthnAlphabet() {
        let original = Data([0xfb, 0xff, 0xef, 0x01])
        let encoded = original.base64URLEncodedString

        #expect(encoded == "-__vAQ")
        #expect(Data(base64URLEncoded: encoded) == original)
    }

    @Test func decodesWebAuthnOptionEnvelopes() throws {
        let registration = try JSONDecoder().decode(APIClient.PasskeyRegistrationOptions.self, from: Data(#"{"sessionId":"registration-session","options":{"publicKey":{"challenge":"AQID","rp":{"id":"measure-trail.ydfk.site"},"user":{"id":"dXNlci0x","name":"admin"},"excludeCredentials":[{"id":"Y3JlZGVudGlhbA"}]}}}"#.utf8))
        let login = try JSONDecoder().decode(APIClient.PasskeyLoginOptions.self, from: Data(#"{"sessionId":"login-session","options":{"publicKey":{"challenge":"BAUG","rpId":"measure-trail.ydfk.site"}}}"#.utf8))

        #expect(registration.options.publicKey.rp.id == "measure-trail.ydfk.site")
        #expect(registration.options.publicKey.excludeCredentials?.first?.id == "Y3JlZGVudGlhbA")
        #expect(login.options.publicKey.rpId == "measure-trail.ydfk.site")
    }

    @Test func encodesAssertionUsingWebAuthnFieldNames() throws {
        let credential = APIClient.AssertionCredential(
            id: "credential",
            rawId: "credential",
            response: .init(clientDataJSON: "client", authenticatorData: "authenticator", signature: "signature", userHandle: "user")
        )
        let object = try #require(JSONSerialization.jsonObject(with: JSONEncoder().encode(credential)) as? [String: Any])
        let response = try #require(object["response"] as? [String: Any])

        #expect(object["rawId"] as? String == "credential")
        #expect(object["type"] as? String == "public-key")
        #expect(response["authenticatorData"] as? String == "authenticator")
        #expect(response["userHandle"] as? String == "user")
    }
}
