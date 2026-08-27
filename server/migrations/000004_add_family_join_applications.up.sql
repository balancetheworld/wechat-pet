ALTER TABLE families ADD COLUMN code TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_families_code ON families (code);

CREATE TABLE IF NOT EXISTS family_join_applications (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT family_join_applications_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT family_join_applications_user_fk FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT family_join_applications_status_check CHECK (status IN ('pending', 'active', 'rejected')),
    CONSTRAINT family_join_applications_family_user_unique UNIQUE (family_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_family_join_applications_family_status ON family_join_applications (family_id, status);
