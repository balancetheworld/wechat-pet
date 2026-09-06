import type { CalendarRecordCategory, MedicalType } from '../../types/calendar'
import type { Pet, PetProfile } from '../../types/pet'
import { Button, Image, Input, Picker, ScrollView, Text, Textarea, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useCallback, useEffect, useMemo, useState } from 'react'
import backgroundImage from '../../assets/background1.png'
import passportImage from '../../assets/passport.png'
import { routes } from '../../constants/routes'
import { createCalendarRecord } from '../../services/calendar'
import { getPetProfile, getPetResource, getPets } from '../../services/pet'
import { usePetStore } from '../../stores/pet-store'
import { navigateTo } from '../../utils/navigation'
import './index.scss'
import '../manual.scss'

interface PersonalityItem {
  id: string
  trait: string
  value: string
}

interface QuestionItem {
  id: string
  question: string
  answer: string
}

interface BirthdayRecord {
  id: string
  year: number
  age: number
  summary: string
}

interface WeightRecord {
  id: string
  measured_at: string
  weight: number
}

interface GrowthEvent {
  id: string
  type: string
  occurred_at: string
  recorder: string
  content: string
}

/* 章节（书固定 7 个章节；内容多的章节自动拆成多页） */
const CHAPTERS = [
  { key: 'cover', name: '封面' },
  { key: 'identity', name: '身份名片' },
  { key: 'personality', name: '个性说明书' },
  { key: 'health', name: '健康资料' },
  { key: 'birthday', name: '生日纪念册' },
  { key: 'growth', name: '成长足迹' },
  { key: 'back', name: '封底' },
] as const

type ChapterKey = typeof CHAPTERS[number]['key']

/* 每页可容纳的条数（超出自动开新页） */
const PERSONALITY_PER_PAGE = 3
const BIRTHDAY_PER_PAGE = 3
const GROWTH_FIRST_PAGE_EVENTS = 2
const GROWTH_PER_PAGE = 3

/* 书本长宽比（拉长版） */
const BOOK_RATIO = '1086 / 1620'

/* 医疗类型标签(对应日历 GrowthEvent 标题) */
const MEDICAL_TYPE_LABEL: Record<MedicalType, string> = {
  vaccine: '疫苗',
  deworming: '驱虫',
  checkup: '体检',
  visit: '就诊',
  medication: '用药',
  other: '其他',
}

interface BookPage {
  chapter: ChapterKey
  name: string
  part: number
  parts: number
}

function formatDate(value?: string) {
  return value || '暂无记录'
}

function formatGender(value: string) {
  if (value === 'male') {
    return '男孩'
  }
  if (value === 'female') {
    return '女孩'
  }
  return value || '暂无记录'
}

export default function Profile() {
  const pets = usePetStore(state => state.pets)
  const currentPetId = usePetStore(state => state.currentPetId)
  const setPets = usePetStore(state => state.setPets)
  const setCurrentPetId = usePetStore(state => state.setCurrentPetId)
  const [profile, setProfile] = useState<PetProfile | null>(null)
  const [personality, setPersonality] = useState<PersonalityItem[]>([])
  const [questions, setQuestions] = useState<QuestionItem[]>([])
  const [birthdayRecords, setBirthdayRecords] = useState<BirthdayRecord[]>([])
  const [weights, setWeights] = useState<WeightRecord[]>([])
  const [growthEvents, setGrowthEvents] = useState<GrowthEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [currentPage, setCurrentPage] = useState(0)
  const [touchStartX, setTouchStartX] = useState<number | null>(null)
  const [touchDeltaX, setTouchDeltaX] = useState(0)
  const [turning, setTurning] = useState(false)
  const [turnDirection, setTurnDirection] = useState<'next' | 'previous' | null>(null)
  const [turnTargetPage, setTurnTargetPage] = useState<number | null>(null)
  const [resetting, setResetting] = useState(false)
  /* 目录弹层 */
  const [tocOpen, setTocOpen] = useState(false)
  /* 详情弹层 */
  const [detail, setDetail] = useState<{ title: string, subtitle: string, body: string } | null>(null)
  /* 宠物切换弹层 */
  const [switcherOpen, setSwitcherOpen] = useState(false)
  /* 成长足迹添加事件弹层 */
  const [growthFormVisible, setGrowthFormVisible] = useState(false)
  const [growthFormCategory, setGrowthFormCategory] = useState<CalendarRecordCategory>('daily')
  const [growthFormMedicalType, setGrowthFormMedicalType] = useState<MedicalType>('checkup')
  const [growthFormCustomMedicalType, setGrowthFormCustomMedicalType] = useState('')
  const [growthFormContent, setGrowthFormContent] = useState('')
  const [growthFormDate, setGrowthFormDate] = useState<string>(() => {
    const d = new Date()
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  })
  const [growthFormPetID, setGrowthFormPetID] = useState('')
  const [growthSubmitting, setGrowthSubmitting] = useState(false)

  const selectedPet = pets.find(item => item.id === currentPetId) || pets[0]

  /* ===== 动态分页：按数据量把每个章节拆成若干页 ===== */
  const pages = useMemo<BookPage[]>(() => {
    const list: BookPage[] = []
    const personalityParts = Math.max(1, Math.ceil(questions.length / PERSONALITY_PER_PAGE))
    const birthdayParts = Math.max(1, Math.ceil(birthdayRecords.length / BIRTHDAY_PER_PAGE))
    const growthParts = growthEvents.length <= GROWTH_FIRST_PAGE_EVENTS
      ? 1
      : 1 + Math.ceil((growthEvents.length - GROWTH_FIRST_PAGE_EVENTS) / GROWTH_PER_PAGE)

    const push = (chapter: ChapterKey, name: string, part: number, parts: number) => {
      list.push({ chapter, name, part, parts })
    }

    push('cover', '封面', 1, 1)
    push('identity', '身份名片', 1, 1)
    for (let i = 1; i <= personalityParts; i++) push('personality', '个性说明书', i, personalityParts)
    push('health', '健康资料', 1, 1)
    for (let i = 1; i <= birthdayParts; i++) push('birthday', '生日纪念册', i, birthdayParts)
    for (let i = 1; i <= growthParts; i++) push('growth', '成长足迹', i, growthParts)
    push('back', '封底', 1, 1)
    return list
  }, [questions.length, birthdayRecords.length, growthEvents.length])

  const pageCount = pages.length

  /* 数据变化导致页数变少时，收回越界的当前页 */
  useEffect(() => {
    if (currentPage > pageCount - 1) {
      setCurrentPage(0)
    }
  }, [pageCount, currentPage])

  const loadProfile = useCallback(async (pet: Pet) => {
    setLoading(true)
    try {
      const petProfile = await getPetProfile(pet.id)
      const [personalityItems, questionItems, records, weightItems, eventItems] = await Promise.all([
        getPetResource<PersonalityItem[]>(pet.id, 'personality'),
        getPetResource<QuestionItem[]>(pet.id, 'questions'),
        getPetResource<BirthdayRecord[]>(pet.id, 'birthday-records'),
        getPetResource<WeightRecord[]>(pet.id, 'weights'),
        getPetResource<GrowthEvent[]>(pet.id, 'growth-events'),
      ])
      setProfile(petProfile)
      setPersonality(personalityItems)
      setQuestions(questionItems)
      setBirthdayRecords(records)
      setWeights(weightItems)
      setGrowthEvents(eventItems)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载档案失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    async function load() {
      let pet = selectedPet
      if (!pet) {
        try {
          const petList = await getPets()
          setPets(petList)
          pet = petList[0]
        }
        catch (error) {
          const message = error instanceof Error ? error.message : '加载宠物失败'
          await Taro.showToast({ title: message, icon: 'none' })
          setLoading(false)
          return
        }
      }
      if (pet) {
        await loadProfile(pet)
        return
      }
      setLoading(false)
    }

    void load()
  }, [loadProfile, selectedPet, setPets])

  const goToPage = useCallback((page: number) => {
    setCurrentPage(Math.max(0, Math.min(pageCount - 1, page)))
  }, [pageCount])

  const handleTouchStart = useCallback((event: any) => {
    if (turning) {
      return
    }
    setTouchStartX(event.touches[0].clientX)
    setTouchDeltaX(0)
    setTurnDirection(null)
  }, [turning])

  const handleTouchMove = useCallback((event: any) => {
    if (touchStartX === null) {
      return
    }
    const deltaX = event.touches[0].clientX - touchStartX
    if ((deltaX < 0 && currentPage >= pageCount - 1) || (deltaX > 0 && currentPage <= 0)) {
      setTouchDeltaX(0)
      setTurnDirection(null)
      return
    }
    setTouchDeltaX(Math.max(-360, Math.min(360, deltaX)))
    setTurnDirection(deltaX < 0 ? 'next' : 'previous')
  }, [currentPage, pageCount, touchStartX])

  const handleTouchEnd = useCallback((event: any) => {
    if (touchStartX === null) {
      return
    }
    const deltaX = event.changedTouches[0].clientX - touchStartX
    setTouchStartX(null)
    const rotation = Math.max(-180, Math.min(180, deltaX * 0.5))
    const canTurnNext = rotation <= -35 && currentPage < pageCount - 1
    const canTurnPrevious = rotation >= 35 && currentPage > 0
    if (!canTurnNext && !canTurnPrevious) {
      setTouchDeltaX(0)
      setTurnDirection(null)
      return
    }
    const direction = canTurnNext ? 'next' : 'previous'
    const targetPage = currentPage + (direction === 'next' ? 1 : -1)
    setTurning(true)
    setTurnDirection(direction)
    setTurnTargetPage(targetPage)
    setTouchDeltaX(direction === 'next' ? -360 : 360)
    setTimeout(() => {
      setResetting(true)
      goToPage(targetPage)
      setTouchDeltaX(0)
      setTimeout(() => {
        setTurnTargetPage(null)
        setTurnDirection(null)
        setResetting(false)
        setTurning(false)
      }, 16)
    }, 260)
  }, [currentPage, goToPage, pageCount, touchStartX])

  const openDetail = (title: string, subtitle: string, body: string) => setDetail({ title, subtitle, body })

  const handleSwitchPet = useCallback((petId: string) => {
    if (petId === currentPetId) {
      setSwitcherOpen(false)
      return
    }
    setSwitcherOpen(false)
    setCurrentPetId(petId)
    /* 切回第 1 页（封面）等待新档案加载 */
    setCurrentPage(0)
  }, [currentPetId, setCurrentPetId])

  /* ===== 成长足迹添加事件 ===== */
  function openGrowthForm() {
    const targetPetID = currentPetId || pets[0]?.id || ''
    const today = new Date()
    const todayStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
    setGrowthFormCategory('daily')
    setGrowthFormMedicalType('checkup')
    setGrowthFormCustomMedicalType('')
    setGrowthFormContent('')
    setGrowthFormDate(todayStr)
    setGrowthFormPetID(targetPetID)
    setGrowthFormVisible(true)
  }

  function closeGrowthForm() {
    setGrowthFormVisible(false)
  }

  async function handleCreateGrowthEvent() {
    if (growthSubmitting) {
      return
    }
    const petID = growthFormPetID
    if (!petID) {
      await Taro.showToast({ title: '请选择宠物', icon: 'none' })
      return
    }
    if (growthFormCategory === 'daily' && !growthFormContent.trim()) {
      await Taro.showToast({ title: '请写点内容', icon: 'none' })
      return
    }
    if (growthFormCategory === 'medical' && growthFormMedicalType === 'other' && !growthFormCustomMedicalType.trim()) {
      await Taro.showToast({ title: '请填写医疗类型', icon: 'none' })
      return
    }
    setGrowthSubmitting(true)
    const occurredAt = `${growthFormDate}T12:00:00+09:00`
    /* 同步到日历所需字段: type 标题 */
    const typeLabel = growthFormCategory === 'medical'
      ? (growthFormMedicalType === 'other' ? growthFormCustomMedicalType.trim() : MEDICAL_TYPE_LABEL[growthFormMedicalType])
      : '日常'
    try {
      /* 1) 写入后端日历记录(同步到日历) */
      await createCalendarRecord({
        category: growthFormCategory,
        medical_type: growthFormCategory === 'medical' ? growthFormMedicalType : undefined,
        custom_medical_type: growthFormCategory === 'medical' && growthFormMedicalType === 'other'
          ? growthFormCustomMedicalType.trim()
          : undefined,
        pet_id: petID,
        content: growthFormContent.trim() || undefined,
        occurred_at: occurredAt,
      })
      /* 2) 立即在档案页成长足迹中追加一条(乐观更新,无需等后端推送) */
      const newEvent: GrowthEvent = {
        id: `local-${Date.now()}`,
        type: typeLabel,
        occurred_at: occurredAt,
        recorder: '我',
        content: growthFormContent.trim() || (growthFormCategory === 'medical' ? '已记录医疗事项' : '已记录今日小事'),
      }
      setGrowthEvents(previous => [newEvent, ...previous])
      setGrowthFormVisible(false)
      await Taro.showToast({ title: '已记一笔', icon: 'success' })
      /* 3) 后台静默重拉一次成长足迹,以防后端做了转换映射 */
      void getPetResource<GrowthEvent[]>(petID, 'growth-events')
        .then(fresh => setGrowthEvents(fresh))
        .catch(() => {})
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '保存事件失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setGrowthSubmitting(false)
    }
  }

  /* ===== 翻页辅助 ===== */
  const adjacentPage = turnTargetPage ?? (turnDirection === 'previous'
    ? (currentPage > 0 ? currentPage - 1 : null)
    : (currentPage < pageCount - 1 ? currentPage + 1 : null))

  /* ===== 页面: 封面 ===== */
  const renderCover = () => (
    <View
      className="page cover-page"
      onClick={() => goToPage(1)}
      style={{ backgroundImage: `url(${passportImage})` }}
    />
  )

  /* ===== 章节: 身份名片 ===== */
  const renderProfilePage = () => (
    <View className="page content-page">
      <View className="page-body">
        <View className="page-head">
          <Text className="page-eyebrow">PROFILE</Text>
          <Text className="page-title">{profile?.name || selectedPet?.name || '身份名片'}</Text>
          <Text className="page-subtitle">它的基本档案，一页看全</Text>
        </View>
        <View className="identity-hero">
          <Button className="identity-photo" onClick={() => openDetail('头像', '点击上传新头像', '在这里可以上传或更换宠物的头像照片，作为这本档案的封面留念。')}>
            {profile?.name?.slice(0, 1) || '宠'}
          </Button>
          <View className="identity-meta">
            <Text className="identity-name">{profile?.name || selectedPet?.name || '宠'}</Text>
            <Text className="identity-type">
{profile?.breed || '品种待补充'}
{' '}
·
{' '}
{formatGender(profile?.gender || '')}
            </Text>
          </View>
        </View>
        <View className="stat-row">
          <View className="stat">
            <Text className="strong">{profile?.birthday ? `${profile.age}` : '—'}</Text>
            <Text className="span">当前年龄（岁）</Text>
          </View>
          <View className="stat">
            <Text className="strong">{profile?.home_date ? `${profile.companion_days.toLocaleString()}` : '—'}</Text>
            <Text className="span">陪伴天数</Text>
          </View>
          <View className="stat">
            <Text className="strong">{profile?.next_birthday_days === undefined ? '—' : `${profile.next_birthday_days}`}</Text>
            <Text className="span">下次生日（天）</Text>
          </View>
        </View>
        <View className="info-list">
          <View className="info-row info-static">
            <Text className="span">出生信息</Text>
            <Text className="strong">{formatDate(profile?.birthday)}</Text>
          </View>
          <View className="info-row info-static">
            <Text className="span">到家日期</Text>
            <Text className="strong">{formatDate(profile?.home_date)}</Text>
          </View>
          <Button className="info-row" onClick={() => openDetail('身份与证件', '已收纳 2 项', '在这里集中管理宠物的疫苗本、芯片号、繁育证明等证件信息，仅家庭成员可见。')}>
            <Text className="span">身份与证件</Text>
            <Text className="strong">已收纳 2 项</Text>
          </Button>
        </View>
      </View>
    </View>
  )

  /* ===== 章节: 个性说明书（每页 3 条问答，超出自动开新页） ===== */
  const renderPersonalityPage = (part: number) => {
    const start = (part - 1) * PERSONALITY_PER_PAGE
    const partQuestions = questions.slice(start, start + PERSONALITY_PER_PAGE)
    return (
      <View className="page content-page">
        <View className="page-body">
          <View className="page-head">
            <Text className="page-eyebrow">PERSONALITY</Text>
            <Text className="page-title">个性说明书</Text>
            <Text className="page-subtitle">性格标签，加上一份读懂它的说明书</Text>
          </View>
          {part === 1 && (
            <View className="tags">
              {personality.length > 0
                ? personality.map(item => (
<Text className="tag" key={item.id}>
{item.trait}
{item.value ? ` · ${item.value}` : ''}
</Text>
))
                : <Text className="empty-text">还没有性格标签</Text>}
            </View>
          )}
          <View className="manual-list">
            {partQuestions.length === 0 && (
              <View className="manual-item" style={{ opacity: 0.6, pointerEvents: 'none' }}>
                <Text className="manual-index">—</Text>
                <View>
                  <Text className="h3">正在补充中</Text>
                  <Text className="p">相处一段时间后，这里会记录它的性格特点。</Text>
                </View>
              </View>
            )}
            {partQuestions.map((q, i) => (
              <Button
                key={q.id}
                className="manual-item detail-trigger"
                onClick={() => openDetail(q.question, q.answer || '暂无回答', q.answer || '可以点击编辑补充更多关于它的描述。')}
              >
                <Text className="manual-index">{String(start + i + 1).padStart(2, '0')}</Text>
                <View className="manual-item-body">
                  <Text className="h3">{q.question}</Text>
                  {q.answer && <Text className="p">{q.answer}</Text>}
                </View>
                <Text className="i">›</Text>
              </Button>
            ))}
          </View>
        </View>
      </View>
    )
  }

  /* ===== 章节: 健康资料 ===== */
  const renderHealthPage = () => {
    return (
      <View className="page content-page">
        <View className="page-body">
          <View className="page-head">
            <Text className="page-eyebrow">HEALTH</Text>
            <Text className="page-title">健康资料</Text>
            <Text className="page-subtitle">健康档案仅对家庭成员可见</Text>
          </View>
          <View className="health-lead">
            <View className="health-lead-main">
              <Text className="span">整体状态</Text>
              <Text className="strong">{profile ? (profile.sterilized ? '已绝育 · 状态稳定' : '未绝育 · 状态稳定') : '暂无资料'}</Text>
            </View>
            <Text className="span health-lead-tip">仅家庭可见</Text>
          </View>
          <View className="health-grid">
            <Button
              className="health-item detail-trigger"
              onClick={() => openDetail('过敏信息', profile?.breed ? `${profile.breed} 品种` : '暂无记录', '过敏信息由家庭成员补充。常见包括食物、环境与药物，记录后会显示在这里。')}
            >
              <Text className="health-label">过敏信息</Text>
              <Text className="health-value">{profile ? '待补充' : '—'}</Text>
              <Text className="health-note">点击查看详情</Text>
            </Button>
            <Button
              className="health-item detail-trigger"
              onClick={() => openDetail('既往疾病', '暂无记录', '在这里汇总既往病史、检查报告与治疗过程，方便家庭医生快速了解情况。')}
            >
              <Text className="health-label">既往疾病</Text>
              <Text className="health-value">暂无</Text>
              <Text className="health-note">点击查看详情</Text>
            </Button>
            <Button
              className="health-item detail-trigger"
              onClick={() => openDetail('长期用药', '目前无用药', '本模块只保存档案，不提供药物剂量建议；具体用药请遵医嘱。')}
            >
              <Text className="health-label">长期用药</Text>
              <Text className="health-value">无</Text>
              <Text className="health-note">点击查看详情</Text>
            </Button>
            <Button
              className="health-item detail-trigger"
              onClick={() => openDetail('最近疫苗', '待补充', '记录最近一次疫苗的种类、接种时间与医院，凭证仅家庭成员可见。')}
            >
              <Text className="health-label">最近疫苗</Text>
              <Text className="health-value">—</Text>
              <Text className="health-note">点击查看详情</Text>
            </Button>
          </View>
          <Text className="updated">档案由家庭成员维护</Text>
        </View>
      </View>
    )
  }

  /* ===== 章节: 生日纪念册（每页 3 条记录，超出自动开新页） ===== */
  const renderBirthdayPage = (part: number) => {
    const start = (part - 1) * BIRTHDAY_PER_PAGE
    const partRecords = birthdayRecords.slice(start, start + BIRTHDAY_PER_PAGE)
    return (
      <View className="page content-page">
        <View className="page-body">
          <View className="page-head">
            <Text className="page-eyebrow">BIRTHDAYS</Text>
            <Text className="page-title">生日纪念册</Text>
            <Text className="page-subtitle">一起数过的每一岁，都值得好好收藏。</Text>
          </View>
          {birthdayRecords.length === 0 && part === 1 && (
            <View className="birthday-feature" style={{ opacity: 0.6, textAlign: 'center' }}>
              <View className="birthday-copy">
                <Text className="span">还没有生日记录</Text>
                <Text className="h3">期待第一个生日</Text>
                <Text className="p">每年的生日都会记录在这里。</Text>
              </View>
            </View>
          )}
          {partRecords.map((r, i) => {
            const open = () => openDetail(`${r.age} 生日`, r.summary || '暂无简介', `出生于 ${r.year} 年。这一年的故事是：${r.summary || '正在补充中'}。`)
            if (start + i === 0) {
              return (
                <View className="birthday-feature" key={r.id} onClick={open}>
                  <View className="media-placeholder">📷</View>
                  <View className="birthday-copy">
                    <Text className="span">
{r.year}
{' '}
·
{' '}
{r.age}
{' '}
岁
                    </Text>
                    <Text className="h3">{r.summary || '一起记录这一岁'}</Text>
                    <Text className="p">
“
{r.summary || '新的一岁，希望你健健康康。'}
”
                    </Text>
                  </View>
                </View>
              )
            }
            return (
              <View className="year-row" key={r.id} onClick={open}>
                <Text className="strong">{r.year}</Text>
                <Text className="span">
{r.age}
{' '}
岁 ·
{' '}
{r.summary || '暂无简介'}
                </Text>
              </View>
            )
          })}
        </View>
      </View>
    )
  }

  /* ===== 章节: 成长足迹（第 1 页：体重卡 + 2 条事件；之后每页 3 条） ===== */
  const renderGrowthPage = (part: number) => {
    const sortedWeights = [...weights].sort((a, b) => b.measured_at.localeCompare(a.measured_at))
    const wLatest = sortedWeights[0]
    const wPrev = sortedWeights[1]
    const wDiff = wLatest && wPrev ? wLatest.weight - wPrev.weight : 0

    const sortedEvents = [...growthEvents].sort((a, b) => b.occurred_at.localeCompare(a.occurred_at))
    const partEvents = part === 1
      ? sortedEvents.slice(0, GROWTH_FIRST_PAGE_EVENTS)
      : sortedEvents.slice(
          GROWTH_FIRST_PAGE_EVENTS + (part - 2) * GROWTH_PER_PAGE,
          GROWTH_FIRST_PAGE_EVENTS + (part - 1) * GROWTH_PER_PAGE,
        )

    return (
      <View className="page content-page">
        <View className="page-body">
          <View className="page-head">
            <Text className="page-eyebrow">GROWTH</Text>
            <Text className="page-title">成长足迹</Text>
            <Text className="page-subtitle">把日子里的小事，慢慢连成它的一生。</Text>
          </View>
          {part === 1 && (
            <>
              <Text className="growth-section-title">体重记录</Text>
              <View
                className="weight-card detail-trigger"
                onClick={() => wLatest
                  ? openDetail('体重记录', `当前 ${wLatest.weight} kg，比上次${wDiff < 0 ? '减少' : '增加'} ${Math.abs(wDiff).toFixed(1)} kg`, `最近记录：${sortedWeights.map(w => `${w.measured_at} ${w.weight} kg`).join('；')}。`)
                  : openDetail('体重记录', '暂无记录', '还没有体重记录，添加后这里会展示体重变化趋势。')}
              >
                {wLatest
? (
                  <>
                    <View className="weight-head">
                      <View className="weight-head-main">
                        <Text className="span">当前体重</Text>
                        <Text className="weight-value">
{wLatest.weight}
{' '}
kg
                        </Text>
                      </View>
                      <View className="weight-change">{wPrev ? `较上次 ${wDiff < 0 ? '−' : '+'}${Math.abs(wDiff).toFixed(1)}` : '首次记录'}</View>
                    </View>
                    <View className="chart">
                      <View className="chart-line" />
                    </View>
                  </>
                )
: (
                  <View className="weight-head">
                    <View className="weight-head-main">
                      <Text className="span">当前体重</Text>
                      <Text className="weight-value weight-value--empty">暂无记录</Text>
                    </View>
                  </View>
                )}
              </View>
              <View className="growth-events-head">
                <Text className="growth-section-title">事件记录</Text>
              </View>
            </>
          )}
          <View className="timeline">
            {partEvents.length === 0 && part === 1 && (
              <View className="moment" style={{ textAlign: 'center', opacity: 0.6 }}>
                <Text className="p">还没有事件记录，添加后这里会展示成长时间线。</Text>
              </View>
            )}
            {partEvents.map(ev => (
              <View
                className="moment"
                key={ev.id}
                onClick={() => openDetail(ev.type, ev.content, `${ev.occurred_at} · ${ev.recorder || '我'}。${ev.content}`)}
              >
                <Text className="time">
{ev.occurred_at}
{' '}
·
{' '}
{ev.recorder || '我'}
                </Text>
                <Text className="h3">{ev.type}</Text>
                <Text className="p">{ev.content}</Text>
              </View>
            ))}
          </View>
        </View>
      </View>
    )
  }

  /* ===== 章节: 封底 ===== */
  const renderBackCover = () => (
    <View className="page back-cover">
      <View className="back-cover-content">
        <View className="back-mark">M</View>
        <Text className="h2">
<Text className="pet-name">{profile?.name || selectedPet?.name || '宠'}</Text>
，下页见
        </Text>
        <Text className="p">新的故事会继续发生，而家会一直把它们好好收着。</Text>
      </View>
      <Text className="back-family">宠物小册 · 由家人共同维护</Text>
    </View>
  )

  const renderBookPage = (pageIndex: number) => {
    const info = pages[pageIndex]
    if (!info) {
      return renderCover()
    }
    switch (info.chapter) {
      case 'cover':
        return renderCover()
      case 'identity':
        return renderProfilePage()
      case 'personality':
        return renderPersonalityPage(info.part)
      case 'health':
        return renderHealthPage()
      case 'birthday':
        return renderBirthdayPage(info.part)
      case 'growth':
        return renderGrowthPage(info.part)
      default:
        return renderBackCover()
    }
  }

  /* 目录：按章节跳转（跳到该章节的第 1 页） */
  const chapterStartPage = (key: ChapterKey) => pages.findIndex(p => p.chapter === key)
  const currentPageInfo = pages[currentPage] || pages[0]

  return (
    <View className="archive-page">
      <Image className="archive-background" src={backgroundImage} mode="aspectFill" />

      {/* 右上角添加宠物按钮已移除：与切换弹层底部"点击添加宠物"入口重复 */}

      {/* 左上角宠物切换：仅宠物名 + 三角标 */}
      <Button
        className="pet-switcher-button"
        onClick={() => setSwitcherOpen(true)}
        disabled={pets.length === 0}
      >
        <Text className="pet-switcher-name">{selectedPet?.name || '暂未选择'}</Text>
        <Text className="pet-switcher-caret">▾</Text>
      </Button>

      <View className="book-container" style={{ aspectRatio: BOOK_RATIO }} onTouchStart={handleTouchStart} onTouchMove={handleTouchMove} onTouchEnd={handleTouchEnd}>
        {adjacentPage !== null && <View className="book-card book-card-next">{renderBookPage(adjacentPage)}</View>}
        <View className={`book-card book-card-current${touchStartX === null && !resetting ? '' : ' dragging'}`} style={{ transform: `rotateY(${Math.max(-180, Math.min(180, touchDeltaX * 0.5))}deg)` }}>
          {renderBookPage(currentPage)}
        </View>
        {loading && <View className="book-loading"><Text>正在加载档案</Text></View>}
        {!loading && !selectedPet && (
          <View className="book-loading">
            <Text>还没有宠物档案</Text>
          </View>
        )}
      </View>

      {/* 底部页码状态：点击打开目录 */}
      <Button className="page-status" onClick={() => setTocOpen(true)}>
        <Text className="span">{currentPageInfo?.name || '封面'}</Text>
        <Text className="page-dot" />
        <Text className="span">
{currentPage + 1}
{' '}
/
{' '}
{pageCount}
        </Text>
      </Button>

      {/* 目录弹层 */}
      <View className={`overlay${tocOpen ? ' open' : ''}`} onClick={() => setTocOpen(false)}>
        <View className="sheet toc-sheet" onClick={event => event.stopPropagation()}>
          <View className="handle" />
          <View className="sheet-head">
            <View className="sheet-head-main">
              <Text className="h2">目录</Text>
              <Text className="p">
{profile?.name || selectedPet?.name || '宠物'}
{' '}
· 共
{' '}
{pageCount}
{' '}
页
              </Text>
            </View>
            <Button className="close-button" onClick={() => setTocOpen(false)}>×</Button>
          </View>
          <View className="chapter-list">
            {CHAPTERS.map((ch) => {
              const startPage = chapterStartPage(ch.key)
              const parts = pages.filter(p => p.chapter === ch.key).length
              const active = currentPageInfo?.chapter === ch.key
              return (
                <Button
                  className={`chapter-item${active ? ' active' : ''}`}
                  key={ch.key}
                  onClick={() => {
                    goToPage(startPage)
                    setTocOpen(false)
                  }}
                >
                  <Text className="chapter-number">{String(startPage + 1).padStart(2, '0')}</Text>
                  <Text className="strong">
{ch.name}
{parts > 1 ? `（${parts} 页）` : ''}
                  </Text>
                  <Text className="chapter-page-num">
{startPage + 1}
{' '}
/
{' '}
{pageCount}
                  </Text>
                </Button>
              )
            })}
          </View>
        </View>
      </View>

      {/* 详情弹层 */}
      <View className={`overlay${detail ? ' open' : ''}`} onClick={() => setDetail(null)}>
        <View className="detail-card" onClick={event => event.stopPropagation()}>
          <View className="detail-card-head">
            <View className="detail-card-head-main">
              <Text className="h2">{detail?.title || '详情'}</Text>
              {!!detail?.subtitle && <Text className="p">{detail.subtitle}</Text>}
            </View>
            <Button className="close-button" onClick={() => setDetail(null)}>×</Button>
          </View>
          <View className="detail-block">
            <Text className="span">记录内容</Text>
            <Text className="p">{detail?.body || ''}</Text>
          </View>
        </View>
      </View>

      {/* 宠物切换弹层（顶部下拉） */}
      <View className={`overlay overlay--top${switcherOpen ? ' open' : ''}`} onClick={() => setSwitcherOpen(false)}>
        <View className="pet-switcher-panel" onClick={event => event.stopPropagation()}>
          <View className="handle" />
          <View className="sheet-head">
            <View className="sheet-head-main">
              <Text className="h2">切换宠物</Text>
              <Text className="p">
共
{pets.length}
{' '}
只 · 选择后将翻开新档案
              </Text>
            </View>
            <Button className="close-button" onClick={() => setSwitcherOpen(false)}>×</Button>
          </View>
          <ScrollView className="pet-switcher-list" scrollY>
            {pets.length === 0 && (
              <View className="pet-switcher-empty">
                <Text className="p">家里还没有宠物档案，先去首页创建一只吧。</Text>
              </View>
            )}
            {pets.map(pet => (
              <Button
                key={pet.id}
                className={`pet-switcher-item${pet.id === currentPetId ? ' active' : ''}`}
                onClick={() => handleSwitchPet(pet.id)}
              >
                <View className="pet-switcher-item-avatar">
                  {pet.name.slice(0, 1)}
                </View>
                <View className="pet-switcher-item-meta">
                  <Text className="strong">{pet.name}</Text>
                  <Text className="span">{pet.id === currentPetId ? '当前展示中' : '点击翻开此档案'}</Text>
                </View>
                {pet.id === currentPetId && <Text className="pet-switcher-check">✓</Text>}
              </Button>
            ))}
            {/* 最底部一行: 点击添加宠物 (与档案右上角添加宠物按钮同路径) */}
            <Button
              className="pet-switcher-add"
              onClick={() => {
                setSwitcherOpen(false)
                navigateTo(routes.pages.petEdit)
              }}
            >
              <Text className="pet-switcher-add-plus">＋</Text>
              <Text className="pet-switcher-add-text">点击添加宠物</Text>
            </Button>
          </ScrollView>
        </View>
      </View>

      {/* 成长足迹添加按钮(只在成长足迹章节显示) */}
      {currentPageInfo?.chapter === 'growth' && (
        <Button className="growth-add-button" onClick={openGrowthForm}>＋ 添加事件</Button>
      )}

      {/* 成长足迹添加事件弹层 */}
      {growthFormVisible && (
        <View className="growth-overlay" onClick={closeGrowthForm}>
          <View className="growth-sheet" onClick={event => event.stopPropagation()}>
            <View className="growth-sheet-handle" />
            <Text className="growth-sheet-title">添加事件</Text>

            <Text className="growth-field-label">分类</Text>
            <View className="growth-segments">
              <View className={`growth-segment${growthFormCategory === 'daily' ? ' selected' : ''}`} onClick={() => setGrowthFormCategory('daily')}>日常</View>
              <View className={`growth-segment${growthFormCategory === 'medical' ? ' selected medical' : ''}`} onClick={() => setGrowthFormCategory('medical')}>医疗</View>
            </View>

            <Text className="growth-field-label">宠物</Text>
            <View className="growth-pet-chips">
              {pets.map(pet => (
                <View key={pet.id} className={`growth-pet-chip${growthFormPetID === pet.id ? ' selected' : ''}`} onClick={() => setGrowthFormPetID(pet.id)}>{pet.name}</View>
              ))}
            </View>

            <Text className="growth-field-label">发生日期</Text>
            <Picker mode="date" value={growthFormDate} onChange={event => setGrowthFormDate(event.detail.value)}>
              <View className="growth-picker-row">
                <Text>{growthFormDate || '选择日期'}</Text>
                <Text>选择</Text>
              </View>
            </Picker>

            {growthFormCategory === 'medical' && (
              <>
                <Text className="growth-field-label">医疗类型</Text>
                <View className="growth-type-chips">
                  {(['vaccine', 'deworming', 'checkup', 'visit', 'medication', 'other'] as MedicalType[]).map(type => (
                    <View key={type} className={`growth-type-chip${growthFormMedicalType === type ? ' selected' : ''}`} onClick={() => setGrowthFormMedicalType(type)}>{MEDICAL_TYPE_LABEL[type]}</View>
                  ))}
                </View>
                {growthFormMedicalType === 'other' && (
                  <Input className="growth-custom-medical-type" value={growthFormCustomMedicalType} maxlength={50} placeholder="请输入医疗类型" onInput={event => setGrowthFormCustomMedicalType(event.detail.value)} />
                )}
              </>
            )}

            <Text className="growth-field-label">记录内容</Text>
            <Textarea className="growth-textarea" value={growthFormContent} maxlength={1000} placeholder={growthFormCategory === 'medical' ? '医院、药品等需要备注的写在这里哦～' : '写下今天发生的事'} onInput={event => setGrowthFormContent(event.detail.value)} />

            <View className="growth-sheet-actions">
              <View className="growth-cancel-button" onClick={closeGrowthForm}>取消</View>
              <View className={`growth-save-button${growthSubmitting ? ' disabled' : ''}`} onClick={handleCreateGrowthEvent}>{growthSubmitting ? '保存中' : '保存'}</View>
            </View>
          </View>
        </View>
      )}
    </View>
  )
}
