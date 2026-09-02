CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    openid TEXT NOT NULL,
    unionid TEXT,
    nickname TEXT,
    avatar_asset_id TEXT,
    last_login_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_openid_unique UNIQUE (openid)
);

CREATE INDEX IF NOT EXISTS idx_users_unionid ON users (unionid);
