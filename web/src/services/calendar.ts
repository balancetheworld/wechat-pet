import type { CalendarDay, CalendarMonth, CalendarRecord, CompleteCalendarReminderRequest, CompleteCalendarReminderResponse, CreateCalendarRecordRequest } from '../types/calendar'
import { request, uploadFile } from './request'

export function getCalendarMonth(month: string, petID?: string) {
  const query = petID ? `?pet_id=${encodeURIComponent(petID)}` : ''
  return request<CalendarMonth>({
    path: `/api/v1/calendar/months/${encodeURIComponent(month)}${query}`,
  })
}

export function getCalendarDay(date: string) {
  return request<CalendarDay>({
    path: `/api/v1/calendar/days/${encodeURIComponent(date)}`,
  })
}

export function createCalendarRecord(data: CreateCalendarRecordRequest) {
  return request<CalendarRecord>({
    path: '/api/v1/calendar/records',
    method: 'POST',
    data,
  })
}

/* 部分更新日历记录(内容/发生时间): 档案事件编辑后同步日历用 */
export function updateCalendarRecord(recordID: string, data: { content?: string, occurred_at?: string }) {
  return request<CalendarRecord>({
    path: `/api/v1/calendar/records/${encodeURIComponent(recordID)}`,
    method: 'PATCH',
    data,
  })
}

export function completeCalendarReminder(reminderID: string, data: CompleteCalendarReminderRequest) {
  return request<CompleteCalendarReminderResponse>({
    path: `/api/v1/calendar/reminders/${encodeURIComponent(reminderID)}/complete`,
    method: 'POST',
    data,
  })
}

export function uploadCalendarImage(filePath: string) {
  return uploadFile<{ asset_id: string }>({
    path: '/api/v1/assets/upload',
    filePath,
    name: 'file',
    formData: { type: 'calendar_image' },
  })
}
