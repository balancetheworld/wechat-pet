import { useState, type Dispatch, type SetStateAction } from 'react'
import { View, Text, Input, Textarea, Image } from '@tarojs/components'
import { CAL_PETS, CAL_MEMBERS, CalRecord, CalTodo } from './data'
import pawHandlePng from '../../assets/cal-paw-handle.png'
import pawHandleRoundPng from '../../assets/cal-paw-handle-round.png'

const pad = (n: number) => (n < 10 ? '0' + n : '' + n)
const fmt = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
const parse = (s: string) => { const [y, m, d] = s.split('-').map(Number); return new Date(y, m - 1, d) }
const MONTH_EN = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
const MONTH_CN = ['一月', '二月', '三月', '四月', '五月', '六月', '七月', '八月', '九月', '十月', '十一月', '十二月']
const WEEK_CN = ['日', '一', '二', '三', '四', '五', '六']

const CAL_TODAY = new Date(2026, 7, 14)

interface Props {
  onToast: (msg: string) => void
  records: CalRecord[]
  todos: CalTodo[]
  onRecordsChange: Dispatch<SetStateAction<CalRecord[]>>
  onTodosChange: Dispatch<SetStateAction<CalTodo[]>>
}

export default function CalPanel({ onToast, records, todos, onRecordsChange: setRecords, onTodosChange: setTodos }: Props) {
  const [calOpen, setCalOpen] = useState(false)
  const [bmBounce, setBmBounce] = useState(false)
  const [calTurn, setCalTurn] = useState<'prev' | 'next' | null>(null)
  const [curDate, setCurDate] = useState('2026-08-14')
  const [viewYM, setViewYM] = useState({ y: 2026, m: 7 })
  const [filterPet, setFilterPet] = useState<string | null>(null)
  const [overlayOpen, setOverlayOpen] = useState(false)
  const [addCat, setAddCat] = useState<'daily' | 'medical'>('daily')
  const [addPet, setAddPet] = useState<string | null>(null)
  const [addRemind, setAddRemind] = useState(false)
  const [addCycle, setAddCycle] = useState<'once' | 'quarterly' | 'yearly'>('yearly')
  const [addTime, setAddTime] = useState('09:00')
  const [addText, setAddText] = useState('')
  const [doneTodo, setDoneTodo] = useState<CalTodo | null>(null)

  // 猫爪书签贴着日历下边沿一起移动：纯 CSS 实现（.cal-dropdown .cal-bookmark { top: max(92px, 100%) }），
  // 日历收起时高度为 0，猫爪停在 92px（避开顶部状态栏）；拉出后 top = 100% 实时贴住下边沿（含回弹）

  const getPet = (id: string) => CAL_PETS.find(p => p.id === id)!
  const getBy = (by: string) => CAL_MEMBERS[by] || { name: by }
  const recordsOf = (date: string) => records.filter(r => r.date === date)
  const todosOf = (date: string) => todos.filter(t => t.status === 'pending' && t.remindDate === date)
  const medOn = (date: string, pf?: string | null) => records.some(r => r.date === date && r.cat === 'medical' && (!pf || r.pet === pf))
  const dailyOn = (date: string, pf?: string | null) => records.some(r => r.date === date && r.cat === 'daily' && (!pf || r.pet === pf))
  const todoOn = (date: string, pf?: string | null) => todos.some(t => t.status === 'pending' && t.remindDate === date && (!pf || t.pet === pf))

  const d = parse(curDate)
  const isToday = curDate === fmt(CAL_TODAY)
  const recs = recordsOf(curDate)
  const tds = todosOf(curDate)

  const nextRemindDate = () => {
    const nd = parse(curDate)
    if (addCycle === 'yearly') nd.setFullYear(nd.getFullYear() + 1)
    else if (addCycle === 'quarterly') nd.setMonth(nd.getMonth() + 3)
    return fmt(nd)
  }

  const cycleFilter = () => {
    const ids: (string | null)[] = [null, ...CAL_PETS.map(p => p.id)]
    const idx = ids.indexOf(filterPet)
    setFilterPet(ids[(idx + 1) % ids.length])
  }

  // 日历下拉按钮（猫爪书签）回弹动画：先清 class 再下一帧重加，保证每次点击都重放
  const toggleCal = () => {
    setCalOpen(v => !v)
    setBmBounce(false)
    requestAnimationFrame(() => setBmBounce(true))
  }

  // 月份切换改为“翻页”手势：点击标题左侧翻上一个月，右侧翻下一个月
  const turnMonth = (dir: 'prev' | 'next') => {
    if (calTurn) return
    setCalTurn(dir)
    setTimeout(() => {
      setViewYM(v => {
        if (dir === 'prev') {
          const m = v.m - 1
          return m < 0 ? { y: v.y - 1, m: 11 } : { y: v.y, m }
        }
        const m = v.m + 1
        return m > 11 ? { y: v.y + 1, m: 0 } : { y: v.y, m }
      })
    }, 260)
    setTimeout(() => setCalTurn(null), 520)
  }

  const openAdd = () => {
    setAddCat('daily'); setAddPet(null); setAddRemind(false); setAddCycle('yearly')
    setAddText(''); setAddTime('09:00')
    setOverlayOpen(true)
  }

  const saveRecord = () => {
    const text = addText.trim()
    if (!addPet || !text) return
    const isMed = addCat === 'medical'
    setRecords(prev => [...prev, {
      date: curDate, pet: addPet, cat: addCat, medType: null,
      title: text.length > 14 ? text.slice(0, 14) + '…' : text,
      text, imgs: isMed ? ['🩺'] : ['📷'], by: 'ma', time: addTime || '09:00',
    }])
    if (isMed && addRemind) {
      const nd = parse(curDate)
      if (addCycle === 'yearly') nd.setFullYear(nd.getFullYear() + 1)
      else if (addCycle === 'quarterly') nd.setMonth(nd.getMonth() + 3)
      const cycleName = addCycle === 'yearly' ? '每年' : addCycle === 'quarterly' ? '每3个月' : '一次性'
      setTodos(prev => [...prev, { id: 't' + Date.now(), pet: addPet, title: '下次医疗提醒', remindDate: fmt(nd), cycle: addCycle, cycleName, status: 'pending', by: 'ma' }])
    }
    setOverlayOpen(false)
    onToast('记录已保存')
  }

  const confirmDone = () => {
    if (!doneTodo) return
    const t = doneTodo
    setTodos(prev => prev.map(x => x.id === t.id ? { ...x, status: 'completed' as const } : x))
    setRecords(prev => [...prev, {
      date: fmt(CAL_TODAY), pet: t.pet, cat: 'medical', medType: t.title,
      title: t.title, text: '通过待办提醒完成，已自动记录。', imgs: ['✅'], by: 'ma',
      time: '20:00',
    }])
    if (t.cycle !== 'once') {
      const nd = new Date(CAL_TODAY)
      if (t.cycle === 'yearly') nd.setFullYear(nd.getFullYear() + 1)
      else if (t.cycle === 'quarterly') nd.setMonth(nd.getMonth() + 3)
      setTodos(prev => [...prev, { ...t, id: 't' + Date.now(), remindDate: fmt(nd), status: 'pending' as const }])
    }
    setCurDate(fmt(CAL_TODAY))
    setDoneTodo(null)
    onToast('待办已完成')
  }

  // 月历 42 格
  const first = new Date(viewYM.y, viewYM.m, 1)
  const start = new Date(viewYM.y, viewYM.m, 1 - first.getDay())
  const cells: { ds: string; day: number; isCur: boolean }[] = []
  for (let i = 0; i < 42; i++) {
    const day = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i)
    cells.push({ ds: fmt(day), day: day.getDate(), isCur: day.getMonth() === viewYM.m })
  }

  const monthCnSpaced = MONTH_CN[viewYM.m].replace(/./g, c => c + ' ')
  const filterName = filterPet ? getPet(filterPet).name : '全部宠物'
  const saveOk = !!addPet && addText.trim().length > 0

  return (
    <View className='cal-module-content' id='calModuleContent'>
      <View className={`cal-dropdown${calOpen ? ' open' : ''}`} id='calDropdown'>
        <View className={`cal-bookmark${calOpen ? ' open' : ''}${bmBounce ? ' bounce' : ''}`} id='calBookmark'
          onClick={(e) => { e.stopPropagation(); toggleCal() }}
          onAnimationEnd={() => setBmBounce(false)}>
          {/* 收起态用圆顶尾部猫爪，展开态换成直边尾部猫爪 */}
          <Image className='cal-bookmark-img' src={calOpen ? pawHandlePng : pawHandleRoundPng} mode='aspectFit' />
        </View>

        <View className='cal-dd-clip'>
          <View className='cal-dd-content'>
          <View className='cal-dd-title'>
            <View className='cal-dd-month-en' id='calMonthEn'>{MONTH_EN[viewYM.m]}</View>
            <View className='cal-dd-month-cn' id='calMonthCn'>{monthCnSpaced}</View>
          </View>
          <View className='cal-dd-head'>
            <View className='cal-dd-filter' id='calFilterBtn' onClick={(e) => { e.stopPropagation(); cycleFilter() }}>🐾 {filterName} ▾</View>
            <Text className='cal-dd-ym' id='calYM'>{viewYM.y} · {viewYM.m + 1}</Text>
          </View>
          <View className={`cal-dd-sheet${calTurn ? ' turn-' + calTurn : ''}`}>
            <View className='cal-dd-weekrow'>
              <View className='cal-dd-week'>S</View><View className='cal-dd-week'>M</View><View className='cal-dd-week'>T</View>
              <View className='cal-dd-week'>W</View><View className='cal-dd-week'>T</View><View className='cal-dd-week'>F</View><View className='cal-dd-week'>S</View>
            </View>
            <View className='cal-dd-grid' id='calGrid'>
              {cells.map((c, i) => (
                <View key={i}
                  className={`cal-dd-cell${c.isCur ? '' : ' other-month'}${c.ds === curDate ? ' selected' : ''}${c.isCur && todoOn(c.ds, filterPet) ? ' alarm' : ''}`}
                  onClick={() => { setCurDate(c.ds); setCalOpen(false) }}
                >
                  <View className='cdd-day'>{c.day}</View>
                  {c.isCur && medOn(c.ds, filterPet) && <Text className='cdd-dot med' />}
                  {c.isCur && dailyOn(c.ds, filterPet) && <Text className='cdd-dot daily' />}
                </View>
              ))}
            </View>
          </View>
        </View>
        </View>
        {/* 箭头由 CSS ::before 绘制（proto.scss .cal-turn-hit），保证在热区内绝对居中 */}
        <View className='cal-turn-hit prev' onClick={(e) => { e.stopPropagation(); turnMonth('prev') }} />
        <View className='cal-turn-hit next' onClick={(e) => { e.stopPropagation(); turnMonth('next') }} />
      </View>

      <View className='cal-day-scroll' id='calDayScroll'>
        <View className='cal-day-card'>
          <View className='cal-day-header'>
            <View className='d-date' id='calDayDate'>
              {d.getMonth() + 1}月{d.getDate()}日 星期{WEEK_CN[d.getDay()]}
              {isToday && <Text className='d-today-tag'>今天</Text>}
            </View>
            <View className='d-summary' id='calDaySummary'>
              {recs.length}条记录{tds.length ? ` · ${tds.length}条待办提醒` : ''}
            </View>
          </View>
          <View className='cal-day-body' id='calDayBody'>
            {recs.length === 0 && tds.length === 0 ? (
              <View className='cal-day-empty'>
                <View className='e-emoji'>🍃</View>
                <View className='e-title'>这一天还没有记录</View>
                <View className='e-sub'>点击右下角「＋」记录点什么吧</View>
              </View>
            ) : (
              <>
                {tds.map(t => (
                  <View key={t.id} className='cal-rec-row todo'>
                    <View className='cal-rec-main'>
                      <View className='cal-rec-head'>
                        <Text className='cal-rec-cat todo'>待办</Text>
                        <Text className='cal-rec-pet'>{getPet(t.pet).name}</Text>
                        <Text className='cal-rec-time'>⏰ {t.remindDate.slice(5)}</Text>
                      </View>
                      <View className='cal-rec-text'>{t.title}</View>
                      <View className='td-sub'>提醒日 {t.remindDate.slice(5)} · {t.cycleName}一次</View>
                      <View className='cal-rec-by'>{getBy(t.by).name} 记录</View>
                    </View>
                    <View className='cal-btn-done' onClick={() => setDoneTodo(t)}>✓ 完成</View>
                  </View>
                ))}
                {[...recs].sort((a, b) => b.time.localeCompare(a.time)).map((r, i) => (
                  <View key={i} className={`cal-rec-row ${r.cat}`}>
                    <View className='cal-rec-main'>
                      <View className='cal-rec-head'>
                        <Text className={`cal-rec-cat ${r.cat}`}>{r.cat === 'medical' ? '医疗' : '日常'}</Text>
                        <Text className='cal-rec-pet'>{getPet(r.pet).name}</Text>
                        <Text className='cal-rec-time'>{r.time}</Text>
                      </View>
                      {r.imgs ? (
                        <View className={`cal-rec-imgs count-${Math.min(r.imgs.length, 4)}${r.imgs.length > 4 ? ' many' : ''}`}>
                          {r.imgs.map((img, j) => <View key={j} className='img-thumb'>{img}</View>)}
                        </View>
                      ) : null}
                      {r.text ? <View className='cal-rec-text'>{r.text}</View> : null}
                      <View className='cal-rec-by'>{getBy(r.by).name} 记录</View>
                    </View>
                  </View>
                ))}
              </>
            )}
          </View>
        </View>
      </View>

      <View className='cal-fab' id='calFab'>
        <View className='cal-fab-btn' id='calAddBtn' onClick={openAdd}>＋</View>
      </View>

      {/* 添加记录 */}
      <View className={`cal-overlay${overlayOpen ? ' show' : ''}`} id='calOverlay' onClick={() => setOverlayOpen(false)}>
        <View className='cal-sheet' id='calSheet' onClick={(e) => e.stopPropagation()}>
          <View className='cal-sheet-handle' />
          <View className='cal-sheet-title'>添加记录</View>
          <View className='cal-field-label'>分类</View>
          <View className='cal-seg'>
            <View className={`cal-seg-item daily-s${addCat === 'daily' ? ' active' : ''}`} onClick={() => setAddCat('daily')}><Text className='seg-dot' />日常</View>
            <View className={`cal-seg-item med-s${addCat === 'medical' ? ' active' : ''}`} onClick={() => setAddCat('medical')}><Text className='seg-dot' />医疗</View>
          </View>
          <View className='cal-field-label'>宠物</View>
          <View className='cal-pet-chips' id='calPetChips'>
            {CAL_PETS.map(p => (
              <Text key={p.id} className={`cal-pet-chip${addPet === p.id ? ' selected' : ''}`} onClick={() => setAddPet(p.id)}>{p.emoji} {p.name}</Text>
            ))}
          </View>
          <View className='cal-field-label'>时间</View>
          <Input type='text' className='cal-textarea' id='calTimeInput' style={{ minHeight: '44px', padding: '11px 14px' }} value={addTime} onInput={(e) => setAddTime(e.detail.value)} />
          <View className='cal-field-label'>记录内容</View>
          <Textarea
            className='cal-textarea' id='calText' style={{ minHeight: '88px' }}
            placeholder={addCat === 'medical' ? '记录做了什么医疗行为，如：今天接种狂犬疫苗第二针…' : '记录日常点滴，如：今天终于学会握手啦！'}
            value={addText} onInput={(e) => setAddText(e.detail.value)}
          />
          {addCat === 'medical' && (
            <View id='calRemindBox'>
              <View className='cal-switch-row'>
                <View className='sr-label'>⏰ 添加待办提醒（仅医疗可设）</View>
                <View className={`cal-switch${addRemind ? ' on' : ''}`} id='calRemindSwitch' onClick={() => setAddRemind(!addRemind)} />
              </View>
              {addRemind && (
                <View className='cal-remind-panel' id='calRemindPanel'>
                  <View className='cal-remind-row'><Text>提醒日期</Text><Text className='rr-val' id='calRemindDateVal'>{nextRemindDate()}</Text></View>
                  <View className='cal-remind-row'><Text>重复周期</Text></View>
                  <View className='cal-cycle-chips' id='calCycleChips'>
                    <Text className={`cal-cycle-chip${addCycle === 'once' ? ' selected' : ''}`} onClick={() => setAddCycle('once')}>一次性</Text>
                    <Text className={`cal-cycle-chip${addCycle === 'quarterly' ? ' selected' : ''}`} onClick={() => setAddCycle('quarterly')}>每3个月</Text>
                    <Text className={`cal-cycle-chip${addCycle === 'yearly' ? ' selected' : ''}`} onClick={() => setAddCycle('yearly')}>每年</Text>
                  </View>
                </View>
              )}
            </View>
          )}
          <View className={`cal-submit-btn${saveOk ? '' : ' disabled'}`} id='calSubmitBtn' onClick={saveRecord}>保存记录</View>
        </View>
      </View>

      {/* 标记完成 */}
      <View className={`cal-overlay${doneTodo ? ' show' : ''}`} id='calDoneOverlay' onClick={() => setDoneTodo(null)}>
        <View className='cal-sheet' onClick={(e) => e.stopPropagation()}>
          <View className='cal-sheet-handle' />
          <View className='cal-sheet-title'>✅ 标记完成</View>
          <View className='cal-done-info'>
            <Text>「{doneTodo?.title}」</Text> 已完成？
            系统自动生成医疗记录，并以<Text>实际完成日期（今天）为基准</Text>，按周期 <Text>{doneTodo?.cycleName}</Text> 自动顺延下一次提醒。
          </View>
          <View className='cal-sheet-actions'>
            <View className='cal-btn cancel' id='calDoneCancel' onClick={() => setDoneTodo(null)}>取消</View>
            <View className='cal-btn save' id='calDoneConfirm' onClick={confirmDone}>确认完成</View>
          </View>
        </View>
      </View>
    </View>
  )
}
