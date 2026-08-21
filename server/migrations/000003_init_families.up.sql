CREATE TABLE IF NOT EXISTS families (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS family_members (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT family_members_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT family_members_user_fk FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT family_members_role_check CHECK (role IN ('member', 'owner')),
    CONSTRAINT family_members_status_check CHECK (status IN ('active', 'inactive')),
    CONSTRAINT family_members_family_user_unique UNIQUE (family_id, user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_family_members_active_user ON family_members (user_id) WHERE status = 'active';
