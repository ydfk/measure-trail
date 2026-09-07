package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// 生成符合 WebAuthn none attestation 格式的测试凭据，走完整 HTTP 注册校验。
func registrationCredential(t *testing.T, challenge string) json.RawMessage {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := cbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1,
		-2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte("localhost"))
	authData := append([]byte{}, rpHash[:]...)
	// UP、UV、AT 标志，计数器为零，AAGUID 为零，凭据 ID 长度为 16。
	authData = append(authData, 0x45, 0, 0, 0, 0)
	authData = append(authData, make([]byte, 16)...)
	authData = append(authData, 0, 16)
	credentialID := make([]byte, 16)
	if _, err := rand.Read(credentialID); err != nil {
		t.Fatal(err)
	}
	authData = append(authData, credentialID...)
	authData = append(authData, publicKey...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "authData": authData, "attStmt": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	clientData, err := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": challenge, "origin": "http://localhost:21000"})
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	credential, err := json.Marshal(map[string]any{
		"id": encode(credentialID), "rawId": encode(credentialID), "type": "public-key",
		"response": map[string]string{"clientDataJSON": encode(clientData), "attestationObject": encode(attestation)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return credential
}
