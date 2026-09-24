import type { CommonEvent, ITouchEvent } from '@tarojs/components/types/common'
import type { CSSProperties } from 'react'
import type { Pet, PetProfile } from '../../types/pet'
import { Button, Image, Input, Picker, ScrollView, Slider, Text, Textarea, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import backgroundImage from '../../assets/background1.jpg'
import bookPaperImage from '../../assets/book-page-bg.jpg'
import catPhoto from '../../assets/cat2.png'
import passportImage from '../../assets/passport.jpg'
import { FloatingGuide } from '../../components/floating-guide'
import { routes } from '../../constants/routes'
import { createCalendarRecord, updateCalendarRecord, uploadCalendarImage } from '../../services/calendar'
import { createPetResource, deletePet, deletePetResource, getPetProfile, getPetResource, getPets, updatePetResource } from '../../services/pet'
import { assetURL, uploadFile } from '../../services/request'
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

interface BirthdayMediaItem {
  id: string
  record_id: string
  type: string
  asset_id: string
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
  /* 本地补充记录对应的日历记录 ID: 表单添加时由 createCalendarRecord 返回, 用于后续同步编辑到日历 */
  calendar_record_id?: string
}

interface CertificateItem {
  id: string
  type: string
  name: string
  number: string
  occurred_at?: string
  details?: string
  asset_id?: string
}

/* 证件页预显示示例卡: 用户添加任意真实证件后不再显示 */
const PLACEHOLDER_CERTIFICATE = {
  name: '狂犬病免疫证明',
  time: '示例 · 待补充',
  details: '每年接种一针狂犬疫苗，接种后医院会出具免疫证明，注明疫苗批号与下次接种时间。点击下方"添加证件"录入真实证件后，这张示例卡会自动消失。',
}

/* 章节（书固定 7 个章节；内容多的章节自动拆成多页） */
const CHAPTERS = [
  { key: 'cover', name: '封面' },
  { key: 'identity', name: '身份名片' },
  { key: 'certificates', name: '证件收藏' },
  { key: 'health', name: '健康资料' },
  { key: 'personality', name: '个性说明书' },
  { key: 'birthday', name: '生日纪念册' },
  { key: 'growth', name: '成长足迹' },
  { key: 'back', name: '封底' },
] as const

type ChapterKey = typeof CHAPTERS[number]['key']

/* 章节英文标题（用于左上角"FOOTPRINTS / 足迹"风格） */
const CHAPTER_EN: Record<ChapterKey, string> = {
  cover: 'COVER',
  identity: 'PROFILE',
  certificates: 'CERTIFICATES',
  personality: 'PERSONALITY',
  health: 'HEALTH',
  birthday: 'BIRTHDAYS',
  growth: 'FOOTPRINTS',
  back: 'BACK COVER',
}

/* 每页可容纳的条数（超出自动开新页） */
const PERSONALITY_PER_PAGE = 5
const BIRTHDAY_PER_PAGE = 7
const CERTIFICATES_PER_PAGE = 3
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

/* 健康资料页体重卡片: 展示最近三条记录 */
const HEALTH_WEIGHT_PREVIEW = 3
/* 体重合法区间 (kg), 防止误输 */
const WEIGHT_MIN_KG = 0.1
const WEIGHT_MAX_KG = 150

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

/* 预置问答(preset-q-*)编辑后的本地覆盖缓存: 预置项不是服务端记录无法 PATCH,
   编辑结果存本地, 加载档案时覆盖默认内容, 保证修改不因重新拉取而回退 */
interface QuestionOverride { question: string, answer: string }

const questionOverridesStorageKey = (petID: string) => `pet-question-overrides-${petID}`

function readQuestionOverrides(petID: string): Record<string, QuestionOverride> {
  try {
    const stored = Taro.getStorageSync<unknown>(questionOverridesStorageKey(petID))
    if (stored && typeof stored === 'object' && !Array.isArray(stored)) {
      return stored as Record<string, QuestionOverride>
    }
    return {}
  }
  catch {
    return {}
  }
}

function writeQuestionOverrides(petID: string, overrides: Record<string, QuestionOverride>) {
  try {
    Taro.setStorageSync(questionOverridesStorageKey(petID), overrides)
  }
  catch {
    /* 存储失败静默忽略 */
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
  /* 只保留 YYYY-MM-DD, 兼容后端可能返回的 2023-04-12T00:00:00Z 形式 */
  return value ? value.slice(0, 10) : '暂无记录'
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
  const clearCurrentPet = usePetStore(state => state.clearCurrentPet)
  const [profile, setProfile] = useState<PetProfile | null>(null)
  const [personality, setPersonality] = useState<PersonalityItem[]>([])
  const [questions, setQuestions] = useState<QuestionItem[]>([])
  const [birthdayRecords, setBirthdayRecords] = useState<BirthdayRecord[]>([])
  const [birthdayMedia, setBirthdayMedia] = useState<BirthdayMediaItem[]>([])
  const [certificates, setCertificates] = useState<CertificateItem[]>([])
  const [weights, setWeights] = useState<WeightRecord[]>([])
  const [growthEvents, setGrowthEvents] = useState<GrowthEvent[]>([])
  /* 健康资料页: 体重记录表单/编辑态 */
  const [weightFormVisible, setWeightFormVisible] = useState(false)
  const [weightFormPetID, setWeightFormPetID] = useState('')
  const [weightFormValue, setWeightFormValue] = useState('')
  const [weightFormDate, setWeightFormDate] = useState('')
  const [weightFormEditingID, setWeightFormEditingID] = useState('')
  const [weightSubmitting, setWeightSubmitting] = useState(false)
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
  /* 详情弹层 (editableTitle=true 时标题以输入框呈现, 可修改提问; subtitle 仅存不再展示) */
  const [detail, setDetail] = useState<{ title: string, subtitle: string, body: string, bodyKey?: string, onSave?: (newBody: string, newTitle?: string, newDate?: string) => void, editableTitle?: boolean, meta?: string, editableDate?: boolean, image?: string } | null>(null)
  /* detail 弹层编辑缓冲 */
  const [detailDraft, setDetailDraft] = useState('')
  const [detailTitleDraft, setDetailTitleDraft] = useState('')
  const [detailDateDraft, setDetailDateDraft] = useState('')
  /* 宠物切换弹层 */
  const [switcherOpen, setSwitcherOpen] = useState(false)
  /* 档案页内联编辑状态：null=正常, 其它=对应章节进入"页面内可编辑"模式 */
  const [editingChapter, setEditingChapter] = useState<ChapterKey | null>(null)
  /* 右上角"修改"按钮滑入/滑出动画: mounted 控制是否渲染(退场动画播完再卸载),
     phase 驱动动画类: enter=右侧屏外待命 → in=纯滑动滑入; out=向右滑出 */
  const [editBtnMounted, setEditBtnMounted] = useState(false)
  const [editBtnPhase, setEditBtnPhase] = useState<'enter' | 'in' | 'out'>('enter')
  /* ===== 成长足迹添加事件 (沿用 calendar 的 cal-* 弹层 + createCalendarRecord; 仅日常) ===== */
  const [growthFormVisible, setGrowthFormVisible] = useState(false)
  const [growthFormContent, setGrowthFormContent] = useState('')
  const [growthFormDate, setGrowthFormDate] = useState<string>(() => {
    const d = new Date()
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  })
  const [growthFormPetID, setGrowthFormPetID] = useState('')
  const [growthSubmitting, setGrowthSubmitting] = useState(false)
  /* 图片上传 (与日历添加记录表单一致): 上传后拿 asset_id, 提交时随记录写入 */
  const [growthFormMediaAssetIDs, setGrowthFormMediaAssetIDs] = useState<string[]>([])
  const [growthFormLocalImagePaths, setGrowthFormLocalImagePaths] = useState<string[]>([])
  const [growthFormUploading, setGrowthFormUploading] = useState(false)

  /* ===== 生日纪念册添加记录 (通用档案资源 POST birthday-records + birthday-media) ===== */
  const [birthdayFormVisible, setBirthdayFormVisible] = useState(false)
  const [birthdayFormPetID, setBirthdayFormPetID] = useState('')
  const [birthdayFormYear, setBirthdayFormYear] = useState(String(new Date().getFullYear()))
  const [birthdayFormAge, setBirthdayFormAge] = useState('')
  const [birthdayFormSummary, setBirthdayFormSummary] = useState('')
  const [birthdaySubmitting, setBirthdaySubmitting] = useState(false)
  const [birthdayFormMediaAssetIDs, setBirthdayFormMediaAssetIDs] = useState<string[]>([])
  const [birthdayFormLocalImagePaths, setBirthdayFormLocalImagePaths] = useState<string[]>([])
  const [birthdayFormUploading, setBirthdayFormUploading] = useState(false)

  const selectedPet = pets.find(item => item.id === currentPetId) || pets[0]

  /* ===== 成长足迹/生日纪念册添加表单打开时, 隐藏底部 tab-bar (与日历页同机制), 避免遮住表单 ===== */
  const setCalendarFormVisible = useAppStore(state => state.setCalendarFormVisible)
  const anyFormVisible = growthFormVisible || birthdayFormVisible || weightFormVisible

  useEffect(() => {
    setCalendarFormVisible(anyFormVisible)
    Taro.eventCenter.trigger('calendar-form-visibility', anyFormVisible)
    return () => {
      setCalendarFormVisible(false)
      Taro.eventCenter.trigger('calendar-form-visibility', false)
    }
  }, [anyFormVisible, setCalendarFormVisible])

  /* ===== 动态分页：按数据量把每个章节拆成若干页 ===== */
  const pages = useMemo<BookPage[]>(() => {
    const list: BookPage[] = []
    const personalityParts = Math.max(1, Math.ceil(questions.length / PERSONALITY_PER_PAGE))
    const birthdayParts = Math.max(1, Math.ceil(birthdayRecords.length / BIRTHDAY_PER_PAGE))
    const certificateParts = Math.max(1, Math.ceil(certificates.length / CERTIFICATES_PER_PAGE))
    const growthParts = growthEvents.length <= GROWTH_FIRST_PAGE_EVENTS
      ? 1
      : 1 + Math.ceil((growthEvents.length - GROWTH_FIRST_PAGE_EVENTS) / GROWTH_PER_PAGE)

    const push = (chapter: ChapterKey, name: string, part: number, parts: number) => {
      list.push({ chapter, name, part, parts })
    }

    push('cover', '封面', 1, 1)
    push('identity', '身份名片', 1, 1)
    for (let i = 1; i <= certificateParts; i++) push('certificates', '证件收藏', i, certificateParts)
    push('health', '健康资料', 1, 1)
    for (let i = 1; i <= personalityParts; i++) push('personality', '个性说明书', i, personalityParts)
    for (let i = 1; i <= birthdayParts; i++) push('birthday', '生日纪念册', i, birthdayParts)
    for (let i = 1; i <= growthParts; i++) push('growth', '成长足迹', i, growthParts)
    push('back', '封底', 1, 1)
    return list
  }, [questions.length, birthdayRecords.length, growthEvents.length, certificates.length])

  const pageCount = pages.length

  /* 数据变化导致页数变少时，收回越界的当前页 */
  useEffect(() => {
    if (currentPage > pageCount - 1) {
      // eslint-disable-next-line react-hooks-extra/no-direct-set-state-in-use-effect
      setCurrentPage(0)
    }
  }, [pageCount, currentPage])

  const loadProfile = useCallback(async (pet: Pet, silent = false) => {
    /* silent: 从表单返回等场景的无感刷新, 不闪 loading 遮罩 */
    if (!silent) {
      setLoading(true)
    }
    try {
      const petProfile = await getPetProfile(pet.id)
      const [personalityItems, questionItems, records, weightItems, eventItems, birthdayMediaItems, certificateItems] = await Promise.all([
        getPetResource<PersonalityItem[]>(pet.id, 'personality'),
        getPetResource<QuestionItem[]>(pet.id, 'questions'),
        getPetResource<BirthdayRecord[]>(pet.id, 'birthday-records'),
        getPetResource<WeightRecord[]>(pet.id, 'weights'),
        getPetResource<GrowthEvent[]>(pet.id, 'growth-events'),
        getPetResource<BirthdayMediaItem[]>(pet.id, 'birthday-media'),
        getPetResource<CertificateItem[]>(pet.id, 'certificates'),
      ])
      setProfile(petProfile)
      /* 个性页: 预置标签/问答固定展示在前, 后端已有且不重复的条目追加在后 */
      const presetTraits = new Set(DEFAULT_PERSONALITY.map(tag => tag.trait))
      /* 预置问答应用本地编辑覆盖(用户改过的预置项以修改后的内容展示) */
      const questionOverrides = readQuestionOverrides(pet.id)
      const displayPresetQuestions = DEFAULT_QUESTIONS.map(q => (
        questionOverrides[q.id] ? { ...q, ...questionOverrides[q.id] } : q
      ))
      const presetQuestions = new Set(displayPresetQuestions.map(q => q.question))
      setPersonality([
        ...DEFAULT_PERSONALITY,
        ...personalityItems.filter(item => !presetTraits.has(item.trait)),
      ])
      setQuestions([
        ...displayPresetQuestions,
        ...questionItems.filter(item => !presetQuestions.has(item.question)),
      ])
      /* 生日记录按年份新→旧排序: 最新一年的记录做大图卡, 下方年份行新记录在上 */
      setBirthdayRecords([...records].sort((a, b) => b.year - a.year))
      setBirthdayMedia(Array.isArray(birthdayMediaItems) ? birthdayMediaItems : [])
      setCertificates(Array.isArray(certificateItems) ? certificateItems : [])
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

  /* ===== 弹层头像: 每只宠物拉一次 profile 取 avatar_asset_id (后端列表接口暂只返回 id/name) ===== */
  const [petAvatars, setPetAvatars] = useState<Record<string, string>>({})
  useEffect(() => {
    let cancelled = false
    async function loadAvatars() {
      const entries = await Promise.all(pets.map(async (item) => {
        try {
          const itemProfile = await getPetProfile(item.id)
          return [item.id, itemProfile.avatar_asset_id || ''] as const
        }
        catch {
          return [item.id, ''] as const
        }
      }))
      if (!cancelled) {
        setPetAvatars(Object.fromEntries(entries))
      }
    }
    if (pets.length > 0) {
      void loadAvatars()
    }
 else {
      setPetAvatars({})
    }
    return () => {
      cancelled = true
    }
  }, [pets])

  /* 从宠物表单保存返回时刷新档案: useDidShow 在每次页面显示(含 navigateBack)时触发;
     首次显示跳过, 由上方 useEffect 负责初始加载, 避免重复请求 */
  const hasShownOnceRef = useRef(false)
  useDidShow(() => {
    if (!hasShownOnceRef.current) {
      hasShownOnceRef.current = true
      return
    }
    if (selectedPet) {
      void loadProfile(selectedPet, true)
      /* 宠物名/列表也可能被表单改过, 一并刷新(失败不影响档案) */
      getPets().then(setPets).catch(() => {})
    }
  })

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
  }, [currentPage, pageCount, touchStartX])

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
  }, [currentPage, goToPage, pageCount, touchStartX])

  const openDetail = (title: string, subtitle: string, body: string, onSave?: (newBody: string, newTitle?: string, newDate?: string) => void, editableTitle?: boolean, meta?: string, editableDate?: boolean, image?: string) => {
    /* 编辑章节下若调用方未传 onSave, 自动提供一个本地保存提示 */
    let effectiveOnSave = onSave
    if (editingChapter && !onSave) {
      effectiveOnSave = (_newBody: string) => {
        Taro.showToast({ title: `已保存"${title}"的新内容到本地`, icon: 'success' })
      }
    }
    setDetail({ title, subtitle, body, onSave: effectiveOnSave, editableTitle, meta, editableDate, image })
    setDetailDraft(body)
    setDetailTitleDraft(title)
    /* 可编辑日期: 从 meta 中取出日期部分("YYYY-MM-DD · 记录者"的前段)作为草稿 */
    setDetailDateDraft(editableDate ? (meta || '').split(' · ')[0] || '' : '')
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

  /* ===== 删除宠物 (切换弹层每行右上角小叉): 两次确认后调后端 DELETE /pets/:id ===== */
  async function handleDeletePet(pet: Pet) {
    const first = await Taro.showModal({
      title: '删除档案',
      content: `确定要删除“${pet.name}”的档案吗`,
      cancelText: '取消',
      confirmText: '确定',
    })
    if (!first.confirm) {
      return
    }
    const second = await Taro.showModal({
      title: '删除档案',
      content: `真的要删除“${pet.name}”的档案吗，里面的各项记录都会丢失哦`,
      cancelText: '取消',
      confirmText: '确定',
    })
    if (!second.confirm) {
      return
    }
    try {
      await deletePet(pet.id)
      const rest = pets.filter(item => item.id !== pet.id)
      setPets(rest)
      /* 删除的是当前展示中的宠物时: 切到剩余第一只; 一只都不剩则清空当前宠物 */
      if (currentPetId === pet.id) {
        if (rest.length > 0) {
          setCurrentPetId(rest[0].id)
        }
 else {
          clearCurrentPet()
        }
        setEditingChapter(null)
        setCurrentPage(0)
      }
      Taro.showToast({ title: '已删除', icon: 'success' })
    }
    catch (error) {
      Taro.showToast({ title: error instanceof Error ? error.message : '删除失败，请重试', icon: 'none' })
    }
  }

  /* ===== 成长足迹添加事件 ===== */
  function openGrowthForm() {
    const targetPetID = currentPetId || pets[0]?.id || ''
    const today = new Date()
    const todayStr = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
    setGrowthFormContent('')
    setGrowthFormDate(todayStr)
    setGrowthFormPetID(targetPetID)
    setGrowthFormMediaAssetIDs([])
    setGrowthFormLocalImagePaths([])
    setGrowthFormUploading(false)
    setGrowthFormVisible(true)
  }

  /* 选图并上传到资产存储 (与日历页 handleChooseImage 同款) */
  async function handleGrowthChooseImage() {
    try {
      const result = await Taro.chooseImage({
        count: Math.min(9 - growthFormLocalImagePaths.length, 9),
        sizeType: ['compressed'],
        sourceType: ['album', 'camera'],
      })
      if (!result.tempFilePaths.length) {
        return
      }
      setGrowthFormUploading(true)
      const uploaded = await Promise.all(result.tempFilePaths.map(filePath => uploadCalendarImage(filePath)))
      setGrowthFormMediaAssetIDs(previous => [...previous, ...uploaded.map(item => item.asset_id)])
      setGrowthFormLocalImagePaths(previous => [...previous, ...result.tempFilePaths])
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '图片上传失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setGrowthFormUploading(false)
    }
  }

  function closeGrowthForm() {
    setGrowthFormVisible(false)
  }

  /* ===== 生日纪念册添加记录 ===== */
  function openBirthdayForm() {
    const targetPetID = currentPetId || pets[0]?.id || ''
    const now = new Date()
    const year = now.getFullYear()
    /* 默认几岁: 用宠物生日的年份推算, 推不出来留空让用户填 */
    const birthYear = Number.parseInt((profile?.birthday || '').slice(0, 4), 10)
    setBirthdayFormPetID(targetPetID)
    setBirthdayFormYear(String(year))
    setBirthdayFormAge(Number.isFinite(birthYear) && birthYear > 0 ? String(year - birthYear) : '')
    setBirthdayFormSummary('')
    setBirthdayFormMediaAssetIDs([])
    setBirthdayFormLocalImagePaths([])
    setBirthdayFormUploading(false)
    setBirthdayFormVisible(true)
  }

  function closeBirthdayForm() {
    setBirthdayFormVisible(false)
  }

  /* 选图并上传到资产存储 (与成长足迹表单同款) */
  async function handleBirthdayChooseImage() {
    try {
      const result = await Taro.chooseImage({
        count: Math.min(9 - birthdayFormLocalImagePaths.length, 9),
        sizeType: ['compressed'],
        sourceType: ['album', 'camera'],
      })
      if (!result.tempFilePaths.length) {
        return
      }
      setBirthdayFormUploading(true)
      const uploaded = await Promise.all(result.tempFilePaths.map(filePath => uploadCalendarImage(filePath)))
      setBirthdayFormMediaAssetIDs(previous => [...previous, ...uploaded.map(item => item.asset_id)])
      setBirthdayFormLocalImagePaths(previous => [...previous, ...result.tempFilePaths])
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '图片上传失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setBirthdayFormUploading(false)
    }
  }

  async function handleCreateBirthdayRecord() {
    if (birthdaySubmitting) {
      return
    }
    const petID = birthdayFormPetID
    if (!petID) {
      await Taro.showToast({ title: '请选择宠物', icon: 'none' })
      return
    }
    const year = Number.parseInt(birthdayFormYear, 10)
    if (!Number.isFinite(year) || year < 1990 || year > 2100) {
      await Taro.showToast({ title: '请输入正确的年份', icon: 'none' })
      return
    }
    const age = Number.parseInt(birthdayFormAge, 10)
    if (!Number.isFinite(age) || age < 0 || age > 50) {
      await Taro.showToast({ title: '请输入正确的年龄', icon: 'none' })
      return
    }
    if (birthdayFormUploading) {
      return
    }
    setBirthdaySubmitting(true)
    try {
      /* 1) 写入生日记录(year/age/summary), 后端返回带 id 的完整 payload */
      const created = await createPetResource(petID, 'birthday-records', {
        year,
        age,
        summary: birthdayFormSummary.trim(),
      })
      const recordID = typeof created?.id === 'string' ? created.id : `local-${Date.now()}`
      /* 2) 关联照片: 逐张写入 birthday-media */
      if (birthdayFormMediaAssetIDs.length) {
        await Promise.all(birthdayFormMediaAssetIDs.map(assetID => createPetResource(petID, 'birthday-media', {
          record_id: recordID,
          type: 'photo',
          asset_id: assetID,
        })))
      }
      /* 3) 本地追加, 与加载排序一致: 年份新→旧, 最新一年的记录展示在大图卡 */
      setBirthdayRecords(previous => [...previous, { id: recordID, year, age, summary: birthdayFormSummary.trim() }].sort((a, b) => b.year - a.year))
      /* 4) 照片同步进本地状态, 详情卡与首图立即可见 */
      if (birthdayFormMediaAssetIDs.length) {
        setBirthdayMedia(previous => [...previous, ...birthdayFormMediaAssetIDs.map((assetID, index) => ({
          id: `${recordID}-media-${index}`,
          record_id: recordID,
          type: 'photo',
          asset_id: assetID,
        }))])
      }
      setBirthdayFormVisible(false)
      await Taro.showToast({ title: '生日已记录', icon: 'success' })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '保存生日记录失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setBirthdaySubmitting(false)
    }
  }

  /* ===== 证件收藏: 添加证件表单 (复用 cal-* 弹层样式) ===== */
  const [certFormVisible, setCertFormVisible] = useState(false)
  const [certFormPetID, setCertFormPetID] = useState('')
  const [certFormName, setCertFormName] = useState('')
  const [certFormDate, setCertFormDate] = useState('')
  const [certFormDetails, setCertFormDetails] = useState('')
  const [certFormAssetID, setCertFormAssetID] = useState('')
  const [certFormUploading, setCertFormUploading] = useState(false)
  const [certSubmitting, setCertSubmitting] = useState(false)

  function openCertForm() {
    setCertFormPetID(currentPetId || pets[0]?.id || '')
    setCertFormName('')
    setCertFormDate('')
    setCertFormDetails('')
    setCertFormAssetID('')
    setCertFormUploading(false)
    setCertSubmitting(false)
    setCertFormVisible(true)
  }

  function closeCertForm() {
    setCertFormVisible(false)
  }

  async function handleCertChooseImage() {
    try {
      const result = await Taro.chooseImage({
        count: 1,
        sizeType: ['compressed'],
        sourceType: ['album', 'camera'],
      })
      const filePath = result.tempFilePaths?.[0]
      if (!filePath) {
        return
      }
      setCertFormUploading(true)
      const uploaded = await uploadFile<{ asset_id: string }>({
        path: '/api/v1/assets/upload',
        filePath,
        name: 'file',
        formData: { type: 'certificate' },
      })
      setCertFormAssetID(uploaded.asset_id)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '图片上传失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setCertFormUploading(false)
    }
  }

  async function handleCreateCertificate() {
    if (certSubmitting) {
      return
    }
    const petID = certFormPetID
    if (!petID) {
      await Taro.showToast({ title: '请选择宠物', icon: 'none' })
      return
    }
    const name = certFormName.trim()
    if (!name) {
      await Taro.showToast({ title: '请填写证件名称', icon: 'none' })
      return
    }
    if (certFormUploading) {
      return
    }
    setCertSubmitting(true)
    try {
      /* 后端证书资源字段: type/name/number/occurred_at/details/asset_id (POST 需全量携带) */
      const created = await createPetResource(petID, 'certificates', {
        type: 'certificate',
        name,
        number: '',
        occurred_at: certFormDate,
        details: certFormDetails.trim(),
        asset_id: certFormAssetID,
      })
      const recordID = typeof created?.id === 'string' ? created.id : `local-${Date.now()}`
      setCertificates(previous => [...previous, {
        id: recordID,
        type: 'certificate',
        name,
        number: '',
        occurred_at: certFormDate,
        details: certFormDetails.trim(),
        asset_id: certFormAssetID,
      }])
      Taro.showToast({ title: '已收录证件', icon: 'success' })
      closeCertForm()
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '保存证件失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setCertSubmitting(false)
    }
  }

  /* ===== 健康资料: 体重记录 (通用资源 weights: measured_at + weight) =====
     - 健康页展示最近三次体重, 最新一条为"当前体重"
     - 编辑态: 点历史行=修改(预填弹层 PATCH), 行右侧小叉=删除(确认后 DELETE)
     - 体重记录与档案共用数据源, 成长足迹页的体重卡会同步反映 */
  function todayString() {
    const now = new Date()
    return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  }

  function openWeightForm() {
    setWeightFormPetID(currentPetId || pets[0]?.id || '')
    setWeightFormValue('')
    setWeightFormDate(todayString())
    setWeightFormEditingID('')
    setWeightSubmitting(false)
    setWeightFormVisible(true)
  }

  function openWeightEdit(record: WeightRecord) {
    setWeightFormPetID(currentPetId || pets[0]?.id || '')
    setWeightFormValue(String(record.weight))
    setWeightFormDate((record.measured_at || '').slice(0, 10))
    setWeightFormEditingID(record.id)
    setWeightSubmitting(false)
    setWeightFormVisible(true)
  }

  function closeWeightForm() {
    setWeightFormVisible(false)
  }

  async function handleSaveWeight() {
    if (weightSubmitting) {
      return
    }
    const petID = weightFormPetID
    if (!petID) {
      await Taro.showToast({ title: '请选择宠物', icon: 'none' })
      return
    }
    const raw = weightFormValue.trim()
    const parsed = Number(raw)
    if (!raw || !Number.isFinite(parsed)) {
      await Taro.showToast({ title: '请填写体重数值', icon: 'none' })
      return
    }
    if (parsed < WEIGHT_MIN_KG || parsed > WEIGHT_MAX_KG) {
      await Taro.showToast({ title: `体重请填写 ${WEIGHT_MIN_KG}–${WEIGHT_MAX_KG} kg`, icon: 'none' })
      return
    }
    if (!weightFormDate) {
      await Taro.showToast({ title: '请选择测量日期', icon: 'none' })
      return
    }
    const weight = Number(parsed.toFixed(2))
    setWeightSubmitting(true)
    try {
      if (weightFormEditingID) {
        await updatePetResource(petID, 'weights', weightFormEditingID, { measured_at: weightFormDate, weight })
        setWeights(previous => previous.map(item => (
          item.id === weightFormEditingID ? { ...item, measured_at: weightFormDate, weight } : item
        )))
        Taro.showToast({ title: '已保存修改', icon: 'success' })
      }
      else {
        const created = await createPetResource(petID, 'weights', { measured_at: weightFormDate, weight })
        const recordID = typeof created?.id === 'string' ? created.id : `local-${Date.now()}`
        setWeights(previous => [...previous, { id: recordID, measured_at: weightFormDate, weight }])
        Taro.showToast({ title: '已记录体重', icon: 'success' })
      }
      closeWeightForm()
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '保存体重失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setWeightSubmitting(false)
    }
  }

  async function handleDeleteWeight(record: WeightRecord) {
    const petID = currentPetId || pets[0]?.id || ''
    if (!petID) {
      return
    }
    const confirmed = await Taro.showModal({
      title: '删除体重记录',
      content: `确定删除 ${(record.measured_at || '').slice(0, 10)} 的体重记录（${fmtWeight(record.weight)} kg）吗？删除后无法找回哦`,
      cancelText: '取消',
      confirmText: '确定',
    })
    if (!confirmed.confirm) {
      return
    }
    try {
      await deletePetResource(petID, 'weights', record.id)
      setWeights(previous => previous.filter(item => item.id !== record.id))
      Taro.showToast({ title: '已删除', icon: 'success' })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '删除失败,请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
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
    if (!growthFormContent.trim() && !growthFormMediaAssetIDs.length) {
      await Taro.showToast({ title: '写点内容或添加图片', icon: 'none' })
      return
    }
    if (growthFormUploading) {
      return
    }
    setGrowthSubmitting(true)
    const occurredAt = `${growthFormDate}T12:00:00+09:00`
    try {
      /* 1) 写入后端日历记录(同步到日历), 成长足迹仅支持日常类型; 图片随记录一并写入 */
      const createdRecord = await createCalendarRecord({
        category: 'daily',
        pet_id: petID,
        content: growthFormContent.trim() || undefined,
        media_asset_ids: growthFormMediaAssetIDs.length ? growthFormMediaAssetIDs : undefined,
        occurred_at: occurredAt,
      })
      /* 2) 立即在档案页成长足迹中追加一条(乐观更新,无需等后端推送); 记下日历记录 ID 备后续同步编辑 */
      const newEvent: GrowthEvent = {
        id: `local-${Date.now()}`,
        type: '日常',
        occurred_at: occurredAt,
        recorder: '我',
        content: growthFormContent.trim(),
        calendar_record_id: createdRecord?.id,
      }
      /* 2) 写入本地补充缓存 + 乐观更新, 保证"事件记录"下立即出现 */
      writeLocalGrowthEvents(petID, [newEvent, ...readLocalGrowthEvents(petID)])
      setGrowthEvents(previous => [newEvent, ...previous])
      setGrowthFormVisible(false)
      setGrowthFormMediaAssetIDs([])
      setGrowthFormLocalImagePaths([])
      await Taro.showToast({ title: '已记一笔', icon: 'success' })
      /* 3) 后台静默重拉并与本地缓存合并: 服务端映射出的正式记录自然取代本地记录 */
      void getPetResource<GrowthEvent[]>(petID, 'growth-events')
        .then((fresh) => {
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

  /* ===== 章节: 身份名片 (人设卡排版: 左上照片 + 右上简洁信息 + 下方详细介绍) ===== */
  const renderProfilePage = () => {
    const petName = profile?.name || selectedPet?.name || '宠'
    /* 详细介绍: 由档案字段自动生成一句话简介 */
    const bioParts: string[] = []
    if (profile?.breed) {
      bioParts.push(`是一只${profile.breed}`)
    }
    if (profile?.gender) {
      bioParts.push(`性别${formatGender(profile.gender)}`)
    }
    if (profile?.birthday) {
      bioParts.push(`${formatDate(profile.birthday)} 出生，现在 ${profile.age} 岁`)
    }
    if (profile?.home_date) {
      bioParts.push(`${formatDate(profile.home_date)} 来到家里，已陪伴我们 ${profile.companion_days} 天`)
    }
    if (profile?.birthday && profile.next_birthday_days !== undefined) {
      bioParts.push(`下一次生日还有 ${profile.next_birthday_days} 天`)
    }
    const bioText = bioParts.length > 0
      ? `${petName} ${bioParts.join('，')}。`
      : '资料还空空的，点击右上角「修改」补充它的品种、生日和到家日期，这里会自动生成它的专属简介。'
    return (
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
          <View className="identity-photo">
            {profile?.avatar_asset_id
              ? <Image className="identity-photo-img" src={assetURL(profile.avatar_asset_id)} mode="aspectFill" />
              : <Text className="identity-photo-char">{petName.slice(0, 1)}</Text>}
          </View>
          <View className="identity-meta">
            <Text className="identity-about">About.</Text>
            <View className="identity-meta-divider" />
            <View className="identity-fact">
              <Text className="identity-fact-key">年龄</Text>
              <Text className="identity-fact-val">{profile?.birthday ? `${profile.age} 岁` : '待补充'}</Text>
            </View>
            <View className="identity-fact">
              <Text className="identity-fact-key">性别</Text>
              <Text className="identity-fact-val">{formatGender(profile?.gender || '')}</Text>
            </View>
            <View className="identity-fact">
              <Text className="identity-fact-key">品种</Text>
              <Text className="identity-fact-val">{profile?.breed || '待补充'}</Text>
            </View>
            <View className="identity-fact">
              <Text className="identity-fact-key">出生</Text>
              <Text className="identity-fact-val">{formatDate(profile?.birthday)}</Text>
            </View>
            <View className="identity-fact">
              <Text className="identity-fact-key">到家</Text>
              <Text className="identity-fact-val">{formatDate(profile?.home_date)}</Text>
            </View>
          </View>
        </View>
        <View className="identity-bio">
          <View className="identity-bio-head">
            <Text className="identity-bio-name">{petName}</Text>
            <View className="identity-bio-title">
              <Text className="identity-bio-zh">简介</Text>
              <Text className="identity-bio-en">Info.</Text>
            </View>
          </View>
          <Text className="identity-bio-text">{bioText}</Text>
        </View>
        {editingChapter === 'identity' && (
          <View className="inline-edit-add" onClick={() => Taro.showToast({ title: '编辑身份信息: 后续版本支持', icon: 'none' })}>＋ 编辑身份信息</View>
        )}
      </View>
    </View>
    )
  }

  /* ===== 章节: 证件收藏 (每行两张证件卡, 名称紧贴卡片上沿; 卡内左图右时间+说明) ===== */
  const renderCertificatesPage = (part: number) => {
    const start = (part - 1) * CERTIFICATES_PER_PAGE
    const partCerts = certificates.slice(start, start + CERTIFICATES_PER_PAGE)
    const showPlaceholder = certificates.length === 0 && part === 1
    return (
      <View className="page content-page" style={PAGE_PAPER_STYLE}>
        <View className="page-body">
          <View className="page-head">
            <View className="page-head-title">
              <Text className="page-head-en">{CHAPTER_EN.certificates}</Text>
              <Text className="page-head-zh">证件收藏</Text>
            </View>
            <View className="page-head-divider" />
          </View>
          <View className="cert-grid">
            {showPlaceholder && (
              <View className="cert-cell">
                <Text className="cert-name">{PLACEHOLDER_CERTIFICATE.name}</Text>
                <View className="cert-card cert-card--placeholder">
                  <View className="cert-photo">
                    <Text className="cert-photo-char">证</Text>
                  </View>
                  <View className="cert-info">
                    <Text className="cert-time">{PLACEHOLDER_CERTIFICATE.time}</Text>
                    <Text className="cert-details">{PLACEHOLDER_CERTIFICATE.details}</Text>
                  </View>
                </View>
              </View>
            )}
            {partCerts.map(cert => (
              <View className="cert-cell" key={cert.id}>
                <Text className="cert-name">{cert.name}</Text>
                <View className="cert-card">
                  <View className="cert-photo">
                    {cert.asset_id
                      ? <Image className="cert-photo-img" src={assetURL(cert.asset_id)} mode="aspectFill" />
                      : <Text className="cert-photo-char">{cert.name.slice(0, 1)}</Text>}
                  </View>
                  <View className="cert-info">
                    <Text className="cert-time">{cert.occurred_at ? formatDate(cert.occurred_at) : '时间待补充'}</Text>
                    <Text className="cert-details">{cert.details || '暂无说明'}</Text>
                  </View>
                </View>
              </View>
            ))}
            {/* 添加入口: 仅在右下角"修改"进入编辑态后出现 */}
            {editingChapter === 'certificates' && (
              <View className="cert-cell" onClick={openCertForm}>
                <View className="cert-add">
                  <Text className="cert-add-plus">＋</Text>
                  <Text className="cert-add-text">添加证件</Text>
                </View>
              </View>
            )}
          </View>
        </View>
      </View>
    )
  }

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
              {personality.map((item, tagIndex) => {
                const tagBody = `${item.trait}${item.value ? ` · ${item.value}` : ''}`
                const isEditing = editingChapter === 'personality'
                return (
                  <View
                    className={`tag-capsule tag-capsule--c${tagIndex % 6}${isEditing ? ' editable' : ''}`}
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
                onClick={() => {
                  if (editingChapter === 'personality') {
                    /* 编辑态: 标题(提问)与内容(回答)均可修改, 标题为空时保留原提问 */
                    openDetail(
                      q.question,
                      '',
                      q.answer || '',
                      (newBody: string, newTitle?: string) => {
                        const petID = selectedPet?.id
                        if (!petID) {
                          return
                        }
                        const nextTitle = (newTitle ?? '').trim() || q.question
                        const nextAnswer = newBody.trim()
                        /* 本地先更新, 弹窗关闭后立即可见 */
                        setQuestions(previous => previous.map(item => (
                          item.id === q.id ? { ...item, question: nextTitle, answer: nextAnswer } : item
                        )))
                        if (q.id.startsWith('preset-')) {
                          /* 预置问答不是服务端记录: 写入本地覆盖缓存, 重拉合并后不回退 */
                          const overrides = readQuestionOverrides(petID)
                          overrides[q.id] = { question: nextTitle, answer: nextAnswer }
                          writeQuestionOverrides(petID, overrides)
                          Taro.showToast({ title: '已保存修改', icon: 'success' })
                        }
                        else {
                          /* 服务端问答: PATCH 持久化到后端 */
                          void updatePetResource(petID, 'questions', q.id, { question: nextTitle, answer: nextAnswer })
                            .then(() => Taro.showToast({ title: '已保存修改', icon: 'success' }))
                            .catch(() => Taro.showToast({ title: '保存失败,请重试', icon: 'none' }))
                        }
                      },
                      true,
                    )
                  }
                  else {
                    openDetail(q.question, q.answer || '暂无回答', q.answer || '可以点击编辑补充更多关于它的描述。')
                  }
                }}
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
            <View
              className="health-item health-tone-1 detail-trigger"
              onClick={() => openDetail('过敏信息', profile?.breed ? `${profile.breed} 品种` : '暂无记录', '过敏信息由家庭成员补充。常见包括食物、环境与药物，记录后会显示在这里。')}
            >
              <Text className="health-label">过敏信息</Text>
              <Text className="health-value">{profile ? '待补充' : '—'}</Text>

            </View>
            <View
              className="health-item health-tone-2 detail-trigger"
              onClick={() => openDetail('既往疾病', '暂无记录', '在这里汇总既往病史、检查报告与治疗过程，方便家庭医生快速了解情况。')}
            >
              <Text className="health-label">既往疾病</Text>
              <Text className="health-value">暂无</Text>

            </View>
            <View
              className="health-item health-tone-3 detail-trigger"
              onClick={() => openDetail('长期用药', '目前无用药', '本模块只保存档案，不提供药物剂量建议；具体用药请遵医嘱。')}
            >
              <Text className="health-label">长期用药</Text>
              <Text className="health-value">无</Text>

            </View>
            <View
              className="health-item health-tone-4 detail-trigger"
              onClick={() => openDetail('最近疫苗', '待补充', '记录最近一次疫苗的种类、接种时间与医院，凭证仅家庭成员可见。')}
            >
              <Text className="health-label">最近疫苗</Text>
              <Text className="health-value">—</Text>

            </View>
          </View>
          <Text className="updated">点击查看详情</Text>
        </View>
      </View>
    )
  }

  /* ===== 章节: 生日纪念册（每页 3 条记录，超出自动开新页） ===== */
  /* 生日记录关联的第一张照片 URL (无照片返回空串) */
  const birthdayRecordImage = (recordID: string) => {
    const media = birthdayMedia.find(item => item.record_id === recordID)
    return media ? assetURL(media.asset_id) : ''
  }

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
            /* 详情卡附照片: 取该记录的第一张生日照片(有才显示, 只显示一张) */
            const recordImage = birthdayRecordImage(r.id)
            const open = () => openDetail(`${r.age} 生日`, r.summary || '暂无简介', `出生于 ${r.year} 年。这一年的故事是：${r.summary || '正在补充中'}。`, undefined, undefined, undefined, undefined, recordImage)
            if (start + i === 0) {
              return (
                <View className="birthday-feature" key={r.id} onClick={open}>
                  <View className="media-placeholder">
                    {recordImage
                      ? <Image className="media-placeholder-image" src={recordImage} mode="aspectFill" />
                      : '📷'}
                  </View>
                  <View className="birthday-copy">
                    <Image className="birthday-copy-bg" src={catPhoto} mode="aspectFill" />
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
            <View className="inline-edit-add" onClick={openBirthdayForm}>＋ 添加生日记录</View>
          )}
        </View>
      </View>
    )
  }

  /* ===== 章节: 成长足迹（第 1 页：体重卡 + 2 条事件；之后每页 3 条） ===== */
  const renderGrowthPage = (part: number) => {
    /* 体重按测量日期新→旧, 只取最近三次展示 */
    const sortedWeights = [...weights].sort((a, b) => b.measured_at.localeCompare(a.measured_at))
    const recentWeights = sortedWeights.slice(0, HEALTH_WEIGHT_PREVIEW)
    const wLatest = recentWeights[0]
    const wPrev = recentWeights[1]
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
                  ? openDetail('体重记录', `当前 ${fmtWeight(wLatest.weight)} kg，比上次${wDiff < 0 ? '减少' : '增加'} ${Math.abs(wDiff).toFixed(2)} kg`, `最近记录：${recentWeights.map(w => `${w.measured_at} ${fmtWeight(w.weight)} kg`).join('；')}。`)
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
              {/* 体重管理: 最近三次, 编辑态可改可删, 可添加 */}
              <View className="health-weight">
                {recentWeights.map((item, index) => (
                  <View
                    className={`weight-row${isEditing ? ' editable' : ''}`}
                    key={item.id}
                    onClick={() => {
                      if (isEditing) {
                        openWeightEdit(item)
                      }
                    }}
                  >
                    <Text className="weight-row-date">{(item.measured_at || '').slice(0, 10)}</Text>
                    {index === 0 && <Text className="weight-row-badge">最新</Text>}
                    <Text className="weight-row-value">
                      {fmtWeight(item.weight)}
                      {' kg'}
                    </Text>
                    {isEditing && (
                      <View
                        className="weight-row-del"
                        onClick={(event) => {
                          event.stopPropagation()
                          void handleDeleteWeight(item)
                        }}
                      >
                        ×
                      </View>
                    )}
                  </View>
                ))}
                {recentWeights.length === 0 && (
                  <Text className="weight-empty">还没有体重记录，编辑页面后可以记下第一次称重。</Text>
                )}
                {isEditing && (
                  <View className="inline-edit-add" onClick={openWeightForm}>＋ 记录体重</View>
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
                onClick={() => {
                  const meta = `${(ev.occurred_at || '').slice(0, 10)} · ${ev.recorder || '我'}`
                  if (editingChapter === 'growth') {
                    /* 编辑态: 标题与日期均为点击可修改(下划线输入框), 内容走"记录内容"; 日期仅替换前段 */
                    openDetail(
                      ev.type,
                      '',
                      ev.content,
                      (newBody: string, newTitle?: string, newDate?: string) => {
                        const petID = selectedPet?.id
                        if (!petID) {
                          return
                        }
                        const nextTitle = (newTitle ?? '').trim() || ev.type
                        const nextDate = (newDate ?? '').trim() || ev.occurred_at.slice(0, 10)
                        const nextContent = newBody.trim()
                        const nextOccurredAt = (ev.occurred_at || `${nextDate}T00:00:00Z`).replace(/^\d{4}-\d{2}-\d{2}/, nextDate)
                        /* 本地先更新, 保证弹窗关闭后立即可见 */
                        setGrowthEvents(previous => previous.map(item => (
                          item.id === ev.id
                            ? { ...item, type: nextTitle, content: nextContent, occurred_at: nextOccurredAt }
                            : item
                        )))
                        if (ev.id.startsWith('local-')) {
                          /* 本地补充记录: 同步写入本地缓存, 否则重新进入页面 loadProfile 合并旧缓存后改动会被覆盖 */
                          const stored = readLocalGrowthEvents(petID)
                          writeLocalGrowthEvents(petID, stored.map(item => (
                            item.id === ev.id
                              ? { ...item, type: nextTitle, content: nextContent, occurred_at: nextOccurredAt }
                              : item
                          )))
                          /* 同步更新对应的日历记录(表单添加的事件都写了一份到日历), 失败不影响档案侧 */
                          if (ev.calendar_record_id) {
                            void updateCalendarRecord(ev.calendar_record_id, { content: nextContent, occurred_at: nextOccurredAt }).catch(() => {})
                          }
                          Taro.showToast({ title: '已保存修改', icon: 'success' })
                        }
                        else {
                          /* 服务端正式记录: PATCH 持久化到后端, 否则重新加载后会被服务端旧数据覆盖 */
                          void updatePetResource(petID, 'growth-events', ev.id, {
                            type: nextTitle,
                            occurred_at: nextOccurredAt,
                            content: nextContent,
                          })
                            .then(() => Taro.showToast({ title: '已保存修改', icon: 'success' }))
                            .catch(() => Taro.showToast({ title: '保存失败,请重试', icon: 'none' }))
                        }
                      },
                      true,
                      meta,
                      true,
                    )
                  }
 else {
                    /* 非编辑态: 只读查看 */
                    openDetail(ev.type, ev.content, ev.content, undefined, undefined, meta)
                  }
                }}
              >
                <Text className="time">
{(ev.occurred_at || '').slice(0, 10)}
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
      case 'certificates':
        body = renderCertificatesPage(info.part)
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

  /* 右上角"修改"按钮: 出现时从屏幕右侧滑入, 消失时向右滑出(封面/封底不显示)。
     章节在非封面/封底页之间切换时 active 保持 true, 按钮不重复播动画 */
  const editBtnActive = Boolean(currentPageInfo) && currentPageInfo.chapter !== 'cover' && currentPageInfo.chapter !== 'back'
  useEffect(() => {
    if (editBtnActive) {
      setEditBtnMounted(true)
      setEditBtnPhase('enter')
      /* 下一拍再切入 in, 确保初始 enter 态(屏外)先完成样式提交, 动画才能从右侧起跑 */
      const timer = setTimeout(() => setEditBtnPhase('in'), 30)
      return () => clearTimeout(timer)
    }
    setEditBtnPhase('out')
    const timer = setTimeout(() => setEditBtnMounted(false), 320)
    return () => clearTimeout(timer)
  }, [editBtnActive])

  return (
    <View className="archive-page">
      <Image className="archive-background" src={backgroundImage} mode="aspectFill" />

      {/* 右上角添加宠物按钮已移除：与切换弹层底部"点击添加宠物"入口重复 */}

      {/* 左上角宠物切换：仅宠物名 + 三角标 */}
      <Button
        className="pet-switcher-button"
        onClick={() => setSwitcherOpen(true)}
      >
        <Text className="pet-switcher-name">{selectedPet?.name || '暂未选择'}</Text>
        <Text className="pet-switcher-caret">▾</Text>
      </Button>

      {/* 右上角档案外编辑入口：作用于当前所在章节（封面/封底不显示; 带右侧滑入/滑出动画） */}
      {editBtnMounted && currentPageInfo && (
        <View
          className={`page-edit-button edit-btn-${editBtnPhase}${editingChapter === currentPageInfo.chapter ? ' editing' : ''}`}
          onClick={() => handleEditPage(currentPageInfo.chapter)}
        >
          <Text className="page-edit-icon">{editingChapter === currentPageInfo.chapter ? '√' : '✎'}</Text>
          <Text className="page-edit-text">{editingChapter === currentPageInfo.chapter ? '完成' : '修改'}</Text>
        </View>
      )}

      <View className={`book-container${currentPage === 0 ? ' is-cover' : ''}`} style={{ aspectRatio: BOOK_RATIO }} onTouchStart={handleTouchStart} onTouchMove={handleTouchMove} onTouchEnd={handleTouchEnd}>
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

      {/* 详情弹层 (支持编辑模式: 当 openDetail 传入 onSave 时, 自动切换为可编辑 Textarea;
          editableTitle=true 时标题变为可编辑输入框, subtitle 灰色小字已按需求移除) */}
      <View
        className={`overlay${detail ? ' open' : ''}`}
        onClick={() => {
          setDetail(null)
          setDetailDraft('')
          setDetailTitleDraft('')
          setDetailDateDraft('')
        }}
      >
        <View className="detail-card" onClick={event => event.stopPropagation()}>
          <View className="detail-card-head">
            <View className="detail-card-head-main">
              {detail?.editableTitle && detail?.onSave
                ? (
                  <Input
                    className="detail-title-input"
                    value={detailTitleDraft}
                    maxlength={30}
                    onInput={event => setDetailTitleDraft(event.detail.value)}
                  />
                )
                : <Text className="h2">{detail?.title || '详情'}</Text>}
              {!!detail?.meta && (
                detail?.editableDate && detail?.onSave
                  ? (
                    <View className="detail-meta-row">
                      <Input
                        className="detail-date-input"
                        value={detailDateDraft}
                        maxlength={10}
                        onInput={event => setDetailDateDraft(event.detail.value)}
                      />
                      <Text className="detail-meta">{(detail.meta || '').split(' · ').slice(1).join(' · ') ? ` · ${(detail.meta || '').split(' · ').slice(1).join(' · ')}` : ''}</Text>
                    </View>
                  )
                  : <Text className="detail-meta">{detail.meta}</Text>
              )}
            </View>
            <Button
              className="close-button"
              onClick={() => {
                setDetail(null)
                setDetailDraft('')
                setDetailTitleDraft('')
                setDetailDateDraft('')
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
          {!!detail?.image && (
            <View className="detail-block detail-block--media">
              <Text className="span">照片</Text>
              <Image
                className="detail-image"
                src={detail.image}
                mode="aspectFill"
                onClick={() => Taro.previewImage({ urls: [detail.image as string] })}
              />
            </View>
          )}
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
                  detail.onSave?.(detailDraft, detailTitleDraft, detailDateDraft)
                  setDetail(null)
                  setDetailDraft('')
                  setDetailTitleDraft('')
                  setDetailDateDraft('')
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
                  {petAvatars[pet.id]
                    ? <Image className="pet-switcher-item-avatar-img" src={assetURL(petAvatars[pet.id])} mode="aspectFill" />
                    : pet.name.slice(0, 1)}
                </View>
                <View className="pet-switcher-item-meta">
                  <Text className="strong">{pet.name}</Text>
                  <Text className="span">{pet.id === currentPetId ? '当前展示中' : '点击翻开此档案'}</Text>
                </View>
                <View
                  className="pet-switcher-delete"
                  onClick={(event) => {
                    event.stopPropagation()
                    void handleDeletePet(pet)
                  }}
                >
                  <Text className="pet-switcher-delete-x">×</Text>
                </View>
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

            <View className="cal-image-actions">
              <View className="cal-image-button" onClick={handleGrowthChooseImage}>{growthFormUploading ? '上传中' : '添加图片'}</View>
              {growthFormLocalImagePaths.length > 0 && (
                <Text>
                  已选择
                  {growthFormLocalImagePaths.length}
                  /9 张
                </Text>
              )}
            </View>

            <View className="cal-sheet-actions">
              <View className="cal-cancel-button" onClick={closeGrowthForm}>取消</View>
              <View className={`cal-save-button${growthSubmitting ? ' disabled' : ''}`} onClick={handleCreateGrowthEvent}>{growthSubmitting ? '保存中' : '保存'}</View>
            </View>
          </View>
        </View>
      )}

      {/* 生日纪念册添加记录弹层 (复用 cal-* 弹层样式) */}
      {birthdayFormVisible && (
        <View className="cal-overlay" onClick={closeBirthdayForm}>
          <View className="cal-sheet" onClick={event => event.stopPropagation()}>
            <View className="cal-sheet-handle" />
            <Text className="cal-sheet-title">添加生日记录</Text>

            <Text className="cal-field-label">宠物</Text>
            <View className="cal-pet-chips">
              {pets.map(pet => (
                <View key={pet.id} className={`cal-pet-chip${birthdayFormPetID === pet.id ? ' selected' : ''}`} onClick={() => setBirthdayFormPetID(pet.id)}>{pet.name}</View>
              ))}
            </View>

            <View className="cal-form-row">
              <View className="cal-form-col">
                <Text className="cal-field-label">年份</Text>
                <Input className="cal-input" type="number" value={birthdayFormYear} maxlength={4} placeholder="如 2025" onInput={event => setBirthdayFormYear(event.detail.value)} />
              </View>
              <View className="cal-form-col">
                <Text className="cal-field-label">几岁</Text>
                <Input className="cal-input" type="number" value={birthdayFormAge} maxlength={2} placeholder="如 1" onInput={event => setBirthdayFormAge(event.detail.value)} />
              </View>
            </View>

            <Text className="cal-field-label">这一岁的故事</Text>
            <Textarea className="cal-textarea" value={birthdayFormSummary} maxlength={500} placeholder="记录这一年里值得留念的事" onInput={event => setBirthdayFormSummary(event.detail.value)} />

            <View className="cal-image-actions">
              <View className="cal-image-button" onClick={handleBirthdayChooseImage}>{birthdayFormUploading ? '上传中' : '添加照片'}</View>
              {birthdayFormLocalImagePaths.length > 0 && (
                <Text>
                  已选择
                  {birthdayFormLocalImagePaths.length}
                  /9 张
                </Text>
              )}
            </View>

            <View className="cal-sheet-actions">
              <View className="cal-cancel-button" onClick={closeBirthdayForm}>取消</View>
              <View className={`cal-save-button${birthdaySubmitting ? ' disabled' : ''}`} onClick={handleCreateBirthdayRecord}>{birthdaySubmitting ? '保存中' : '保存'}</View>
            </View>
          </View>
        </View>
      )}
      {/* 添加证件弹层 (复用 cal-* 弹层样式) */}
      {certFormVisible && (
        <View className="cal-overlay" onClick={closeCertForm}>
          <View className="cal-sheet" onClick={event => event.stopPropagation()}>
            <View className="cal-sheet-handle" />
            <Text className="cal-sheet-title">添加证件</Text>

            <Text className="cal-field-label">宠物</Text>
            <View className="cal-pet-chips">
              {pets.map(pet => (
                <View key={pet.id} className={`cal-pet-chip${certFormPetID === pet.id ? ' selected' : ''}`} onClick={() => setCertFormPetID(pet.id)}>{pet.name}</View>
              ))}
            </View>

            <Text className="cal-field-label">证件名称</Text>
            <Input className="cal-input" maxlength={30} value={certFormName} placeholder="如：狂犬病免疫证明" onInput={event => setCertFormName(event.detail.value)} />

            <Text className="cal-field-label">时间</Text>
            <Picker mode="date" value={certFormDate} end={certFormDate || undefined} onChange={event => setCertFormDate(event.detail.value)}>
              <View className="cal-picker-row">
                <Text>{certFormDate || '选择日期'}</Text>
                <Text>选择</Text>
              </View>
            </Picker>

            <Text className="cal-field-label">详细说明</Text>
            <Textarea className="cal-textarea" value={certFormDetails} maxlength={500} placeholder="如疫苗批号、接种医院、下次补种时间等" onInput={event => setCertFormDetails(event.detail.value)} />

            <View className="cal-image-actions">
              <View className="cal-image-button" onClick={handleCertChooseImage}>{certFormUploading ? '上传中' : (certFormAssetID ? '更换图片' : '添加图片')}</View>
              {certFormAssetID && <Text>已选择 1 张</Text>}
            </View>

            <View className="cal-sheet-actions">
              <View className="cal-cancel-button" onClick={closeCertForm}>取消</View>
              <View className={`cal-save-button${certSubmitting ? ' disabled' : ''}`} onClick={handleCreateCertificate}>{certSubmitting ? '保存中' : '保存'}</View>
            </View>
          </View>
        </View>
      )}
      {/* 体重记录弹层 (健康资料页): 新增/修改共用 (复用 cal-* 弹层样式) */}
      {weightFormVisible && (
        <View className="cal-overlay" onClick={closeWeightForm}>
          <View className="cal-sheet" onClick={event => event.stopPropagation()}>
            <View className="cal-sheet-handle" />
            <Text className="cal-sheet-title">{weightFormEditingID ? '修改体重记录' : '记录体重'}</Text>

            <Text className="cal-field-label">宠物</Text>
            <View className="cal-pet-chips">
              {pets.map(pet => (
                <View
                  key={pet.id}
                  className={`cal-pet-chip${weightFormPetID === pet.id ? ' selected' : ''}${weightFormEditingID ? ' locked' : ''}`}
                  onClick={() => {
                    if (!weightFormEditingID) {
                      setWeightFormPetID(pet.id)
                    }
                  }}
                >
                  {pet.name}
                </View>
              ))}
            </View>

            <View className="cal-form-row">
              <View className="cal-form-col">
                <Text className="cal-field-label">体重（kg）</Text>
                <Input
                  className="cal-input"
                  type="digit"
                  value={weightFormValue}
                  maxlength={6}
                  placeholder="如 4.35"
                  onInput={event => setWeightFormValue(event.detail.value)}
                />
              </View>
              <View className="cal-form-col">
                <Text className="cal-field-label">测量日期</Text>
                <Picker mode="date" value={weightFormDate} onChange={event => setWeightFormDate(event.detail.value)}>
                  <View className="cal-picker-row">
                    <Text>{weightFormDate || '选择日期'}</Text>
                    <Text>选择</Text>
                  </View>
                </Picker>
              </View>
            </View>

            <View className="cal-sheet-actions">
              <View className="cal-cancel-button" onClick={closeWeightForm}>取消</View>
              <View className={`cal-save-button${weightSubmitting ? ' disabled' : ''}`} onClick={handleSaveWeight}>{weightSubmitting ? '保存中' : '保存'}</View>
            </View>
          </View>
        </View>
      )}
      <FloatingGuide />
    </View>
  )
}
