CREATE TABLE IF NOT EXISTS assets (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    family_id TEXT,
    type TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT assets_user_fk FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT assets_family_fk FOREIGN KEY (family_id) REFERENCES families (id)
);

CREATE INDEX IF NOT EXISTS idx_assets_family ON assets (family_id);
CREATE INDEX IF NOT EXISTS idx_assets_user ON assets (user_id);
