CREATE TABLE IF NOT EXISTS calendar_records (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    pet_id TEXT NOT NULL,
    category TEXT NOT NULL,
    medical_type TEXT,
    content TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMP NOT NULL,
    occurred_on DATE NOT NULL,
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    CONSTRAINT calendar_records_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT calendar_records_pet_fk FOREIGN KEY (pet_id) REFERENCES pets (id),
    CONSTRAINT calendar_records_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id),
    CONSTRAINT calendar_records_updated_by_fk FOREIGN KEY (updated_by) REFERENCES users (id),
    CONSTRAINT calendar_records_category_check CHECK (category IN ('medical', 'daily')),
    CONSTRAINT calendar_records_medical_type_check CHECK (medical_type IS NULL OR medical_type IN ('vaccine', 'deworming', 'checkup', 'visit', 'medication', 'other'))
);

CREATE TABLE IF NOT EXISTS calendar_record_media (
    id TEXT PRIMARY KEY,
    record_id TEXT NOT NULL,
    family_id TEXT NOT NULL,
    asset_id TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT calendar_record_media_record_fk FOREIGN KEY (record_id) REFERENCES calendar_records (id),
    CONSTRAINT calendar_record_media_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT calendar_record_media_sort_order_check CHECK (sort_order > 0),
    CONSTRAINT calendar_record_media_record_order_unique UNIQUE (record_id, sort_order)
);

CREATE TABLE IF NOT EXISTS calendar_reminders (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    pet_id TEXT NOT NULL,
    source_record_id TEXT NOT NULL,
    previous_reminder_id TEXT,
    reminder_date DATE NOT NULL,
    repeat_type TEXT NOT NULL,
    repeat_interval_days INTEGER,
    advance_days INTEGER NOT NULL DEFAULT 3,
    notification_channels TEXT NOT NULL DEFAULT '["in_app"]',
    status TEXT NOT NULL DEFAULT 'pending',
    completed_at TIMESTAMP,
    completed_by TEXT,
    completed_record_id TEXT,
    created_by TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT calendar_reminders_family_fk FOREIGN KEY (family_id) REFERENCES families (id),
    CONSTRAINT calendar_reminders_pet_fk FOREIGN KEY (pet_id) REFERENCES pets (id),
    CONSTRAINT calendar_reminders_source_record_fk FOREIGN KEY (source_record_id) REFERENCES calendar_records (id),
    CONSTRAINT calendar_reminders_previous_fk FOREIGN KEY (previous_reminder_id) REFERENCES calendar_reminders (id),
    CONSTRAINT calendar_reminders_completed_record_fk FOREIGN KEY (completed_record_id) REFERENCES calendar_records (id),
    CONSTRAINT calendar_reminders_created_by_fk FOREIGN KEY (created_by) REFERENCES users (id),
    CONSTRAINT calendar_reminders_completed_by_fk FOREIGN KEY (completed_by) REFERENCES users (id),
    CONSTRAINT calendar_reminders_repeat_type_check CHECK (repeat_type IN ('once', 'monthly', 'yearly', 'custom_days')),
    CONSTRAINT calendar_reminders_repeat_interval_check CHECK ((repeat_type = 'custom_days' AND repeat_interval_days > 0) OR (repeat_type != 'custom_days' AND repeat_interval_days IS NULL)),
    CONSTRAINT calendar_reminders_advance_days_check CHECK (advance_days >= 0),
    CONSTRAINT calendar_reminders_status_check CHECK (status IN ('pending', 'completed'))
);

CREATE INDEX IF NOT EXISTS idx_calendar_records_family_day ON calendar_records (family_id, occurred_on, occurred_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_calendar_records_family_pet_day ON calendar_records (family_id, pet_id, occurred_on) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_calendar_record_media_record ON calendar_record_media (record_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_calendar_reminders_family_day ON calendar_reminders (family_id, reminder_date, status);
CREATE INDEX IF NOT EXISTS idx_calendar_reminders_family_pet_day ON calendar_reminders (family_id, pet_id, reminder_date, status);
