export type CalendarRecordCategory = 'medical' | 'daily'
export type MedicalType = 'vaccine' | 'deworming' | 'checkup' | 'visit' | 'medication' | 'other'
export type ReminderRepeatType = 'once' | 'monthly' | 'yearly' | 'custom_days'
export type ReminderChannel = 'in_app' | 'push'

export interface CalendarPet {
  id: string
  name: string
  avatar_asset_id: string
}

export interface CalendarMember {
  user_id: string
  nickname: string
  avatar_asset_id: string
}

export interface CalendarMedia {
  id: string
  asset_id: string
  url?: string
  sort_order: number
}

export interface CalendarReminder {
  id: string
  source_record_id: string
  reminder_date: string
  pet: CalendarPet
  content: string
  medical_type: MedicalType | ''
  custom_medical_type: string
  created_by: CalendarMember
  repeat_type: ReminderRepeatType
  repeat_interval_days: number | null
  advance_days: number
  notification_channels: ReminderChannel[]
  status: 'pending' | 'completed'
}

export interface CalendarRecord {
  id: string
  category: CalendarRecordCategory
  medical_type: MedicalType | ''
  custom_medical_type: string
  content: string
  occurred_at: string
  pet: CalendarPet
  media: CalendarMedia[]
  created_by: CalendarMember
  reminder: CalendarReminder | null
  reminders: CalendarReminder[]
}

export interface CalendarDayMarker {
  date: string
  has_medical_record: boolean
  has_daily_record: boolean
  has_pending_reminder: boolean
}

export interface CalendarMonth {
  month: string
  pet_id: string
  days: CalendarDayMarker[]
}

export interface CalendarDay {
  date: string
  summary: {
    record_count: number
    pending_reminder_count: number
  }
  reminders: CalendarReminder[]
  records: CalendarRecord[]
}

export interface CreateCalendarReminderRequest {
  reminder_date: string
  repeat_type: ReminderRepeatType
  repeat_interval_days?: number
  advance_days?: number
  notification_channels?: ReminderChannel[]
}

export interface CreateCalendarRecordRequest {
  category: CalendarRecordCategory
  medical_type?: MedicalType
  custom_medical_type?: string
  pet_id: string
  content?: string
  media_asset_ids?: string[]
  occurred_at?: string
  reminder?: CreateCalendarReminderRequest
  reminders?: CreateCalendarReminderRequest[]
}

export interface CompleteCalendarReminderRequest {
  completed_at?: string
  content?: string
  media_asset_ids?: string[]
}

export interface CompleteCalendarReminderResponse {
  completed_record: CalendarRecord
  completed_reminder: CalendarReminder
  next_reminder: CalendarReminder | null
}
