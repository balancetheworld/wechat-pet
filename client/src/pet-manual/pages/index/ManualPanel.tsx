import { useState, useEffect } from 'react'
import { Input, Textarea } from '@tarojs/components'
import { View, Text, Button, Html, ScrollView } from '../../components/compat'
import { CHAPTER_NAMES, CAL_PETS, PetRecord, CalRecord, CalTodo } from './data'
import * as PetAPI from '../../services/pet'
import {
  questionsToPersonality,
  traitsToTags,
  healthRecordToPetHealth,
  birthdayRecordsToBirthdays,
  weightItemsToWeights,
  growthItemsToEvents,
} from '../../services/mappers'

const TODAY = new Date(2026, 7, 14)
const parse = (s: string) => { const [y, m, d] = s.split('-').map(Number); return new Date(y, m - 1, d) }
const daysBetween = (a: Date, b: Date) => Math.round((b.getTime() - a.getTime()) / 86400000)
const cnDate = (s: string) => s.replace(/^(\d+)-0?(\d+)-0?(\d+)$/, '$1年$2月$3日')
const pad = (n: number) => (n < 10 ? '0' + n : '' + n)
const fmt = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
const GROWTH_TODAY = fmt(TODAY)

interface GrowthEvent { type: string; date: string; title: string; content: string; author: string }

interface Detail { title: string; subtitle: string; body: string }

interface Props {
  pet: PetRecord
  page: number
  onTurn: (p: number) => void
  onToast: (msg: string) => void
  onAddRecord: (r: CalRecord) => void
  onAddTodo: (t: CalTodo) => void
}

export default function ManualPanel({ pet, page: current, onTurn, onToast, onAddRecord, onAddTodo }: Props) {
  const [turning, setTurning] = useState<number | null>(null)
  const [zBoost, setZBoost] = useState(false)
  const [tocOpen, setTocOpen] = useState(false)
  const [detail, setDetail] = useState<Detail | null>(null)

  // 成长足迹：添加事件表单（与日历板块同一套表单/样式）
  const [gmOpen, setGmOpen] = useState(false)
  const [gmCat, setGmCat] = useState<'daily' | 'medical'>('daily')
  const [gmPet, setGmPet] = useState<string | null>(null)
  const [gmRemind, setGmRemind] = useState(false)
  const [gmCycle, setGmCycle] = useState<'once' | 'quarterly' | 'yearly'>('yearly')
  const [gmDate, setGmDate] = useState(GROWTH_TODAY)
  const [gmTime, setGmTime] = useState('09:00')
  const [gmText, setGmText] = useState('')
  const [gmEvents, setGmEvents] = useState<GrowthEvent[]>([])

  // API 加载的子资源（宠物切换时重新拉取）
  const [apiPersonality, setApiPersonality] = useState<PetRecord['personalityQuestions'] | null>(null)
  const [apiTags, setApiTags] = useState<string[] | null>(null)
  const [apiHealth, setApiHealth] = useState<PetRecord['health'] | null>(null)
  const [apiBirthdays, setApiBirthdays] = useState<PetRecord['birthdays'] | null>(null)
  const [apiWeights, setApiWeights] = useState<PetRecord['weights'] | null>(null)
  const [apiEvents, setApiEvents] = useState<PetRecord['events'] | null>(null)
  const [apiDiseases, setApiDiseases] = useState<{ name: string; status: string; details: string }[]>([])
  const [apiVaccines, setApiVaccines] = useState<{ name: string; vaccinated_at: string; details: string }[]>([])
  const [certCount, setCertCount] = useState<number | null>(null)
  // subLoading 值暂未被 UI 消费（加载中状态可后续接骨架屏），仅保留 setter 驱动状态
  const [, setSubLoading] = useState(false)

  // 宠物切换时加载子资源（对齐文件三后端泛化资源路由）
  useEffect(() => {
    let cancelled = false
    const loadSubResources = async () => {
      setSubLoading(true)
      // 重置为 null，组件层会用 pet 内嵌的 mock 数据兜底
      setApiPersonality(null); setApiTags(null); setApiHealth(null)
      setApiBirthdays(null); setApiWeights(null); setApiEvents(null)
      setApiDiseases([]); setApiVaccines([]); setCertCount(null)
      try {
        // 并行加载：问答/特质/健康/疾病/疫苗/生日/体重/事件/证件
        const [questions, traits, health, diseases, vaccines, birthdays, weights, events, certificates] = await Promise.allSettled([
          PetAPI.getPetQuestions(pet.id),
          PetAPI.getPetTraits(pet.id),
          PetAPI.getPetHealth(pet.id),
          PetAPI.getPetDiseases(pet.id),
          PetAPI.getPetVaccines(pet.id),
          PetAPI.getPetBirthdayRecords(pet.id),
          PetAPI.getPetWeights(pet.id),
          PetAPI.getPetGrowthEvents(pet.id),
          PetAPI.getPetCertificates(pet.id),
        ])
        if (cancelled) return
        if (questions.status === 'fulfilled') setApiPersonality(questionsToPersonality(questions.value))
        if (traits.status === 'fulfilled') {
          const tags = traitsToTags(traits.value)
          setApiTags(tags.length ? tags : null)
        }
        if (health.status === 'fulfilled') {
          const h = health.value
          const ds = diseases.status === 'fulfilled' ? diseases.value : []
          const vs = vaccines.status === 'fulfilled' ? vaccines.value : []
          setApiHealth(healthRecordToPetHealth(h, ds, vs))
          setApiDiseases(ds)
          setApiVaccines(vs)
        }
        if (birthdays.status === 'fulfilled') setApiBirthdays(birthdayRecordsToBirthdays(birthdays.value))
        if (weights.status === 'fulfilled') setApiWeights(weightItemsToWeights(weights.value))
        if (events.status === 'fulfilled') setApiEvents(growthItemsToEvents(events.value))
        if (certificates.status === 'fulfilled') setCertCount(certificates.value.length)
      } catch {
        // API 异常时静默回退到 pet 内嵌数据
      } finally {
        if (!cancelled) setSubLoading(false)
      }
    }
    // 跳过 mock 宠物 ID（milo/doubao），直接用内嵌数据
    if (pet.id && !pet.id.startsWith('milo') && !pet.id.startsWith('doubao') && !pet.id.startsWith('pet_')) {
      loadSubResources()
    }
    return () => { cancelled = true }
  }, [pet.id])

  const total = CHAPTER_NAMES.length

  // 身份页统计
  const birth = parse(pet.birthDate)
  const arrival = parse(pet.arrivalDate)
  const ageYears = Math.floor(daysBetween(birth, TODAY) / 365.25)
  const companionDays = daysBetween(arrival, TODAY)
  let nextBd = new Date(TODAY.getFullYear(), birth.getMonth(), birth.getDate())
  if (nextBd.getTime() < TODAY.getTime()) nextBd = new Date(TODAY.getFullYear() + 1, birth.getMonth(), birth.getDate())
  const nextBdDays = daysBetween(TODAY, nextBd)

  // 体重（优先用 API 数据，回退到 pet 内嵌数据）
  const weights = apiWeights ?? pet.weights
  const wLatest = weights[0]
  const wPrev = weights[1]
  const wDiffNum = wLatest && wPrev ? parseFloat(wLatest.value) - parseFloat(wPrev.value) : 0

  const goToPage = (index: number) => {
    const next = Math.max(0, Math.min(total - 1, index))
    if (next === current) return
    setTurning(next > current ? current : next)
    setZBoost(true)
    onTurn(next)
    setTimeout(() => { setTurning(null); setZBoost(false) }, 540)
  }

  const pageZ = (i: number) => (turning !== null && i === turning && zBoost ? total + 2 : total - i)
  const pageClass = (i: number) =>
    `page${i === 0 ? ' cover' : ''}${i === total - 1 ? ' back-cover' : ' content-page'}${i < current ? ' flipped' : ''}${turning === i ? ' turning' : ''}`

  const openDetail = (title: string, subtitle: string, body: string) => setDetail({ title, subtitle, body })

  // 成长足迹：打开添加事件表单（默认选中当前宠物对应的日历宠物）
  const petIdToCal = (id: string) => id === 'milo' ? 'mi' : id === 'doubao' ? 'dou' : null
  const openGm = () => {
    setGmCat('daily'); setGmPet(petIdToCal(pet.id)); setGmRemind(false); setGmCycle('yearly')
    setGmDate(GROWTH_TODAY); setGmTime('09:00'); setGmText('')
    setGmOpen(true)
  }

  const gmNextRemind = () => {
    const nd = parse(gmDate || GROWTH_TODAY)
    if (gmCycle === 'yearly') nd.setFullYear(nd.getFullYear() + 1)
    else if (gmCycle === 'quarterly') nd.setMonth(nd.getMonth() + 3)
    return fmt(nd)
  }

  const saveGm = () => {
    const text = gmText.trim()
    if (!gmPet || !text) return
    const isMed = gmCat === 'medical'
    const date = (gmDate.trim() || GROWTH_TODAY)
    const title = text.length > 14 ? text.slice(0, 14) + '…' : text
    // 真实宠物：同步保存到后端成长事件（失败静默，保留本地展示）
    const isRealPet = pet.id && !pet.id.startsWith('milo') && !pet.id.startsWith('doubao') && !pet.id.startsWith('pet_')
    if (isRealPet) {
      PetAPI.createGrowthEvent(pet.id, {
        type: isMed ? '健康记录' : '日常趣事',
        occurred_at: date,
        recorder: '我',
        content: text,
      }).catch(() => { /* 后端保存失败不影响本地体验 */ })
    }
    // 同步到日历板块（共享 records）
    onAddRecord({
      date, pet: gmPet, cat: gmCat, medType: null,
      title, text, imgs: isMed ? ['🩺'] : ['📷'], by: '我', time: gmTime || '09:00',
    })
    if (isMed && gmRemind) {
      const cycleName = gmCycle === 'yearly' ? '每年' : gmCycle === 'quarterly' ? '每3个月' : '一次性'
      onAddTodo({ id: 'g' + Date.now(), pet: gmPet, title: '下次医疗提醒', remindDate: gmNextRemind(), cycle: gmCycle, cycleName, status: 'pending', by: '我' })
    }
    // 同时在成长足迹时间线里展示
    setGmEvents(prev => [...prev, { type: isMed ? '健康记录' : '日常趣事', date, title, content: text, author: '我' }])
    setGmOpen(false)
    onToast('事件已添加，并同步到日历')
  }
  const gmSaveOk = !!gmPet && gmText.trim().length > 0

  const healthData = apiHealth ?? pet.health
  // 详情弹层文案：优先用后端真实数据（diseases/vaccines 资源），无数据时保留占位说明
  const diseaseDetail = apiDiseases.length
    ? `既往疾病|${apiDiseases.length} 条记录|${apiDiseases.map(d => `${d.name}（${d.status}）${d.details ? '：' + d.details : ''}`).join('；')}。`
    : '既往疾病|暂无记录|还没有疾病记录，发现异常时及时记录并就医。'
  const vaccineDetail = apiVaccines.length
    ? `最近疫苗|${apiVaccines[0].name}|${apiVaccines.map(v => `${v.name} ${v.vaccinated_at}`).join('；')}。凭证仅家庭成员可见。`
    : '最近疫苗|暂无记录|还没有疫苗接种记录，接种后记得补充登记。'
  const allergyDetail = healthData.allergies.value !== '未记录'
    ? `过敏信息|${healthData.allergies.value}|以上过敏信息来自家庭健康档案，请注意回避相关食物与环境。`
    : '过敏信息|暂未发现|还没有过敏记录，如有发现请在健康档案中补充。'
  const medicationDetail = healthData.medications.value !== '无' && healthData.medications.value !== '未记录'
    ? `长期用药|${healthData.medications.value}|用药信息来自家庭健康档案。本模块只保存档案，不提供药物剂量建议。`
    : '长期用药|目前无用药|本模块只保存档案，不提供药物剂量建议。'
  const healthItems = [
    { ...healthData.allergies, d: allergyDetail },
    { ...healthData.diseases, d: diseaseDetail },
    { ...healthData.medications, d: medicationDetail },
    { ...healthData.vaccines, d: vaccineDetail },
  ]

  const personalityData = apiPersonality ?? pet.personalityQuestions
  const tagData = apiTags ?? pet.tags
  const birthdays = [...(apiBirthdays ?? pet.birthdays)].sort((a, b) => b.date.localeCompare(a.date))
  const petEvents = apiEvents ?? pet.events

  return (
    <>
      <View className='book' id='book'>
        <View className='book-bg' />
        <View className='pages' id='pages'>
          {/* 封面 */}
          <View className={pageClass(0)} style={{ zIndex: pageZ(0) }} data-chapter='封面'>
            <View className='cover-copy'>
              <Text className='pet-name h1'>{pet.name}</Text>
              <Text id='coverTitle' className='span' style={{ display: 'none' }}>{pet.title}</Text>
              <Text id='coverQuote' className='span' style={{ display: 'none' }}>{pet.quote}</Text>
              <Text id='coverYears' className='span' style={{ display: 'none' }}>{pet.years}</Text>
            </View>
          </View>

          {/* 身份名片 */}
          <View className={pageClass(1)} style={{ zIndex: pageZ(1) }} data-chapter='身份名片'>
            <View className='page-scroll'>
              <View className='page-head'>
                <Text className='page-eyebrow span'>PROFILE</Text>
                <Text className='page-title h2'>身份名片</Text>
                <Text className='page-subtitle p'>它的基本档案，一页看全</Text>
              </View>
              <View className='identity-hero'>
                <Button className='identity-photo photo-placeholder' onClick={() => onToast('头像占位')}>头像</Button>
                <View>
                  <Text className='identity-name pet-name h3'>{pet.name}</Text>
                  <Html className='identity-type p' html={pet.type} />
                </View>
              </View>
              <View className='stat-row'>
                <View className='stat'><Text className='strong'>{ageYears}岁</Text><Text className='span'>当前年龄</Text></View>
                <View className='stat'><Text className='strong'>{companionDays.toLocaleString()}</Text><Text className='span'>陪伴天数</Text></View>
                <View className='stat'><Text className='strong'>{nextBdDays}天</Text><Text className='span'>下次生日</Text></View>
              </View>
              <View className='info-list'>
                <View className='info-row info-static'><Text className='span'>出生信息</Text><Text className='strong'>{pet.birthDate ? cnDate(pet.birthDate) : '未记录'}</Text></View>
                <View className='info-row info-static'><Text className='span'>到家日期</Text><Text className='strong'>{cnDate(pet.arrivalDate)}</Text></View>
                <Button className='info-row' onClick={() => onToast(certCount == null
                  ? '身份与证件：加载中…'
                  : certCount === 0 ? '身份与证件：还未收录' : `身份与证件：已收纳 ${certCount} 项`)}>
                  <Text className='span'>身份与证件</Text>
                  <Text className='strong'>{certCount == null ? '—' : certCount === 0 ? '还未收录' : `已收纳 ${certCount} 项`}</Text><Text className='i'>›</Text>
                </Button>
              </View>
            </View>
          </View>

          {/* 个性说明书 */}
          <View className={pageClass(2)} style={{ zIndex: pageZ(2) }} data-chapter='个性说明书'>
            <View className='page-scroll'>
              <View className='page-head'>
                <Text className='page-eyebrow span'>PERSONALITY</Text>
                <Text className='page-title h2'>个性说明书</Text>
                <Text className='page-subtitle p'>性格标签，加上一份读懂它的说明书</Text>
              </View>
              <View className='tags' id='personalityTagsView'>
                {tagData.map(t => <Text className='tag' key={t}>{t}</Text>)}
              </View>
              <View className='manual-list' id='personalityQuestionsView'>
                {personalityData.length === 0 && (
                  <View className='manual-item' style={{ opacity: 0.6, pointerEvents: 'none' }}>
                    <Text className='manual-index span'>—</Text>
                    <View><Text className='h3'>正在补充中</Text><Text className='p'>相处一段时间后，这里会记录它的性格特点。</Text></View>
                  </View>
                )}
                {personalityData.map((q, i) => (
                  <Button className='manual-item detail-trigger' key={i}
                    onClick={() => openDetail(q.title, q.summary, q.detail)}>
                    <Text className='manual-index span'>{String(i + 1).padStart(2, '0')}</Text>
                    <View><Text className='h3'>{q.title}</Text><Text className='p'>{q.summary}</Text></View>
                    <Text className='i'>›</Text>
                  </Button>
                ))}
              </View>
            </View>
          </View>

          {/* 健康资料 */}
          <View className={pageClass(3)} style={{ zIndex: pageZ(3) }} data-chapter='健康资料'>
            <View className='page-scroll'>
              <View className='page-head'>
                <Text className='page-eyebrow span'>HEALTH</Text>
                <Text className='page-title h2'>健康资料</Text>
                <Text className='page-subtitle p'>健康档案仅对家庭成员可见</Text>
              </View>
              <View className='health-lead'>
                <View><Text className='span'>整体状态</Text><Text className='strong'>{healthData.overall}</Text></View>
                <Text className='span'>仅家庭可见</Text>
              </View>
              <View className='health-grid'>
                {healthItems.map((h, i) => {
                  const [t, s, b] = h.d.split('|')
                  return (
                    <Button className='health-item detail-trigger' key={i} onClick={() => openDetail(t, s, b)}>
                      <Text className='health-label span'>{h.label}</Text>
                      <Text className='health-value strong'>{h.value}</Text>
                      <Text className='health-note span'>{h.note}</Text>
                    </Button>
                  )
                })}
              </View>
              <Text className='updated p'>{pet.updated}</Text>
            </View>
          </View>

          {/* 生日纪念册 */}
          <View className={pageClass(4)} style={{ zIndex: pageZ(4) }} data-chapter='生日纪念册'>
            <View className='page-scroll'>
              <View className='page-head'>
                <Text className='page-eyebrow span'>BIRTHDAYS</Text>
                <Text className='page-title h2'>生日纪念册</Text>
                <Text className='page-subtitle p'>一起数过的每一岁，都值得好好收藏。</Text>
              </View>
              <View id='birthdayRecordsView'>
                {birthdays.length === 0 && (
                  <View className='birthday-feature' style={{ opacity: 0.6, textAlign: 'center' }}>
                    <View className='birthday-copy'>
                      <Text className='span'>还没有生日记录</Text>
                      <Text className='h3'>期待第一个生日</Text>
                      <Text className='p'>每年的生日都会记录在这里。</Text>
                    </View>
                  </View>
                )}
                {birthdays.map((r, i) => {
                  const year = r.date.slice(0, 4)
                  const open = () => openDetail(`${r.age}生日`, r.title, `${r.wish} ${r.mediaLabel}。`)
                  if (i === 0) {
                    return (
                      <View className='birthday-feature' key={r.date} onClick={open}>
                        <View className='media-placeholder'>{r.mediaLabel}</View>
                        <View className='birthday-copy'>
                          <Text className='span'>{year} · {r.age}</Text>
                          <Text className='h3'>{r.title}</Text>
                          <Text className='p'>“{r.wish}”</Text>
                        </View>
                      </View>
                    )
                  }
                  return (
                    <View className='year-row' key={r.date} onClick={open}>
                      <Text className='strong'>{year}</Text>
                      <Text className='span'>{r.age} · {r.title}　›</Text>
                    </View>
                  )
                })}
              </View>
            </View>
          </View>

          {/* 成长足迹 */}
          <View className={pageClass(5)} style={{ zIndex: pageZ(5) }} data-chapter='成长足迹'>
            <View className='page-scroll'>
              <View className='page-head'>
                <Text className='page-eyebrow span'>GROWTH</Text>
                <Text className='page-title h2'>成长足迹</Text>
                <Text className='page-subtitle p'>把日子里的小事，慢慢连成它的一生。</Text>
              </View>
              <Text className='growth-section-title h3'>体重记录</Text>
              <View className='weight-card detail-trigger growth-weight-record' id='growthWeightCard'
                onClick={() => wLatest
                  ? openDetail('体重记录', `当前 ${wLatest.value} kg，比上次${wDiffNum < 0 ? '减少' : '增加'} ${Math.abs(wDiffNum).toFixed(1)} kg`, `最近记录：${weights.map(w => `${cnDate(w.date)} ${w.value} kg`).join('；')}。`)
                  : openDetail('体重记录', '暂无记录', '还没有体重记录，添加后这里会展示体重变化趋势。')
                }>
                {wLatest ? (
                  <>
                    <View className='weight-head'>
                      <View><Text className='span'>当前体重</Text><View className='weight-value'>{wLatest.value} kg</View></View>
                      <View className='weight-change'>较上次 {wDiffNum < 0 ? '−' : '+'}{Math.abs(wDiffNum).toFixed(1)}</View>
                    </View>
                    <View className='chart'><View className='chart-line' /></View>
                  </>
                ) : (
                  <View className='weight-head'>
                    <View><Text className='span'>当前体重</Text><View className='weight-value' style={{ fontSize: '14px', color: '#999' }}>暂无记录</View></View>
                  </View>
                )}
              </View>
              <View className='growth-events-head'>
                <Text className='growth-section-title h3'>事件记录</Text>
              </View>
              {/* 时间线用 ScrollView：小程序端原生 scroll-view 保证可滑动，H5 端还原为普通 div（滚动样式由 #growthTimeline 规则驱动） */}
              <ScrollView className='timeline' id='growthTimeline'>
                {(() => {
                  // 本地新增（gmEvents）+ 后端拉取（petEvents）合并后按「日期+标题+内容」去重
                  // （保存成功的事件会写入后端，宠物切换重新拉取时避免与本地副本重复）
                  const seen = new Set<string>()
                  const allEvents = [...gmEvents, ...petEvents]
                    .sort((a, b) => b.date.localeCompare(a.date))
                    .filter(ev => {
                      const key = `${ev.date}|${ev.title}|${ev.content}`
                      if (seen.has(key)) return false
                      seen.add(key)
                      return true
                    })
                  if (allEvents.length === 0) {
                    return <View className='moment' style={{ textAlign: 'center', opacity: 0.6 }}><Text className='p'>还没有事件记录，点击右下角「＋」记录点什么吧</Text></View>
                  }
                  return allEvents.map(ev => (
                    <View className='moment' key={ev.date + ev.title}
                      onClick={() => openDetail(ev.type, ev.title, `${cnDate(ev.date)}。${ev.content} 记录人：${ev.author}。`)}>
                      <Text className='time'>{ev.date} · {ev.author}</Text>
                      <Text className='h3'>{ev.title}</Text>
                      <Text className='p'>{ev.content}</Text>
                    </View>
                  ))
                })()}
              </ScrollView>
            </View>
          </View>

          {/* 封底 */}
          <View className={pageClass(6)} style={{ zIndex: pageZ(6) }} data-chapter='封底'>
            <View>
              <View className='back-mark'>M</View>
              <Text className='h2'><Text className='pet-name span'>{pet.name}</Text>，下页见</Text>
              <Text className='p'>新的故事会继续发生，而家会一直把它们好好收着。</Text>
            </View>
            <View className='back-family'>{pet.backFamily}</View>
          </View>
        </View>
        <Button className='edge edge-left' disabled={current === 0} onClick={() => goToPage(current - 1)} />
        <Button className='edge edge-right' disabled={current === total - 1} onClick={() => goToPage(current + 1)} />
      </View>

      {/* 成长足迹：添加事件按钮（放档案书外，与宠物日历的悬浮添加按钮同款） */}
      {current === 5 && (
        <View className='cal-fab growth-fab' id='growthFab'>
          <View className='cal-fab-btn' id='growthAddBtn' onClick={openGm}>＋</View>
        </View>
      )}

      {/* 页码状态 / 打开目录 */}
      <Button className='page-status' onClick={() => setTocOpen(true)}>
        <Text id='chapterName' className='span'>{CHAPTER_NAMES[current]}</Text>
        <Text className='page-dot span' />
        <Text id='pageNumber' className='span'>{current + 1} / {total}</Text>
      </Button>

      {/* 目录弹层 */}
      <View className={`overlay${tocOpen ? ' open' : ''}`} id='tocOverlay' onClick={() => setTocOpen(false)}>
        <View className='sheet' onClick={e => e.stopPropagation()}>
          <View className='handle' />
          <View className='sheet-head'>
            <View>
              <Text className='h2'>目录</Text>
              <Text id='tocSubtitle' className='p'>{pet.title} · 共 {total} 页</Text>
            </View>
            <Button className='close-button' onClick={() => setTocOpen(false)}>×</Button>
          </View>
          <View className='chapter-list' id='chapterList'>
            {CHAPTER_NAMES.map((name, i) => (
              <Button className={`chapter-item${i === current ? ' active' : ''}`} key={i}
                onClick={() => { goToPage(i); setTocOpen(false) }}>
                <Text className='chapter-number span'>{String(i + 1).padStart(2, '0')}</Text>
                <Text className='strong'>{name}</Text>
                <Text className='span'>{i + 1} / {total}</Text>
              </Button>
            ))}
          </View>
        </View>
      </View>

      {/* 详情弹层（居中卡片） */}
      <View className={`overlay${detail ? ' open' : ''}`} id='detailOverlay' onClick={() => setDetail(null)}>
        <View className='detail-card' onClick={e => e.stopPropagation()}>
          <View className='detail-card-head'>
            <View>
              <Text id='detailTitle' className='h2'>{detail?.title || '详情'}</Text>
              <Text id='detailSubtitle' className='p'>{detail?.subtitle || ''}</Text>
            </View>
            <Button className='close-button' onClick={() => setDetail(null)}>×</Button>
          </View>
          <View className='detail-block'><Text className='span'>记录内容</Text><Text id='detailBody' className='p'>{detail?.body || ''}</Text></View>
        </View>
      </View>

      {/* 成长足迹：添加事件（表单与日历板块一致，保存后同步到日历） */}
      <View className={`cal-overlay gm-overlay${gmOpen ? ' show' : ''}`} id='gmOverlay' onClick={() => setGmOpen(false)}>
        <View className='cal-sheet' id='gmSheet' onClick={(e) => e.stopPropagation()}>
          <View className='cal-sheet-handle' />
          <View className='cal-sheet-title'>添加事件</View>
          <View className='cal-field-label'>分类</View>
          <View className='cal-seg'>
            <View className={`cal-seg-item daily-s${gmCat === 'daily' ? ' active' : ''}`} onClick={() => setGmCat('daily')}><Text className='seg-dot' />日常</View>
            <View className={`cal-seg-item med-s${gmCat === 'medical' ? ' active' : ''}`} onClick={() => setGmCat('medical')}><Text className='seg-dot' />医疗</View>
          </View>
          <View className='cal-field-label'>宠物</View>
          <View className='cal-pet-chips' id='gmPetChips'>
            {CAL_PETS.map(p => (
              <Text key={p.id} className={`cal-pet-chip${gmPet === p.id ? ' selected' : ''}`} onClick={() => setGmPet(p.id)}>{p.emoji} {p.name}</Text>
            ))}
          </View>
          <View className='cal-field-label'>日期</View>
          <Input type='text' className='cal-textarea' id='gmDateInput' style={{ minHeight: '44px', padding: '11px 14px' }}
            placeholder='如 2026-08-14' value={gmDate} onInput={(e) => setGmDate(e.detail.value)} />
          <View className='cal-field-label'>时间</View>
          <Input type='text' className='cal-textarea' id='gmTimeInput' style={{ minHeight: '44px', padding: '11px 14px' }} value={gmTime} onInput={(e) => setGmTime(e.detail.value)} />
          <View className='cal-field-label'>记录内容</View>
          <Textarea
            className='cal-textarea' id='gmText' style={{ minHeight: '88px' }}
            placeholder={gmCat === 'medical' ? '记录做了什么医疗行为，如：今天接种狂犬疫苗第二针…' : '记录日常点滴，如：今天终于学会握手啦！'}
            value={gmText} onInput={(e) => setGmText(e.detail.value)}
          />
          {gmCat === 'medical' && (
            <View id='gmRemindBox'>
              <View className='cal-switch-row'>
                <View className='sr-label'>⏰ 添加待办提醒（仅医疗可设）</View>
                <View className={`cal-switch${gmRemind ? ' on' : ''}`} id='gmRemindSwitch' onClick={() => setGmRemind(!gmRemind)} />
              </View>
              {gmRemind && (
                <View className='cal-remind-panel' id='gmRemindPanel'>
                  <View className='cal-remind-row'><Text>提醒日期</Text><Text className='rr-val' id='gmRemindDateVal'>{gmNextRemind()}</Text></View>
                  <View className='cal-remind-row'><Text>重复周期</Text></View>
                  <View className='cal-cycle-chips' id='gmCycleChips'>
                    <Text className={`cal-cycle-chip${gmCycle === 'once' ? ' selected' : ''}`} onClick={() => setGmCycle('once')}>一次性</Text>
                    <Text className={`cal-cycle-chip${gmCycle === 'quarterly' ? ' selected' : ''}`} onClick={() => setGmCycle('quarterly')}>每3个月</Text>
                    <Text className={`cal-cycle-chip${gmCycle === 'yearly' ? ' selected' : ''}`} onClick={() => setGmCycle('yearly')}>每年</Text>
                  </View>
                </View>
              )}
            </View>
          )}
          <View className={`cal-submit-btn${gmSaveOk ? '' : ' disabled'}`} id='gmSubmitBtn' onClick={saveGm}>保存事件</View>
        </View>
      </View>
    </>
  )
}
