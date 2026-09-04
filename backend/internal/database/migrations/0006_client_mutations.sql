CREATE TABLE client_mutations (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mutation_id TEXT NOT NULL,
    measurement_id TEXT NOT NULL REFERENCES measurements(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY(user_id, mutation_id)
);

CREATE INDEX client_mutations_measurement_idx ON client_mutations(measurement_id);
