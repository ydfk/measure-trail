CREATE TABLE profiles (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    height_mm INTEGER CHECK (height_mm IS NULL OR height_mm > 0),
    target_weight_g INTEGER CHECK (target_weight_g IS NULL OR target_weight_g > 0),
    preferred_unit TEXT NOT NULL DEFAULT 'kg' CHECK (preferred_unit IN ('kg', 'jin')),
    timezone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
    updated_at TEXT NOT NULL
);

CREATE TABLE measurements (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recorded_on TEXT NOT NULL,
    weight_g INTEGER NOT NULL CHECK (weight_g > 0),
    waist_mm INTEGER CHECK (waist_mm IS NULL OR waist_mm > 0),
    note TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'legacy', 'healthkit')),
    healthkit_uuid TEXT,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    client_mutation_id TEXT,
    deleted_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(user_id, client_mutation_id)
);

CREATE UNIQUE INDEX measurements_active_date_idx ON measurements(user_id, recorded_on) WHERE deleted_at IS NULL;
CREATE INDEX measurements_user_updated_idx ON measurements(user_id, updated_at);
CREATE INDEX measurements_user_recorded_idx ON measurements(user_id, recorded_on DESC);
CREATE UNIQUE INDEX measurements_healthkit_uuid_idx ON measurements(user_id, healthkit_uuid) WHERE healthkit_uuid IS NOT NULL;

CREATE TABLE legacy_imports (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_sha256 TEXT NOT NULL,
    source_path TEXT NOT NULL,
    row_count INTEGER NOT NULL,
    imported_at TEXT NOT NULL,
    report_json TEXT NOT NULL,
    UNIQUE(user_id, source_sha256)
);
