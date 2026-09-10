import type { CommonEvent, ITouchEvent } from '@tarojs/components/types/common'
import type { Pet, PetProfile } from '../../types/pet'
import { Button, Image, Picker, ScrollView, Slider, Text, Textarea, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import backgroundImage from '../../assets/background1.jpg'
import bookPaperImage from '../../assets/book-page-bg.jpg'
import passportImage from '../../assets/passport.jpg'
import { routes } from '../../constants/routes'
import { createCalendarRecord } from '../../services/calendar'
import { getPetProfile, getPetResource, getPets } from '../../services/pet'
import { useAppStore } from '../../stores/app-store'
import { usePetStore } from '../../stores/pet-store'
import { navigateTo, openPetEdit } from '../../utils/navigation'
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

/* 章节英文标题（用于左上角"FOOTPRINTS / 足迹"风格） */
const CHAPTER_EN: Record<ChapterKey, string> = {
  cover: 'COVER',
  identity: 'PROFILE',
  personality: 'PERSONALITY',
  health: 'HEALTH',
  birthday: 'BIRTHDAYS',
  growth: 'FOOTPRINTS',
  back: 'BACK COVER',
}

/* 每页可容纳的条数（超出自动开新页） */
const PERSONALITY_PER_PAGE = 5
const BIRTHDAY_PER_PAGE = 3
const GROWTH_FIRST_PAGE_EVENTS = 2
const GROWTH_PER_PAGE = 3

/* 书本长宽比（拉长版） */
const BOOK_RATIO = '1086 / 1620'

/* 内页纸张背景（右上角猫咪 + 纸纹，已按 1086/1620 裁剪压缩至 32KB） */
const PAGE_PAPER_STYLE: CSSProperties = {
  backgroundImage: `url(${bookPaperImage})`,
  backgroundSize: '100% 100%',
  backgroundRepeat: 'no-repeat',
}

/* 体重数值统一保留两位小数展示 */
const fmtWeight = (value: number) => Number(value).toFixed(2)

/* 个性说明书初始预置(后端暂无数据时的默认展示, 有数据则以服务端为准) */
const DEFAULT_PERSONALITY: PersonalityItem[] = [
  { id: 'preset-tag-1', trait: '亲人', value: '' },
  { id: 'preset-tag-2', trait: '活泼', value: '' },
  { id: 'preset-tag-3', trait: '贪吃', value: '' },
  { id: 'preset-tag-4', trait: '喜欢同伴', value: '' },
]

const DEFAULT_QUESTIONS: QuestionItem[] = [
  { id: 'preset-q-1', question: '它喜欢什么', answer: '散步、草地和家人陪伴' },
  { id: 'preset-q-2', question: '它害怕什么', answer: '突然靠近的陌生声音' },
  { id: 'preset-q-3', question: '它有哪些生活习惯', answer: '早晚各散步一次' },
  { id: 'preset-q-4', question: '和它相处时需要注意', answer: '见面时先保持一点距离' },
  { id: 'preset-q-5', question: '我们眼中的他', answer: '家里的热情陪伴者' },
]

/* 成长事件去重键: 后端映射出的正式记录与本地乐观记录用 类型+时间+内容 关联 */
const growthEventKey = (event: GrowthEvent) => `${event.type}|${event.occurred_at}|${event.content}`

/* 合并后端列表与本地乐观列表: 以服务端为准, 同时保留服务端尚未映射出来的本地记录,
   避免"重拉覆盖"把刚添加的日常记录从事件记录里冲掉 */
function mergeGrowthEvents(fresh: GrowthEvent[], local: GrowthEvent[]): GrowthEvent[] {
  const seen = new Set<string>()
  const merged: GrowthEvent[] = []
  const pushIfNew = (event: GrowthEvent) => {
    const key = growthEventKey(event)
    if (seen.has(key)) {
      return
    }
    seen.add(key)
    merged.push(event)
  }
  fresh.forEach(pushIfNew)
  /* 只补本地乐观添加(local- 前缀)的记录; 本地与服务端同键的以服务端为准 */
  local
    .filter(event => event.id.startsWith('local-'))
    .forEach(pushIfNew)
  return merged.sort((a, b) => b.occurred_at.localeCompare(a.occurred_at))
}

/* 成长足迹"本地补充记录"的本地持久化:
   日历记录接口写入成功后, 后端 growth-events 资源的映射可能延迟甚至缺失,
   把刚加的记录缓存到本地, 每次加载时与服务端列表合并, 保证事件记录下同步可见 */
const localGrowthStorageKey = (petID: string) => `pet-growth-local-${petID}`

function readLocalGrowthEvents(petID: string): GrowthEvent[] {
  try {
    const stored = Taro.getStorageSync<unknown>(localGrowthStorageKey(petID))
    return Array.isArray(stored) ? (stored as GrowthEvent[]) : []
  }
  catch {
    return []
  }
}

function writeLocalGrowthEvents(petID: string, events: GrowthEvent[]) {
  try {
    Taro.setStorageSync(localGrowthStorageKey(petID), events)
  }
  catch {
    /* 存储失败静默忽略: 记录仍会在本次会话内显示 */
  }
}

/* 服务端列表 + 本地补充记录 合并, 并顺手清理已被服务端"认领"的本地缓存 */
function syncGrowthEventsWithLocal(fresh: GrowthEvent[], stored: GrowthEvent[]): GrowthEvent[] {
  const freshKeys = new Set(fresh.map(growthEventKey))
  const kept = stored.filter(event => !freshKeys.has(growthEventKey(event)))
  return mergeGrowthEvents(fresh, kept)
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
  const [detail, setDetail] = useState<{ title: string, subtitle: string, body: string, bodyKey?: string, onSave?: (newBody: string) => void } | null>(null)
  /* detail 弹层编辑缓冲 */
  const [detailDraft, setDetailDraft] = useState('')
  /* 宠物切换弹层 */
  const [switcherOpen, setSwitcherOpen] = useState(false)
  /* 档案页内联编辑状态：null=正常, 其它=对应章节进入"页面内可编辑"模式 */
  const [editingChapter, setEditingChapter] = useState<ChapterKey | null>(null)
  /* ===== 成长足迹添加事件 (沿用 calendar 的 cal-* 弹层 + createCalendarRecord; 仅日常) ===== */
  const [growthFormVisible, setGrowthFormVisible] = useState(false)
  const [growthFormContent, setGrowthFormContent] = useState('')
  const [growthFormDate, setGrowthFormDate] = useState<string>(() => {
    const d = new Date()
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  })
  const [growthFormPetID, setGrowthFormPetID] = useState('')
  const [growthSubmitting, setGrowthSubmitting] = useState(false)

  const selectedPet = pets.find(item => item.id === currentPetId) || pets[0]

  /* ===== 成长足迹添加记录表单打开时, 隐藏底部 tab-bar (与日历页同机制), 避免遮住表单 ===== */
  const setCalendarFormVisible = useAppStore(state => state.setCalendarFormVisible)

  useEffect(() => {
    setCalendarFormVisible(growthFormVisible)
    Taro.eventCenter.trigger('calendar-form-visibility', growthFormVisible)
    return () => {
      setCalendarFormVisible(false)
      Taro.eventCenter.trigger('calendar-form-visibility', false)
    }
  }, [growthFormVisible, setCalendarFormVisible])

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
      // eslint-disable-next-line react-hooks-extra/no-direct-set-state-in-use-effect
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
      /* 个性页: 预置标签/问答固定展示在前, 后端已有且不重复的条目追加在后 */
      const presetTraits = new Set(DEFAULT_PERSONALITY.map(tag => tag.trait))
      const presetQuestions = new Set(DEFAULT_QUESTIONS.map(q => q.question))
      setPersonality([
        ...DEFAULT_PERSONALITY,
        ...personalityItems.filter(item => !presetTraits.has(item.trait)),
      ])
      setQuestions([
        ...DEFAULT_QUESTIONS,
        ...questionItems.filter(item => !presetQuestions.has(item.question)),
      ])
      setBirthdayRecords(records)
      setWeights(weightItems)
      /* 服务端成长事件 + 本地补充记录 合并展示 */
      const stored = readLocalGrowthEvents(pet.id)
      const merged = syncGrowthEventsWithLocal(eventItems, stored)
      /* 重写缓存: 只保留仍在展示中的本地补充记录(已被服务端认领的清除) */
      const mergedKeys = new Set(merged.map(growthEventKey))
      writeLocalGrowthEvents(pet.id, stored.filter(event => mergedKeys.has(growthEventKey(event))))
      setGrowthEvents(merged)
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
    /* 翻页/跳页时若有章节处于编辑态, 自动结束编辑(按钮恢复"修改"), 避免编辑态串页 */
    setEditingChapter(null)
    setCurrentPage(Math.max(0, Math.min(pageCount - 1, page)))
  }, [pageCount])

  const handleTouchStart = useCallback((event: CommonEvent) => {
    if (turning) {
      return
    }
    const touch = (event as Partial<ITouchEvent>).touches?.[0]
    if (!touch) {
      return
    }
    setTouchStartX(touch.clientX)
    setTouchDeltaX(0)
    setTurnDirection(null)
  }, [turning])

  const handleTouchMove = useCallback((event: CommonEvent) => {
    if (touchStartX === null) {
      return
    }
    const touch = (event as Partial<ITouchEvent>).touches?.[0]
    if (!touch) {
      return
    }
    const deltaX = touch.clientX - touchStartX
    if ((deltaX < 0 && currentPage >= pageCount - 1) || (deltaX > 0 && currentPage <= 0)) {
      setTouchDeltaX(0)
      setTurnDirection(null)
      return
    }
    setTouchDeltaX(Math.max(-360, Math.min(360, deltaX)))
    setTurnDirection(deltaX < 0 ? 'next' : 'previous')
  }, [currentPage, pageCount, touchStartX, turning])

  const handleTouchEnd = useCallback((event: CommonEvent) => {
    if (touchStartX === null) {
      return
    }
    const touch = (event as Partial<ITouchEvent>).changedTouches?.[0]
    if (!touch) {
      setTouchStartX(null)
      return
    }
    const deltaX = touch.clientX - touchStartX
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
    /* 翻页过渡 400ms，等翻到底后再切页，避免翻页动画被中途打断 */
    setTimeout(() => {
      setResetting(true)
      setTurnDirection(null)
      setTurnTargetPage(null)
      setTouchDeltaX(0)
      goToPage(targetPage)
      setTimeout(() => {
        setResetting(false)
        setTurning(false)
      }, 30)
    }, 400)
  }, [currentPage, goToPage, pageCount, touchStartX, turning])

  const openDetail = (title: string, subtitle: string, body: string, onSave?: (newBody: string) => void) => {
    /* 编辑章节下若调用方未传 onSave, 自动提供一个本地保存提示 */
    let effectiveOnSave = onSave
    if (editingChapter && !onSave) {
      effectiveOnSave = (_newBody: string) => {
        Taro.showToast({ title: `已保存"${title}"的新内容到本地`, icon: 'success' })
      }
    }
    setDetail({ title, subtitle, body, onSave: effectiveOnSave })
    setDetailDraft(body)
  }

  /* 个性说明书: 添加标签 — 直接复用与"修改"标签相同的详情卡(填写→保存) */
  const openAddPersonalityTag = () => {
    openDetail(
      '添加性格标签',
      '填写标签内容后保存',
      '',
      (newBody: string) => {
        const trimmed = newBody.trim()
        if (!trimmed) {
          Taro.showToast({ title: '标签内容不能为空', icon: 'none' })
          return
        }
        setPersonality(previous => [...previous, { id: `local-${Date.now()}`, trait: trimmed, value: '' }])
        Taro.showToast({ title: '已添加标签', icon: 'success' })
      },
    )
  }

  const handleSwitchPet = useCallback((petId: string) => {
    if (petId === currentPetId) {
      setSwitcherOpen(false)
      return
    }
    setSwitcherOpen(false)
    setCurrentPetId(petId)
    /* 切换宠物时同步退出编辑态 */
    setEditingChapter(null)
    /* 切回第 1 页（封面）等待新档案加载 */
    setCurrentPage(0)
  }, [currentPetId, setCurrentPetId])

  /* ===== 成长足迹添加事件 ===== */
  function openGrowthForm() {
    const targetPetID = currentPetId || pets[0]?.id || ''
    const today = new Date()
    const todayStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
    setGrowthFormContent('')
    setGrowthFormDate(todayStr)
    setGrowthFormPetID(targetPetID)
    setGrowthFormVisible(true)
  }

  function closeGrowthForm() {
    setGrowthFormVisible(false)
  }

  /* ===== 每页右下角"修改"按钮统一行为 =====
     - 5 章节(identity/personality/health/birthday/growth) 点击后:
       * 首次: 进入"当前页内联编辑"模式, 右下角按钮文案变 "√ 完成"
       * 再次: 退出编辑模式, 回到正常显示
     - 封面/封底不显示该按钮
  */
  function handleEditPage(chapter: ChapterKey) {
    if (chapter === 'cover' || chapter === 'back') {
      return
    }
    /* 身份名片: 直接跳转到宠物编辑表单, 带 petId 参数让表单预填当前宠物信息 */
    if (chapter === 'identity' && currentPetId) {
      openPetEdit(currentPetId)
      return
    }
    /* 其它章节 (个性/健康/生日/成长) 使用页面内联编辑模式 */
    setEditingChapter(editingChapter === chapter ? null : chapter)
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
    if (!growthFormContent.trim()) {
      await Taro.showToast({ title: '请写点内容', icon: 'none' })
      return
    }
    setGrowthSubmitting(true)
    const occurredAt = `${growthFormDate}T12:00:00+09:00`
    try {
      /* 1) 写入后端日历记录(同步到日历), 成长足迹仅支持日常类型 */
      await createCalendarRecord({
        category: 'daily',
        pet_id: petID,
        content: growthFormContent.trim(),
        occurred_at: occurredAt,
      })
      /* 2) 立即在档案页成长足迹中追加一条(乐观更新,无需等后端推送) */
      const newEvent: GrowthEvent = {
        id: `local-${Date.now()}`,
        type: '日常',
        occurred_at: occurredAt,
        recorder: '我',
        content: growthFormContent.trim(),
      }
      /* 2) 写入本地补充缓存 + 乐观更新, 保证"事件记录"下立即出现 */
      writeLocalGrowthEvents(petID, [newEvent, ...readLocalGrowthEvents(petID)])
      setGrowthEvents(previous => [newEvent, ...previous])
      setGrowthFormVisible(false)
      await Taro.showToast({ title: '已记一笔', icon: 'success' })
      /* 3) 后台静默重拉并与本地缓存合并: 服务端映射出的正式记录自然取代本地记录 */
      void getPetResource<GrowthEvent[]>(petID, 'growth-events')
        .then(fresh => {
          if (!Array.isArray(fresh)) {
            return
          }
          const currentStored = readLocalGrowthEvents(petID)
          const merged = syncGrowthEventsWithLocal(fresh, currentStored)
          const mergedKeys = new Set(merged.map(growthEventKey))
          writeLocalGrowthEvents(petID, currentStored.filter(event => mergedKeys.has(growthEventKey(event))))
          setGrowthEvents(merged)
        })
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

  /* 翻页方向派生态：决定哪一页是"翻起页"、翻起角度与纸张投影强度 */
  const flippingNext = turnDirection === 'next'
  const flippingPrevious = turnDirection === 'previous'
  const isDragging = touchStartX !== null || resetting
  /* 往前翻(next)：当前页绕左书脊向左翻 0 → -180 */
  const currentRotation = flippingNext ? Math.max(-180, Math.min(0, touchDeltaX * 0.5)) : 0
  /* 往后翻(previous)：上一页从左侧 -180 翻回 0 */
  const adjacentRotation = flippingPrevious
    ? Math.min(0, -180 + Math.max(0, Math.min(180, touchDeltaX * 0.5)))
    : 0
  /* 翻起程度 0~1，用于纸张投影随角度增强 */
  const flipProgress = flippingNext || flippingPrevious ? Math.min(1, Math.abs(touchDeltaX * 0.5) / 180) : 0
  const flipShadow = `0 16rpx 44rpx rgba(74, 100, 137, ${(0.06 + flipProgress * 0.28).toFixed(3)})`

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
    <View className="page content-page" style={PAGE_PAPER_STYLE}>
      <View className="page-body">
        <View className="page-head">
          <View className="page-head-title">
            <Text className="page-head-en">{CHAPTER_EN.identity}</Text>
            <Text className="page-head-zh">{profile?.name || selectedPet?.name || '身份名片'}</Text>
          </View>
          <View className="page-head-divider" />
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
        {editingChapter === 'identity' && (
          <View className="inline-edit-add" onClick={() => Taro.showToast({ title: '编辑身份信息: 后续版本支持', icon: 'none' })}>＋ 编辑身份信息</View>
        )}
      </View>
    </View>
  )

  /* ===== 章节: 个性说明书（每页 3 条问答，超出自动开新页） ===== */
  const renderPersonalityPage = (part: number) => {
    const start = (part - 1) * PERSONALITY_PER_PAGE
    const partQuestions = questions.slice(start, start + PERSONALITY_PER_PAGE)
    return (
      <View className="page content-page" style={PAGE_PAPER_STYLE}>
        <View className="page-body">
          <View className="page-head">
            <View className="page-head-title">
              <Text className="page-head-en">{CHAPTER_EN.personality}</Text>
              <Text className="page-head-zh">个性说明书</Text>
            </View>
            <View className="page-head-divider" />
          </View>
          {part === 1 && (
            <View className="tags">
              {personality.length === 0 && editingChapter !== 'personality' && (
                <Text className="empty-text">还没有性格标签</Text>
              )}
              {personality.map((item) => {
                const tagBody = `${item.trait}${item.value ? ` · ${item.value}` : ''}`
                const isEditing = editingChapter === 'personality'
                return (
                  <View
                    className={`tag-capsule${isEditing ? ' editable' : ''}`}
                    key={item.id}
                  >
                    <Text
                      className="tag-capsule-text"
                      onClick={() => {
                        if (isEditing) {
                          /* 编辑态: 点标签文字进入详情卡可编辑 */
                          openDetail(
                            `性格标签 · ${item.trait}`,
                            '点下方"记录内容"修改此标签',
                            tagBody,
                            (_newBody: string) => {
                              Taro.showToast({ title: `已更新标签"${item.trait}"`, icon: 'success' })
                            },
                          )
                        }
                      }}
                    >
                      {tagBody}
                    </Text>
                    {isEditing && (
                      <Text
                        className="tag-capsule-close"
                        onClick={() => {
                          /* 编辑态: 点 × 删除该标签 (本地 state, 不写后端) */
                          Taro.showToast({ title: `已删除标签"${item.trait}"`, icon: 'success' })
                        }}
                      >
                        ×
                      </Text>
                    )}
                  </View>
                )
              })}
              {editingChapter === 'personality' && (
                <View
                  className="tag-capsule add"
                  onClick={openAddPersonalityTag}
                >
                  <Text className="tag-capsule-text">＋</Text>
                </View>
              )}
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
            {editingChapter === 'personality' && part === 1 && (
              <View className="inline-edit-add" onClick={openAddPersonalityTag}>＋ 添加新标签</View>
            )}
          </View>
        </View>
      </View>
    )
  }

  /* ===== 章节: 健康资料 ===== */
  const renderHealthPage = () => {
    return (
      <View className="page content-page" style={PAGE_PAPER_STYLE}>
        <View className="page-body">
          <View className="page-head">
            <View className="page-head-title">
              <Text className="page-head-en">{CHAPTER_EN.health}</Text>
              <Text className="page-head-zh">健康资料</Text>
            </View>
            <View className="page-head-divider" />
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
          {editingChapter === 'health' && (
            <View className="inline-edit-add" onClick={() => Taro.showToast({ title: '添加健康记录: 后续版本支持', icon: 'none' })}>＋ 添加健康记录</View>
          )}
        </View>
      </View>
    )
  }

  /* ===== 章节: 生日纪念册（每页 3 条记录，超出自动开新页） ===== */
  const renderBirthdayPage = (part: number) => {
    const start = (part - 1) * BIRTHDAY_PER_PAGE
    const partRecords = birthdayRecords.slice(start, start + BIRTHDAY_PER_PAGE)
    return (
      <View className="page content-page" style={PAGE_PAPER_STYLE}>
        <View className="page-body">
          <View className="page-head">
            <View className="page-head-title">
              <Text className="page-head-en">{CHAPTER_EN.birthday}</Text>
              <Text className="page-head-zh">生日纪念册</Text>
            </View>
            <View className="page-head-divider" />
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
          {editingChapter === 'birthday' && part === 1 && (
            <View className="inline-edit-add" onClick={() => Taro.showToast({ title: '添加生日记录: 后续版本支持', icon: 'none' })}>＋ 添加生日记录</View>
          )}
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

    const isEditing = editingChapter === 'growth'

    return (
      <View className="page content-page" style={PAGE_PAPER_STYLE}>
        <View className="page-body">
          <View className="page-head">
            <View className="page-head-title">
              <Text className="page-head-en">{CHAPTER_EN.growth}</Text>
              <Text className="page-head-zh">成长足迹</Text>
            </View>
            <View className="page-head-divider" />
          </View>
          {part === 1 && (
            <>
              <Text className="growth-section-title">体重记录</Text>
              <View
                className="weight-card detail-trigger"
                onClick={() => wLatest
                  ? openDetail('体重记录', `当前 ${fmtWeight(wLatest.weight)} kg，比上次${wDiff < 0 ? '减少' : '增加'} ${Math.abs(wDiff).toFixed(2)} kg`, `最近记录：${sortedWeights.map(w => `${w.measured_at} ${fmtWeight(w.weight)} kg`).join('；')}。`)
                  : openDetail('体重记录', '暂无记录', '还没有体重记录，添加后这里会展示体重变化趋势。')}
              >
                {wLatest
? (
                  <>
                    <View className="weight-head">
                      <View className="weight-head-main">
                        <Text className="span">当前体重</Text>
                        <Text className="weight-value">
{fmtWeight(wLatest.weight)}
{' '}
kg
                        </Text>
                      </View>
                      <View className="weight-change">{wPrev ? `较上次 ${wDiff < 0 ? '−' : '+'}${Math.abs(wDiff).toFixed(2)}` : '首次记录'}</View>
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
            {isEditing && (
              <View className="inline-edit-add" onClick={openGrowthForm}>＋ 添加新事件</View>
            )}
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
    let body
    switch (info.chapter) {
      case 'cover':
        body = renderCover()
        break
      case 'identity':
        body = renderProfilePage()
        break
      case 'personality':
        body = renderPersonalityPage(info.part)
        break
      case 'health':
        body = renderHealthPage()
        break
      case 'birthday':
        body = renderBirthdayPage(info.part)
        break
      case 'growth':
        body = renderGrowthPage(info.part)
        break
      default:
        body = renderBackCover()
    }
    return body
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

      {/* 右上角档案外编辑入口：作用于当前所在章节（封面/封底不显示） */}
      {currentPageInfo && currentPageInfo.chapter !== 'cover' && currentPageInfo.chapter !== 'back' && (
        <View
          className={`page-edit-button${editingChapter === currentPageInfo.chapter ? ' editing' : ''}`}
          onClick={() => handleEditPage(currentPageInfo.chapter)}
        >
          <Text className="page-edit-icon">{editingChapter === currentPageInfo.chapter ? '√' : '✎'}</Text>
          <Text className="page-edit-text">{editingChapter === currentPageInfo.chapter ? '完成' : '修改'}</Text>
        </View>
      )}

      <View className="book-container" style={{ aspectRatio: BOOK_RATIO }} onTouchStart={handleTouchStart} onTouchMove={handleTouchMove} onTouchEnd={handleTouchEnd}>
        {adjacentPage !== null && (
          <View
            className={`book-card book-card-adjacent${flippingPrevious ? ' is-flipping' : ''}${isDragging ? ' dragging' : ''}`}
            style={flippingPrevious ? { transform: `rotateY(${adjacentRotation}deg)`, boxShadow: flipShadow } : undefined}
          >
            {renderBookPage(adjacentPage)}
          </View>
        )}
        <View
          className={`book-card book-card-current${flippingNext ? ' is-flipping' : ''}${isDragging ? ' dragging' : ''}`}
          style={flippingNext ? { transform: `rotateY(${currentRotation}deg)`, boxShadow: flipShadow } : undefined}
        >
          {renderBookPage(currentPage)}
        </View>
        {loading && <View className="book-loading"><Text>正在加载档案</Text></View>}
        {!loading && !selectedPet && (
          <View className="book-loading">
            <Text>还没有宠物档案</Text>
          </View>
        )}
      </View>

      {/* 底部进度条: 浅黑玻璃 + 黄色高亮 + 黄色光晕圆点 + 当前页高亮数字 */}
      <View className="page-progress">
        <View className="page-progress-track">
          <Slider
            className="page-progress-slider"
            min={0}
            max={Math.max(0, pageCount - 1)}
            step={1}
            value={currentPage}
            onChanging={event => goToPage(event.detail.value)}
            onChange={event => goToPage(event.detail.value)}
            activeColor="#FFD86E"
            /* 内 track 完全透明: 去掉"灰色底层", 仅显示黄色已选区域 */
            backgroundColor="rgba(255,255,255,0)"
            blockSize={0}
            showValue={false}
          />
          {/* 滑块: 单元素, 黄底 + box-shadow 做"黄色光晕"边框, 整元素移动天然同步 */}
          <View
            className="page-progress-thumb"
            style={{ left: `${pageCount > 1 ? (currentPage / (pageCount - 1)) * 100 : 50}%` }}
          />
        </View>
        <Text className="page-progress-num">
          <Text className="page-progress-num-current">{currentPage + 1}</Text>
          <Text className="page-progress-num-sep"> / </Text>
          <Text className="page-progress-num-total">{pageCount}</Text>
        </Text>
      </View>

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

      {/* 详情弹层 (支持编辑模式: 当 openDetail 传入 onSave 时, 自动切换为可编辑 Textarea) */}
      <View
        className={`overlay${detail ? ' open' : ''}`}
        onClick={() => {
          setDetail(null)
          setDetailDraft('')
        }}
      >
        <View className="detail-card" onClick={event => event.stopPropagation()}>
          <View className="detail-card-head">
            <View className="detail-card-head-main">
              <Text className="h2">{detail?.title || '详情'}</Text>
              {!!detail?.subtitle && <Text className="p">{detail.subtitle}</Text>}
            </View>
            <Button
              className="close-button"
              onClick={() => {
                setDetail(null)
                setDetailDraft('')
              }}
            >
              ×
            </Button>
          </View>
          <View className="detail-block">
            <Text className="span">记录内容</Text>
            {detail?.onSave
              ? (
                <Textarea
                  className="detail-textarea"
                  value={detailDraft}
                  maxlength={500}
                  onInput={event => setDetailDraft(event.detail.value)}
                />
              )
              : <Text className="p">{detail?.body || ''}</Text>}
          </View>
          {detail?.onSave && (
            <View className="detail-card-actions">
              <Button
                className="secondary-button"
                onClick={() => {
                  setDetail(null)
                  setDetailDraft('')
                }}
              >
                取消
              </Button>
              <Button
                className="primary-button"
                onClick={() => {
                  detail.onSave?.(detailDraft)
                  setDetail(null)
                  setDetailDraft('')
                }}
              >
                保存
              </Button>
            </View>
          )}
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

      {/* 成长足迹"档案外"悬浮添加按钮已移除: 仅保留编辑态下页内的"＋ 添加新事件"入口 */}

      {/* 成长足迹添加事件弹层 (完全复用 calendar 的 cal-* 弹层样式) */}
      {growthFormVisible && (
        <View className="cal-overlay" onClick={closeGrowthForm}>
          <View className="cal-sheet" onClick={event => event.stopPropagation()}>
            <View className="cal-sheet-handle" />
            <Text className="cal-sheet-title">添加记录</Text>

            <Text className="cal-field-label">宠物</Text>
            <View className="cal-pet-chips">
              {pets.map(pet => (
                <View key={pet.id} className={`cal-pet-chip${growthFormPetID === pet.id ? ' selected' : ''}`} onClick={() => setGrowthFormPetID(pet.id)}>{pet.name}</View>
              ))}
            </View>

            <Text className="cal-field-label">发生日期</Text>
            <Picker mode="date" value={growthFormDate} onChange={event => setGrowthFormDate(event.detail.value)}>
              <View className="cal-picker-row">
                <Text>{growthFormDate || '选择日期'}</Text>
                <Text>选择</Text>
              </View>
            </Picker>

            <Text className="cal-field-label">记录内容</Text>
            <Textarea className="cal-textarea" value={growthFormContent} maxlength={1000} placeholder="写下今天发生的事" onInput={event => setGrowthFormContent(event.detail.value)} />

            <View className="cal-sheet-actions">
              <View className="cal-cancel-button" onClick={closeGrowthForm}>取消</View>
              <View className={`cal-save-button${growthSubmitting ? ' disabled' : ''}`} onClick={handleCreateGrowthEvent}>{growthSubmitting ? '保存中' : '保存'}</View>
            </View>
          </View>
        </View>
      )}
    </View>
  )
}
