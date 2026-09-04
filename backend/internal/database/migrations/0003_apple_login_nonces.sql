CREATE TABLE apple_login_nonces (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX apple_login_nonces_expiry_idx ON apple_login_nonces(expires_at);
