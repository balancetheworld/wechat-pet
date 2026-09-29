ALTER TABLE calendar_reminders ADD COLUMN notify_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE calendar_reminders ADD COLUMN notify_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE calendar_reminders ADD COLUMN notified_at TIMESTAMP;

CREATE TABLE IF NOT EXISTS wechat_subscribe_grants (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    openid TEXT NOT NULL,
    template_id TEXT NOT NULL,
    remaining INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT wechat_subscribe_grants_user_template_unique UNIQUE (user_id, template_id)
);
