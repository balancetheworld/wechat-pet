import type { CalendarDay, CalendarDayMarker, CalendarMedia, CalendarMonth, CalendarRecordCategory, MedicalType, ReminderRepeatType } from '../../types/calendar'
import type { Pet } from '../../types/pet'
import { Image, Input, Picker, ScrollView, Text, Textarea, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useCallback, useEffect, useRef, useState } from 'react'
import { FloatingGuide } from '../../components/floating-guide'
import PageBackground from '../../components/page-background'
import { routes } from '../../constants/routes'
import { completeCalendarReminder, createCalendarRecord, deleteCalendarRecord, getCalendarDay, getCalendarMonth, recordReminderSubscription, updateCalendarRecord, uploadCalendarImage } from '../../services/calendar'
import { createPetResource, getPetProfile, getPets } from '../../services/pet'
import { assetURL, authorizedAssetURL } from '../../services/request'
import { useAppStore } from '../../stores/app-store'
import { navigateTo } from '../../utils/navigation'
import './index.scss'

const weekDays = ['日', '一', '二', '三', '四', '五', '六']

/* 构建常量缺失(例如改了 config 没重启编译)时退化为不弹订阅, 避免保存流程报错 */
const reminderTemplateID = typeof TARO_APP_REMINDER_TEMPLATE_ID === 'string' ? TARO_APP_REMINDER_TEMPLATE_ID : ''

function pad(value: number) {
  return String(value).padStart(2, '0')
}

function formatDate(date: Date) {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function currentDate() {
  return formatDate(new Date())
}

function monthOf(date: string) {
  return date.slice(0, 7)
}

function dateForRecord(date: string) {
  if (date === currentDate()) {
    const value = new Date()
    return `${date}T${pad(value.getHours())}:${pad(value.getMinutes())}:${pad(value.getSeconds())}+09:00`
  }
  return `${date}T12:00:00+09:00`
}

function dateTitle(date: string) {
  const value = new Date(`${date}T12:00:00`)
  return `${value.getMonth() + 1}月${value.getDate()}日 · 星期${weekDays[value.getDay()]}`
}

function changeMonth(month: string, offset: number) {
  const value = new Date(`${month}-01T12:00:00`)
  value.setMonth(value.getMonth() + offset)
  return `${value.getFullYear()}-${pad(value.getMonth() + 1)}`
}

function monthDays(month: string) {
  const value = new Date(`${month}-01T12:00:00`)
  const year = value.getFullYear()
  const monthIndex = value.getMonth()
  const firstDay = new Date(year, monthIndex, 1).getDay()
  const count = new Date(year, monthIndex + 1, 0).getDate()
  return Array.from({ length: firstDay + count }, (_, index) => {
    if (index < firstDay) {
      return ''
    }
    return `${month}-${pad(index - firstDay + 1)}`
  })
}

function markerFor(markers: CalendarDayMarker[], date: string) {
  return markers.find(marker => marker.date === date)
}

function medicalTypeLabel(type: MedicalType | '', customType = '') {
  const labels: Record<MedicalType, string> = {
    vaccine: '疫苗',
    deworming: '驱虫',
    checkup: '体检',
    visit: '就诊',
    medication: '用药',
    other: '其他',
  }
  if (type === 'other' && customType) {
    return customType
  }
  return type ? labels[type] : ''
}

function repeatTypeLabel(type: ReminderRepeatType) {
  const labels: Record<ReminderRepeatType, string> = {
    once: '一次性',
    monthly: '每月',
    yearly: '每年',
    custom_days: '自定义',
  }
  return labels[type]
}

function mediaURL(media: CalendarMedia) {
  return media.url && /^https?:\/\//.test(media.url) ? authorizedAssetURL(media.url) : assetURL(media.asset_id)
}

function recordMedia(record: CalendarDay['records'][number]) {
  return record.media
    .slice()
    .sort((left, right) => left.sort_order - right.sort_order)
    .map((item: CalendarMedia) => ({ ...item, url: mediaURL(item) }))
    .filter(item => item.url)
}

function recordReminders(record: CalendarDay['records'][number]) {
  return record.reminders || (record.reminder ? [record.reminder] : [])
}

/* 订阅消息授权只能在用户点击事件里发起, 必须在任何 await 之前同步调用 */
function requestReminderSubscription() {
  if (!reminderTemplateID) {
    return Promise.resolve(false)
  }
  return new Promise<boolean>((resolve) => {
    /* Taro 4.2 的 Option 类型把支付宝专用字段 entityIds 标成必填, weapp 只传 tmplIds */
    Taro.requestSubscribeMessage({
      tmplIds: [reminderTemplateID],
      success: result => resolve(result[reminderTemplateID] === 'accept'),
      fail: () => resolve(false),
    } as unknown as Taro.requestSubscribeMessage.Option)
  })
}

export default function Calendar() {
  const today = currentDate()
  const [selectedDate, setSelectedDate] = useState(today)
  const [month, setMonth] = useState(() => monthOf(today))
  const [calendarExpanded, setCalendarExpanded] = useState(false)
  const [selectedPetID, setSelectedPetID] = useState('')
  const [pets, setPets] = useState<Pet[]>([])
  const [calendarMonth, setCalendarMonth] = useState<CalendarMonth>(() => ({ month: monthOf(today), pet_id: '', days: [] }))
  const [calendarDay, setCalendarDay] = useState<CalendarDay | null>(null)
  const [loading, setLoading] = useState(false)
  const [formVisible, setFormVisible] = useState(false)
  const [category, setCategory] = useState<CalendarRecordCategory>('daily')
  const [medicalType, setMedicalType] = useState<MedicalType>('vaccine')
  const [customMedicalType, setCustomMedicalType] = useState('')
  const [recordPetIDs, setRecordPetIDs] = useState<string[]>([])
  const [content, setContent] = useState('')
  const [mediaAssetIDs, setMediaAssetIDs] = useState<string[]>([])
  const [localImagePaths, setLocalImagePaths] = useState<string[]>([])
  const [uploading, setUploading] = useState(false)
  const [reminderEnabled, setReminderEnabled] = useState(false)
  const [reminders, setReminders] = useState<Array<{ reminderDate: string, repeatType: ReminderRepeatType, repeatIntervalDays: string }>>([])
  /* 同步到档案(可选): 'birthday' => 生日纪念页, 'growth' => 成长足迹页, 可多选 */
  const [syncTargets, setSyncTargets] = useState<string[]>([])
  /* 手动选中的日期要"粘住": 选图等系统界面返回会触发页面 onShow, 不能被 useDidShow 重置回今天 */
  const datePickedRef = useRef(false)
  const selectedDateRef = useRef(today)
  selectedDateRef.current = selectedDate
  /* 表单打开那一刻锁定的日期: 保存一律落到这个天, 中途页面 onShow 也不会漂移 */
  const formDateRef = useRef(today)
  /* "更多"菜单当前展开的记录 ID ('' = 全部收起) */
  const [moreOpenID, setMoreOpenID] = useState('')
  /* 编辑模式: 非空时表单为"修改"该条记录 (仅内容可改) */
  const [editingRecordID, setEditingRecordID] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [completingReminderID, setCompletingReminderID] = useState('')
  const setCalendarFormVisible = useAppStore(state => state.setCalendarFormVisible)

  const loadMonth = useCallback(async (targetMonth: string, petID = selectedPetID) => {
    const data = await getCalendarMonth(targetMonth, petID || undefined)
    setCalendarMonth(data)
  }, [selectedPetID])

  const loadDay = useCallback(async (date: string) => {
    const data = await getCalendarDay(date)
    setCalendarDay(data)
  }, [])

  const loadInitialData = useCallback(async (targetDate: string) => {
    setLoading(true)
    try {
      const petList = await getPets()
      setPets(petList)
      setSelectedPetID('')
      await Promise.all([
        getCalendarMonth(monthOf(targetDate)),
        getCalendarDay(targetDate),
      ]).then(([monthData, dayData]) => {
        setCalendarMonth(monthData)
        setCalendarDay(dayData)
      })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载日历失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }, [])

  useDidShow(() => {
    /* 手动选过日期则保持, 否则回落到今天 (避免从系统相册等界面返回时把选中天重置) */
    const target = datePickedRef.current ? selectedDateRef.current : today
    setSelectedDate(target)
    setMonth(monthOf(target))
    setCalendarExpanded(false)
    void loadInitialData(target)
    /* 首次登录悬浮猫指引: 引导页建完家庭后带着标记来到日历页, 在此消费并启动第 7 轮 */
    if (Taro.getStorageSync('pet-first-guide') === 'calendar') {
      Taro.removeStorageSync('pet-first-guide')
      useAppStore.getState().setGuideStage('calendar')
    }
  })

  useEffect(() => {
    setCalendarFormVisible(formVisible)
    Taro.eventCenter.trigger('calendar-form-visibility', formVisible)

    return () => {
      setCalendarFormVisible(false)
      Taro.eventCenter.trigger('calendar-form-visibility', false)
    }
  }, [formVisible, setCalendarFormVisible])

  async function refreshCurrentData(date = selectedDate, targetMonth = month) {
    await Promise.all([loadDay(date), loadMonth(targetMonth)])
  }

  async function handleToggleCalendar() {
    const nextValue = !calendarExpanded
    setCalendarExpanded(nextValue)
    if (!nextValue) {
      return
    }
    setLoading(true)
    try {
      await loadMonth(month)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载月历失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  async function handleSelectDate(date: string) {
    setLoading(true)
    try {
      await loadDay(date)
      datePickedRef.current = true
      setSelectedDate(date)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载日期详情失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  async function handleChangeMonth(offset: number) {
    const nextMonth = changeMonth(month, offset)
    setMonth(nextMonth)
    setLoading(true)
    try {
      await loadMonth(nextMonth)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载月历失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  async function handleToday() {
    const targetMonth = monthOf(today)
    setMonth(targetMonth)
    setLoading(true)
    try {
      const [monthData, dayData] = await Promise.all([
        getCalendarMonth(targetMonth, selectedPetID || undefined),
        getCalendarDay(today),
      ])
      setCalendarMonth(monthData)
      setCalendarDay(dayData)
      setSelectedDate(today)
      datePickedRef.current = false
      setCalendarExpanded(false)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '返回今天失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  async function handleSelectPet() {
    let result: Taro.showActionSheet.SuccessCallbackResult
    try {
      result = await Taro.showActionSheet({ itemList: ['全部宠物', ...pets.map(pet => pet.name)] })
    }
    catch {
      return
    }
    const petID = result.tapIndex === 0 ? '' : pets[result.tapIndex - 1]?.id
    if (petID === undefined) {
      return
    }
    setSelectedPetID(petID)
    setLoading(true)
    try {
      const data = await getCalendarMonth(month, petID || undefined)
      setCalendarMonth(data)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '筛选失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  function openRecordForm() {
    setCategory('daily')
    setMedicalType('vaccine')
    setCustomMedicalType('')
    setRecordPetIDs(pets.length === 1 ? [pets[0].id] : [])
    setContent('')
    setMediaAssetIDs([])
    setLocalImagePaths([])
    setReminderEnabled(false)
    setReminders([])
    /* 同步到档案: 每次打开表单重新勾选 */
    setSyncTargets([])
    setEditingRecordID('')
    /* 锁定表单所属日期: 保存落到打开表单时选中的那天 */
    formDateRef.current = selectedDate
    setFormVisible(true)
  }

  /* 修改模式打开表单: 预填当前记录; 分类/宠物锁定, 内容与图片可改 (图片整体替换) */
  function openEditRecord(record: CalendarDay['records'][number]) {
    setCategory(record.category)
    setMedicalType(record.medical_type || 'vaccine')
    setCustomMedicalType(record.custom_medical_type || '')
    setRecordPetIDs([record.pet.id])
    setContent(record.content || '')
    setMediaAssetIDs(record.media.map(item => item.asset_id))
    setLocalImagePaths(record.media.map(item => mediaURL(item)))
    setReminderEnabled(false)
    setReminders([])
    setSyncTargets([])
    setEditingRecordID(record.id)
    setFormVisible(true)
  }

  async function handleDeleteRecord(recordID: string) {
    const result = await Taro.showModal({
      title: '删除这条记录',
      content: '删除后这条记录将无法找回哦',
      confirmText: '确认',
      cancelText: '取消',
    })
    if (!result.confirm) {
      return
    }
    try {
      await deleteCalendarRecord(recordID)
      await refreshCurrentData()
      await Taro.showToast({ title: '已删除', icon: 'success' })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '删除记录失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
  }

  async function handleChooseImage() {
    try {
      const result = await Taro.chooseImage({
        count: Math.min(9 - localImagePaths.length, 9),
        sizeType: ['compressed'],
        sourceType: ['album', 'camera'],
      })
      if (!result.tempFilePaths.length) {
        return
      }
      setUploading(true)
      const uploaded = await Promise.all(result.tempFilePaths.map(filePath => uploadCalendarImage(filePath)))
      setMediaAssetIDs(previous => [...previous, ...uploaded.map(item => item.asset_id)])
      setLocalImagePaths(previous => [...previous, ...result.tempFilePaths])
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '图片上传失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setUploading(false)
    }
  }

  function handleRemoveImage(index: number) {
    setLocalImagePaths(previous => previous.filter((_, pathIndex) => pathIndex !== index))
    setMediaAssetIDs(previous => previous.filter((_, assetIndex) => assetIndex !== index))
  }

  /* 修改模式保存: 后端 PATCH 仅支持内容(与日期), 分类/图片等保持原样 */
  async function handleUpdateRecord() {
    if (submitting) {
      return
    }
    setSubmitting(true)
    try {
      const formDate = formDateRef.current
      await updateCalendarRecord(editingRecordID, { content: content.trim() || undefined })
      setFormVisible(false)
      setEditingRecordID('')
      await refreshCurrentData(formDate, monthOf(formDate))
      await Taro.showToast({ title: '记录已更新', icon: 'success' })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '更新记录失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setSubmitting(false)
    }
  }

  async function handleCreateRecord() {
    if (editingRecordID) {
      await handleUpdateRecord()
      return
    }
    if (!recordPetIDs.length || (category === 'daily' && !content.trim() && mediaAssetIDs.length === 0) || uploading || submitting || (category === 'medical' && medicalType === 'other' && !customMedicalType.trim()) || reminders.some(reminder => reminder.repeatType === 'custom_days' && Number(reminder.repeatIntervalDays) <= 0)) {
      return
    }
    const remindersRequested = category === 'medical' && reminderEnabled && reminders.length > 0
    /* 订阅弹窗必须在点击事件里同步发起, 结果留到保存成功后回传后端 */
    const subscriptionRequest = remindersRequested ? requestReminderSubscription() : Promise.resolve(false)
    setSubmitting(true)
    try {
      /* 保存目标日 = 打开表单时选中的那天 (表单打开瞬间锁定, 不受中途页面 onShow 影响) */
      const formDate = formDateRef.current
      const occurredAt = dateForRecord(formDate)
      let syncFailed = false
      for (const petID of recordPetIDs) {
        await createCalendarRecord({
          category,
          medical_type: category === 'medical' ? medicalType : undefined,
          custom_medical_type: category === 'medical' && medicalType === 'other' ? customMedicalType.trim() : undefined,
          pet_id: petID,
          content: content.trim() || undefined,
          media_asset_ids: mediaAssetIDs.length ? mediaAssetIDs : undefined,
          occurred_at: occurredAt,
          reminders: category === 'medical' && reminderEnabled
            ? reminders.map(reminder => ({
                reminder_date: reminder.reminderDate,
                repeat_type: reminder.repeatType,
                repeat_interval_days: reminder.repeatType === 'custom_days' ? Number(reminder.repeatIntervalDays) : undefined,
                advance_days: 3,
                notification_channels: ['in_app', 'push'],
              }))
            : undefined,
        })
        /* 同步到档案(可选): 勾选后把这条记录同时写进对应章节; 同步失败不影响日历记录本身 */
        if (syncTargets.length) {
          try {
            if (syncTargets.includes('growth')) {
              await createPetResource(petID, 'growth-events', {
                type: category === 'medical' ? (medicalTypeLabel(medicalType, customMedicalType.trim()) || '医疗') : '日常',
                occurred_at: occurredAt,
                content: content.trim() || '这一天发生了一件值得记录的事',
              })
            }
            if (syncTargets.includes('birthday')) {
              const year = Number.parseInt(formDate.slice(0, 4), 10)
              /* 年龄 = 记录日期时的实岁 (按宠物出生日期推算, 无生日则记 0) */
              let age = 0
              try {
                const profile = await getPetProfile(petID)
                if (profile?.birthday) {
                  const birthYear = Number.parseInt(profile.birthday.slice(0, 4), 10)
                  const birthMonth = Number.parseInt(profile.birthday.slice(5, 7), 10)
                  const birthDay = Number.parseInt(profile.birthday.slice(8, 10), 10)
                  const recordMonth = Number.parseInt(formDate.slice(5, 7), 10)
                  const recordDay = Number.parseInt(formDate.slice(8, 10), 10)
                  const computed = year - birthYear - ((recordMonth < birthMonth) || (recordMonth === birthMonth && recordDay < birthDay) ? 1 : 0)
                  if (Number.isFinite(computed) && computed >= 0) {
                    age = computed
                  }
                }
              }
              catch {}
              const createdBirthday = await createPetResource(petID, 'birthday-records', {
                year,
                age,
                summary: content.trim() || '这一天是值得纪念的日子',
              })
              /* 关联照片: 把日历记录选的图片同步写入 birthday-media, 生日纪念册大图卡才会显示对应照片 */
              const birthdayRecordID = typeof createdBirthday?.id === 'string' ? createdBirthday.id : ''
              if (birthdayRecordID && mediaAssetIDs.length) {
                await Promise.all(mediaAssetIDs.map(assetID => createPetResource(petID, 'birthday-media', {
                  record_id: birthdayRecordID,
                  type: 'photo',
                  asset_id: assetID,
                })))
              }
            }
          }
          catch {
            syncFailed = true
          }
        }
      }
      setFormVisible(false)
      /* 刷新回表单所属的那一天, 保证保存后视图仍停留在用户选中的天 */
      await refreshCurrentData(formDate, monthOf(formDate))
      const subscriptionAccepted = await subscriptionRequest
      let subscriptionRecorded = true
      if (subscriptionAccepted) {
        try {
          await recordReminderSubscription(true)
        }
        catch {
          subscriptionRecorded = false
        }
      }
      let toastTitle = '记录已保存'
      let toastIcon: 'success' | 'none' = 'success'
      if (syncFailed) {
        toastTitle = '日历已保存，档案同步失败'
        toastIcon = 'none'
      }
      else if (!subscriptionRecorded) {
        toastTitle = '已保存，微信提醒设置失败，请重试'
        toastIcon = 'none'
      }
      else if (remindersRequested && !subscriptionAccepted) {
        toastTitle = '已保存，未开启微信提醒'
        toastIcon = 'none'
      }
      await Taro.showToast({ title: toastTitle, icon: toastIcon })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '保存记录失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setSubmitting(false)
    }
  }

  async function handleCompleteReminder(reminderID: string) {
    if (completingReminderID) {
      return
    }
    setCompletingReminderID(reminderID)
    try {
      await completeCalendarReminder(reminderID, { completed_at: dateForRecord(selectedDate) })
      await refreshCurrentData()
      await Taro.showToast({ title: '待办已完成', icon: 'success' })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '完成待办失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setCompletingReminderID('')
    }
  }

  const selectedPet = pets.find(pet => pet.id === selectedPetID)
  const dayRecords = (calendarDay?.records.filter(record => !selectedPetID || record.pet.id === selectedPetID) || []).sort((left, right) => new Date(right.occurred_at).getTime() - new Date(left.occurred_at).getTime())
  const dayReminders = calendarDay?.reminders.filter(reminder => !selectedPetID || reminder.pet.id === selectedPetID) || []
  const hasRecords = Boolean(dayRecords.length || dayReminders.length)
  const canSubmit = Boolean(recordPetIDs.length && (category === 'medical' || content.trim() || mediaAssetIDs.length) && !uploading && !submitting && (category !== 'medical' || medicalType !== 'other' || customMedicalType.trim()) && !reminders.some(reminder => reminder.repeatType === 'custom_days' && Number(reminder.repeatIntervalDays) <= 0))

  return (
    <View className="cal-page">
      <PageBackground />
      <ScrollView className="cal-page-scroll" scrollY>
        <View className="cal-header">
        <View className="cal-header-left">
          <View className="cal-account" onClick={() => navigateTo(routes.pages.account)}>账户</View>
          <View className="cal-calendar-button" onClick={handleToggleCalendar}>{calendarExpanded ? '收起' : '日历'}</View>
        </View>
        <Text className="cal-title">宠物日历</Text>
        <View className="cal-header-spacer" />
        </View>

        {calendarExpanded && (
          <View className="cal-calendar-panel">
          <View className="cal-calendar-toolbar">
            <View className="cal-filter-button" onClick={handleSelectPet}>{selectedPet?.name || '全部宠物'}</View>
            <View className="cal-month-nav">
              <View className="cal-month-arrow" onClick={() => handleChangeMonth(-1)}>‹</View>
              <Text className="cal-month-title">
                {month.replace('-', ' 年 ')}
                月
              </Text>
              <View className="cal-month-arrow" onClick={() => handleChangeMonth(1)}>›</View>
            </View>
            <View className="cal-today-button" onClick={handleToday}>今天</View>
          </View>
          <View className="cal-week-row">
            {weekDays.map(day => <Text key={day}>{day}</Text>)}
          </View>
          <View className="cal-days-grid">
            {monthDays(month).map((date, index) => {
              const marker = date ? markerFor(calendarMonth.days, date) : undefined
              return (
                <View key={date || `blank-${index}`} className={`cal-day-cell${date === selectedDate ? ' selected' : ''}${marker?.has_pending_reminder ? ' reminder' : ''}`} onClick={() => date && handleSelectDate(date)}>
                  {date && <Text>{Number(date.slice(-2))}</Text>}
                  {date && marker && (marker.has_medical_record || marker.has_daily_record) && (
                    <View className="cal-marker-row">
                      {marker.has_medical_record && <View className="cal-marker medical" />}
                      {marker.has_daily_record && <View className="cal-marker daily" />}
                    </View>
                  )}
                </View>
              )
            })}
          </View>
          </View>
        )}

        {!calendarExpanded && (
          <View className="cal-grabber" onClick={handleToggleCalendar}>
            <View />
            <Text>上拉查看日历</Text>
          </View>
        )}

        <View className="cal-day-scroll">
        <View className="cal-day-card">
          <View className="cal-day-header">
            <View>
              <Text className="cal-day-title">
                {dateTitle(selectedDate)}
                {selectedDate === today ? ' · 今天' : ''}
              </Text>
              <Text className="cal-day-summary">{calendarDay ? `${dayRecords.length} 条记录 · ${dayReminders.length} 条待办提醒` : '加载中'}</Text>
            </View>
            {loading && <Text className="cal-loading">加载中</Text>}
          </View>
          <View className="cal-day-body">
            {dayReminders.map(reminder => (
              <View key={reminder.id} className="cal-reminder-card">
                <View className="cal-record-dot medical" />
                <View className="cal-record-main">
                  <View className="cal-record-head">
                    <Text className="cal-record-category reminder">待办提醒</Text>
                    <Text className="cal-record-pet">{reminder.pet.name}</Text>
                  </View>
                  <Text className="cal-record-content">{reminder.content || medicalTypeLabel(reminder.medical_type, reminder.custom_medical_type)}</Text>
                  <Text className="cal-record-meta">
                    记录者：
                    {reminder.created_by.nickname}
                    {' · '}
                    {repeatTypeLabel(reminder.repeat_type)}
                  </Text>
                </View>
                <View className="cal-complete-button" onClick={() => handleCompleteReminder(reminder.id)}>{completingReminderID === reminder.id ? '处理中' : '完成'}</View>
              </View>
            ))}
            {dayRecords.map((record) => {
              const media = recordMedia(record)
              const mediaURLs = media.map(item => item.url)
              const recordReminderList = recordReminders(record)
              return (
                <View key={record.id} className="cal-record-row">
                  <View className={`cal-record-dot ${record.category}`} />
                  <View className="cal-record-main">
                    <View className="cal-record-head">
                      <Text className={`cal-record-category ${record.category}`}>{record.category === 'medical' ? '医疗' : '日常'}</Text>
                      {record.medical_type && <Text className="cal-medical-type">{medicalTypeLabel(record.medical_type, record.custom_medical_type)}</Text>}
                      <Text className="cal-record-pet">{record.pet.name}</Text>
                      <View className="cal-record-more" onClick={() => setMoreOpenID(moreOpenID === record.id ? '' : record.id)}>⋯</View>
                    </View>
                    {record.content && <Text className="cal-record-content">{record.content}</Text>}
                    {media.length > 0 && (
                      <View className={`cal-record-images count-${media.length}`}>
                        {media.map(image => (
                          <Image
                            key={image.id}
                            className="cal-record-image"
                            src={image.url}
                            mode="aspectFill"
                            onClick={() => Taro.previewImage({ current: image.url, urls: mediaURLs })}
                          />
                        ))}
                      </View>
                    )}
                    {recordReminderList.map(reminder => (
                      <Text key={reminder.id} className="cal-record-reminder">
                        提醒日
                        {reminder.reminder_date}
                        {' · '}
                        {repeatTypeLabel(reminder.repeat_type)}
                      </Text>
                    ))}
                    <Text className="cal-record-meta">
                      记录者：
                      {record.created_by.nickname}
                    </Text>
                  </View>
                  {/* 从右上角 ⋯ 延伸出的小列表 */}
                  {moreOpenID === record.id && (
                    <>
                      <View className="cal-more-mask" onClick={() => setMoreOpenID('')} />
                      <View className="cal-more-menu">
                        <View
                          className="cal-more-item"
                          onClick={() => {
                            setMoreOpenID('')
                            openEditRecord(record)
                          }}
                        >
修改
                        </View>
                        <View
                          className="cal-more-item danger"
                          onClick={() => {
                            setMoreOpenID('')
                            void handleDeleteRecord(record.id)
                          }}
                        >
删除
                        </View>
                      </View>
                    </>
                  )}
                </View>
              )
            })}
            {!loading && !hasRecords && (
              <View className="cal-empty">
                <Text className="cal-empty-title">这一天还没有记录</Text>
                <Text className="cal-empty-text">写下关于毛孩子的一件小事</Text>
              </View>
            )}
          </View>
        </View>
        </View>

        <View className="cal-add-button" hoverClass="cal-add-button-hover" onClick={openRecordForm}>
          <View className="cal-add-icon">＋</View>
          <Text>添加记录</Text>
        </View>

        {formVisible && (
          <View className="cal-overlay">
          <View className="cal-sheet">
            <View className="cal-sheet-handle" />
            <Text className="cal-sheet-title">{editingRecordID ? '修改记录' : '添加记录'}</Text>
            <Text className="cal-field-label">分类</Text>
            <View className="cal-segments">
              <View className={`cal-segment${category === 'daily' ? ' selected' : ''}${editingRecordID ? ' locked' : ''}`} onClick={editingRecordID ? undefined : () => setCategory('daily')}>日常</View>
              <View className={`cal-segment${category === 'medical' ? ' selected medical' : ''}${editingRecordID ? ' locked' : ''}`} onClick={editingRecordID ? undefined : () => setCategory('medical')}>医疗</View>
            </View>
            {!editingRecordID && (
              <>
                <Text className="cal-field-label">同步到档案（可选）</Text>
                <View className="cal-pet-chips">
                  {[{ key: 'growth', label: '成长足迹页' }, { key: 'birthday', label: '生日纪念页' }].map(target => (
                    <View
                      key={target.key}
                      className={`cal-pet-chip${syncTargets.includes(target.key) ? ' selected' : ''}`}
                      onClick={() => setSyncTargets(value => value.includes(target.key) ? value.filter(item => item !== target.key) : [...value, target.key])}
                    >
{target.label}
                    </View>
                  ))}
                </View>
              </>
            )}
            <Text className="cal-field-label">宠物</Text>
            <View className="cal-pet-chips">
              {pets.map(pet => <View key={pet.id} className={`cal-pet-chip${recordPetIDs.includes(pet.id) ? ' selected' : ''}${editingRecordID ? ' locked' : ''}`} onClick={editingRecordID ? undefined : () => setRecordPetIDs(value => value.includes(pet.id) ? value.filter(id => id !== pet.id) : [...value, pet.id])}>{pet.name}</View>)}
            </View>
            {category === 'medical' && (
              <>
                <Text className="cal-field-label">医疗类型</Text>
                <View className="cal-type-chips">
                  {(['vaccine', 'deworming', 'checkup', 'visit', 'medication', 'other'] as MedicalType[]).map(type => <View key={type} className={`cal-type-chip${medicalType === type ? ' selected' : ''}`} onClick={() => setMedicalType(type)}>{medicalTypeLabel(type)}</View>)}
                </View>
                {medicalType === 'other' && <Input className="cal-custom-medical-type" value={customMedicalType} maxlength={50} placeholder="请输入医疗类型" onInput={event => setCustomMedicalType(event.detail.value)} />}
              </>
            )}
            <Text className="cal-field-label">记录内容</Text>
            <Textarea className="cal-textarea" value={content} maxlength={1000} placeholder={category === 'medical' ? '医院、药品等需要备注的写在这里哦～' : '写下今天发生的事'} onInput={event => setContent(event.detail.value)} />
            <View className="cal-image-actions">
              <View className="cal-image-button" onClick={handleChooseImage}>{uploading ? '上传中' : '添加图片'}</View>
              {localImagePaths.length > 0 && (
                <Text>
                  已选择
                  {` ${localImagePaths.length}/9 张`}
                </Text>
              )}
            </View>
            {localImagePaths.length > 0 && (
              <View className="cal-image-thumbs">
                {localImagePaths.map((path, index) => (
                  <View className="cal-image-thumb" key={`${path}-${index}`}>
                    <Image
                      className="cal-image-thumb-img"
                      src={path}
                      mode="aspectFill"
                      onClick={() => void Taro.previewImage({ urls: localImagePaths, current: path })}
                    />
                    <View className="cal-image-thumb-remove" onClick={() => handleRemoveImage(index)}>×</View>
                  </View>
                ))}
              </View>
            )}
            {category === 'medical' && (
              <View className="cal-reminder-section">
                <View className="cal-reminder-switch-row">
                  <Text className="cal-field-label">待办提醒</Text>
                  <View className={`cal-switch${reminderEnabled ? ' selected' : ''}`} onClick={() => setReminderEnabled(value => !value)}><View /></View>
                </View>
                {reminderEnabled && (
                  <View className="cal-reminder-fields">
                    <Picker mode="date" value={selectedDate} onChange={event => setReminders(value => value.some(reminder => reminder.reminderDate === event.detail.value) ? value : [...value, { reminderDate: event.detail.value, repeatType: 'yearly', repeatIntervalDays: '30' }])}>
                      <View className="cal-picker-row">
                        <Text>添加提醒日期</Text>
                        <Text>选择</Text>
                      </View>
                    </Picker>
                    {reminders.map((reminder, index) => (
                      <View className="cal-reminder-item" key={reminder.reminderDate}>
                        <View className="cal-reminder-item-head">
                          <Text>
                            下次提醒：
                            {reminder.reminderDate}
                          </Text>
                          <Text className="cal-reminder-remove" onClick={() => setReminders(value => value.filter((_, reminderIndex) => reminderIndex !== index))}>删除</Text>
                        </View>
                        <View className="cal-repeat-chips">
                          {(['once', 'monthly', 'yearly', 'custom_days'] as ReminderRepeatType[]).map(type => <View key={type} className={`cal-repeat-chip${reminder.repeatType === type ? ' selected' : ''}`} onClick={() => setReminders(value => value.map((item, reminderIndex) => reminderIndex === index ? { ...item, repeatType: type } : item))}>{repeatTypeLabel(type)}</View>)}
                        </View>
                        {reminder.repeatType === 'custom_days' && <Input className="cal-custom-days" type="number" value={reminder.repeatIntervalDays} placeholder="间隔天数" onInput={event => setReminders(value => value.map((item, reminderIndex) => reminderIndex === index ? { ...item, repeatIntervalDays: event.detail.value } : item))} />}
                      </View>
                    ))}
                  </View>
                )}
              </View>
            )}
            <View className="cal-sheet-actions">
              <View
                className="cal-cancel-button"
                onClick={() => {
                  setEditingRecordID('')
                  setFormVisible(false)
                }}
              >
取消
              </View>
              <View className={`cal-save-button${canSubmit ? '' : ' disabled'}`} onClick={handleCreateRecord}>{submitting ? '保存中' : '保存'}</View>
            </View>
          </View>
          </View>
        )}
      </ScrollView>
      <FloatingGuide />
    </View>
  )
}
