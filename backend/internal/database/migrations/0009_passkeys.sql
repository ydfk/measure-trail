CREATE TABLE passkey_credentials (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    credential_id_hash TEXT NOT NULL UNIQUE,
    encrypted_credential BLOB NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    last_used_at DATETIME
);

CREATE INDEX passkey_credentials_user_id_idx ON passkey_credentials(user_id);
