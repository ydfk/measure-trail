CREATE TABLE measurement_changes (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    measurement_id TEXT NOT NULL REFERENCES measurements(id) ON DELETE CASCADE,
    changed_at TEXT NOT NULL
);

CREATE INDEX measurement_changes_user_sequence_idx ON measurement_changes(user_id, sequence);

INSERT INTO measurement_changes(user_id, measurement_id, changed_at)
SELECT user_id, id, updated_at FROM measurements;
