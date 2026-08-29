CREATE TABLE IF NOT EXISTS pets (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    CONSTRAINT pets_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT pets_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id),
    CONSTRAINT pets_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users (id)
);

CREATE INDEX IF NOT EXISTS idx_pets_family_active ON pets (family_id, created_at) WHERE deleted_at IS NULL;
