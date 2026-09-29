DROP TABLE IF EXISTS wechat_subscribe_grants;
ALTER TABLE calendar_reminders DROP COLUMN notified_at;
ALTER TABLE calendar_reminders DROP COLUMN notify_attempts;
ALTER TABLE calendar_reminders DROP COLUMN notify_status;
