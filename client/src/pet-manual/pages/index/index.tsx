import { useState, useRef, useEffect } from 'react'
import { View, Text, Button, Input, Image, DateInput } from '../../components/compat'
import Taro from '@tarojs/taro'
import ManualPanel from './ManualPanel'
import AskPanel from './AskPanel'
import CalPanel from './CalPanel'
import { PETS as INIT_PETS, FAMILY as INIT_FAMILY, FamilyMember, PetRecord, CalRecord, CalTodo, INIT_CAL_RECORDS, INIT_CAL_TODOS } from './data'
import * as PetAPI from '../../services/pet'
import { apiProfileToRecord, formToCreatePetBody, formToProfilePatch } from '../../services/mappers'
import { ApiError, isFamilyError } from '../../services/request'
import '../../styles/pet-manual.scss'

// 手写字体（合并自 Pet-Manual src/app.ts 与 src/index.html 的字体引入职责）：
// 小程序端用 loadFontFace 补回 H5 端的手写字体（Ma Shan Zheng / ZCOOL KuaiLe）。
// 注意：真机上需把 fonts.gstatic.com 加入 downloadFile 合法域名。
const WEAPP_FONTS: Array<[string, string]> = [
  ['Ma Shan Zheng', 'https://fonts.gstatic.com/s/mashanzheng/v18/NaPecZTRCLxvwo41b4gvzkXaRMQ.ttf'],
  ['ZCOOL KuaiLe', 'https://fonts.gstatic.com/s/zcoolkuaile/v22/tssqApdaRQokwFjFJjvM6h2Wpg.ttf'],
]

type Section = 'album' | 'calendar' | 'ai'

type OnboardingStep = 'choice' | 'create-family' | 'create-profile' | 'join-code' | 'join-profile' | 'waiting'

type MembersTab = 'list' | 'applications'

const ONBOARDING_SETTINGS: Record<OnboardingStep, { title: string; progress: string; back: boolean }> = {
  choice: { title: '', progress: '', back: false },
  'create-family': { title: '创建家庭', progress: '1 / 2', back: true },
  'create-profile': { title: '创建家庭', progress: '2 / 2', back: true },
  'join-code': { title: '加入家庭', progress: '1 / 2', back: true },
  'join-profile': { title: '加入家庭', progress: '2 / 2', back: true },
  waiting: { title: '加入家庭', progress: '', back: false }
}

const ONBOARDING_PREVIOUS: Partial<Record<OnboardingStep, OnboardingStep>> = {
  'create-family': 'choice',
  'create-profile': 'create-family',
  'join-code': 'choice',
  'join-profile': 'join-code'
}

const speciesOf = (type: string) => type.split('<br>')[0]

const SPECIES_OPTIONS = [
  { id: 'cat', label: '猫' },
  { id: 'dog', label: '狗' },
  { id: 'rabbit', label: '兔' },
  { id: 'hamster', label: '仓鼠' },
  { id: 'bird', label: '鸟' },
  { id: 'other', label: '其他' },
]

const GENDER_OPTIONS = [
  { id: 'male', label: '公' },
  { id: 'female', label: '母' },
  { id: 'unknown', label: '未知' },
]

const NEUTERED_OPTIONS = [
  { id: 'yes', label: '已绝育' },
  { id: 'no', label: '未绝育' },
  { id: 'unknown', label: '未知' },
]

const HEALTH_OPTIONS = [
  { id: 'healthy', label: '健康' },
  { id: 'attention', label: '需关注' },
  { id: 'treatment', label: '治疗中' },
]

const todayString = () => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const newPetFromForm = (form: {
  name: string; avatar: string; speciesId: string; speciesOther: string; genderId: string; neuteredId: string;
  birthDate: string; arrivalDate: string; healthId: string; tagsText: string; quote: string
}, idx: number): PetRecord => {
  const speciesOption = SPECIES_OPTIONS.find(s => s.id === form.speciesId) || SPECIES_OPTIONS[0]
  const speciesLabel = form.speciesId === 'other' ? (form.speciesOther.trim() || '其他') : speciesOption.label
  const gender = GENDER_OPTIONS.find(g => g.id === form.genderId) || GENDER_OPTIONS[0]
  const neutered = NEUTERED_OPTIONS.find(n => n.id === form.neuteredId) || NEUTERED_OPTIONS[0]
  const health = HEALTH_OPTIONS.find(h => h.id === form.healthId) || HEALTH_OPTIONS[0]
  const tags = form.tagsText.split(/[，,\s]+/).map(t => t.trim()).filter(Boolean).slice(0, 8)
  const genderLine = gender.id === 'unknown' ? '性别未知' : `${gender.label} · ${neutered.label}`
  const today = todayString()
  return {
    id: `pet_${Date.now()}_${idx}`,
    name: form.name.trim(),
    title: `${form.name.trim()}的成长小册`,
    quote: form.quote.trim() || `${form.name.trim()}，是我们家的新成员。`,
    years: `${form.birthDate.slice(0, 4)} — 至今`,
    type: `${speciesLabel}<br>${genderLine}`,
    birthDate: form.birthDate,
    arrivalDate: form.arrivalDate,
    tags: tags.length ? tags : ['新成员'],
    personalityQuestions: [
      { title: '它喜欢什么', summary: '还在相处中', detail: '把它带回家后，慢慢记录它喜欢的小事。' },
      { title: '它害怕什么', summary: '正在观察', detail: '新环境里它可能有些紧张，等它熟悉后补充。' },
      { title: '它有哪些生活习惯', summary: '记录中', detail: '相处一周后再来补充它的作息和饮食偏好。' },
      { title: '和它相处时需要注意', summary: '给它一点时间', detail: '新成员到家前几天少打扰，准备好食物和窝。' },
      { title: '我们眼中的它', summary: '家里的小惊喜', detail: '欢迎这位新朋友，故事从这里开始。' },
    ],
    health: {
      overall: health.id === 'healthy' ? '健康' : health.id === 'attention' ? '需关注' : '治疗中',
      allergies: { label: '过敏信息', value: '未记录', note: '入住后补充' },
      diseases: { label: '既往疾病', value: '未记录', note: '入住后补充' },
      medications: { label: '长期用药', value: '未记录', note: '入住后补充' },
      vaccines: { label: '最近疫苗', value: '未记录', note: '入住后补充' },
    },
    updated: `我 添加于 ${today}`,
    backFamily: `${INIT_FAMILY.name} · 更新于 ${today}`,
    weights: [],
    events: [],
    birthdays: [],
    _avatar: form.avatar,
    _health: health.id,
  } as PetRecord & { _avatar?: string; _health?: string }
}

export default function Index() {
  // 字体加载：weapp 端走 loadFontFace；H5 端动态注入 <link>（等价于原项目 index.html 的字体引入，不改主工程模板）
  useEffect(() => {
    if (process.env.TARO_ENV === 'weapp') {
      WEAPP_FONTS.forEach(([family, url]) => {
        Taro.loadFontFace({
          family,
          source: `url("${url}")`,
          global: true,
          fail: (e) => console.warn('字体加载失败:', family, e),
        })
      })
    } else if (typeof document !== 'undefined' && !document.querySelector('link[data-pet-manual-font]')) {
      const link = document.createElement('link')
      link.rel = 'stylesheet'
      link.setAttribute('data-pet-manual-font', '1')
      link.href = 'https://fonts.googleapis.com/css2?family=Ma+Shan+Zheng&family=ZCOOL+KuaiLe&display=swap'
      document.head.appendChild(link)
    }
  }, [])

  const [section, setSection] = useState<Section>('calendar')
  const [bookPage, setBookPage] = useState(0)
  const [toastMsg, setToastMsg] = useState('')
  const [pets, setPets] = useState<PetRecord[]>(INIT_PETS)
  const [activePetId, setActivePetId] = useState(INIT_PETS[0].id)
  const [familyMembers, setFamilyMembers] = useState<FamilyMember[]>(INIT_FAMILY.members.filter(m => m.status !== 'rejected'))
  const [petOverlayOpen, setPetOverlayOpen] = useState(false)
  const [familyOpen, setFamilyOpen] = useState(false)
  const [membersOpen, setMembersOpen] = useState(false)
  const [addPetOpen, setAddPetOpen] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null)
  const [membersTab, setMembersTab] = useState<MembersTab>('list')
  const [onboardingOpen, setOnboardingOpen] = useState(true)
  const [obStep, setObStep] = useState<OnboardingStep>('choice')
  const [obExit, setObExit] = useState<'create' | 'join' | ''>('')
  const [obFamilyName, setObFamilyName] = useState('')
  const [obProfileName, setObProfileName] = useState('')
  const [obFamilyCode, setObFamilyCode] = useState('')
  const [obJoinName, setObJoinName] = useState('')
  const [familyName, setFamilyName] = useState(INIT_FAMILY.name)
  const [familyCode] = useState(INIT_FAMILY.code)
  /* 日历记录/待办：提升到此处，档案成长足迹与日历板块共享同一份数据 */
  const [calRecords, setCalRecords] = useState<CalRecord[]>(INIT_CAL_RECORDS)
  const [calTodos, setCalTodos] = useState<CalTodo[]>(INIT_CAL_TODOS)
  const [formName, setFormName] = useState('')
  const [formAvatar, setFormAvatar] = useState('')
  const [formSpecies, setFormSpecies] = useState('cat')
  const [formSpeciesOther, setFormSpeciesOther] = useState('')
  const [formGender, setFormGender] = useState('unknown')
  const [formNeutered, setFormNeutered] = useState('unknown')
  const [formBirth, setFormBirth] = useState('2024-01-01')
  const [formArrival, setFormArrival] = useState(todayString())
  const [formHealth, setFormHealth] = useState('healthy')
  const [formBreed, setFormBreed] = useState('')
  const [formTags, setFormTags] = useState('')
  const [formQuote, setFormQuote] = useState('')
  // petsLoading/petsError 仅驱动加载逻辑（回退 mock/引导），UI 暂未消费这两个值
  const [, setPetsLoading] = useState(false)
  const [, setPetsError] = useState<string | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const activePet = pets.find(p => p.id === activePetId) || pets[0]

  // 挂载时从后端加载宠物列表
  useEffect(() => {
    let cancelled = false
    const loadPets = async () => {
      setPetsLoading(true)
      setPetsError(null)
      try {
        const apiPets = await PetAPI.getPets()
        if (cancelled) return
        if (apiPets && apiPets.length > 0) {
          // 列表仅含 id/name，退化为最小档案组装（此入口已退役，保持可回挂即可）
          const records = apiPets.map(api => apiProfileToRecord({
            id: api.id, name: api.name, avatar_asset_id: '', cover_asset_id: '',
            breed: '', gender: 'unknown', sterilized: false,
            birthday: null, home_date: null, age: 0, companion_days: 0, next_birthday_days: null,
          }))
          setPets(records)
          setActivePetId(records[0].id)
        }
        // 后端返回空列表时保留 mock 数据（开发阶段兼容）
      } catch (err) {
        if (cancelled) return
        // 开发阶段后端未就绪，静默回退到 mock 数据
        if (err instanceof ApiError) {
          setPetsError(err.msg)
          // 40301（guest 无家庭）引导用户创建/加入家庭
          if (isFamilyError(err.code)) {
            // 打开引导页让用户创建或加入家庭
            setOnboardingOpen(true)
          }
        }
        // 非 ApiError（网络异常等）静默，保留 mock 数据
      } finally {
        if (!cancelled) setPetsLoading(false)
      }
    }
    loadPets()
    return () => { cancelled = true }
  }, [])

  const showToast = (message: string) => {
    setToastMsg(message)
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setToastMsg(''), 2200)
  }

  const switchSection = (s: Section) => {
    if (s === 'album') setBookPage(0)
    setSection(s)
  }

  const choosePet = (id: string) => {
    if (id !== activePetId) {
      setActivePetId(id)
      setBookPage(0)
    }
    setPetOverlayOpen(false)
  }

  const openFamily = () => setFamilyOpen(true)

  const openAddPetFromOverlay = () => {
    setPetOverlayOpen(false)
    setTimeout(() => setAddPetOpen(true), 220)
  }

  const openAddPetFromFamily = () => {
    setAddPetOpen(true)
  }

  const closeAddPet = () => setAddPetOpen(false)

  /* 小程序端选择头像：Taro.chooseImage 返回本地临时路径 */
  const chooseAvatar = () => {
    Taro.chooseImage({
      count: 1,
      sizeType: ['compressed'],
      success: (res) => {
        const path = res.tempFilePaths && res.tempFilePaths[0]
        if (path) setFormAvatar(path)
      },
      fail: () => { /* 用户取消选择 */ }
    })
  }

  const submitAddPet = async () => {
    const name = formName.trim()
    if (!name) { showToast('请填写宠物名字'); return }

    try {
      // 创建（后端只收 name）→ PATCH profile 补富字段（此入口已退役，保持可回挂即可）
      const created = await PetAPI.createPet(formToCreatePetBody({ name }))
      let profile
      try {
        profile = await PetAPI.updatePetProfile(created.id, formToProfilePatch({
          breed: formBreed, genderId: formGender, neuteredId: formNeutered,
          birthDate: formBirth, arrivalDate: formArrival,
        }))
      } catch {
        profile = {
          id: created.id, name, avatar_asset_id: '', cover_asset_id: '',
          breed: formBreed.trim(), gender: formGender, sterilized: formNeutered === 'yes',
          birthday: formBirth || null, home_date: formArrival || null,
          age: 0, companion_days: 0, next_birthday_days: null,
        }
      }
      const newPet = apiProfileToRecord(profile)
      // 保留本地头像预览
      const petMeta = newPet as PetRecord & { _avatar?: string }
      if (formAvatar) petMeta._avatar = formAvatar
      setPets(prev => [...prev, newPet])
      setActivePetId(newPet.id)
      setSection('album')
      setBookPage(0)
      showToast(`${name} 已加入家庭`)
    } catch (err) {
      // 后端未就绪时回退到本地创建（开发阶段兼容）
      const newPet = newPetFromForm({
        name, avatar: formAvatar, speciesId: formSpecies, speciesOther: formSpeciesOther, genderId: formGender,
        neuteredId: formNeutered, birthDate: formBirth, arrivalDate: formArrival,
        healthId: formHealth, tagsText: formTags, quote: formQuote,
      }, pets.length)
      setPets(prev => [...prev, newPet])
      setActivePetId(newPet.id)
      setSection('album')
      setBookPage(0)
      const errMsg = err instanceof ApiError ? err.msg : '网络异常'
      showToast(`${name} 已加入（离线模式：${errMsg}）`)
    } finally {
      setFormName(''); setFormAvatar(''); setFormSpecies('cat'); setFormSpeciesOther(''); setFormGender('unknown')
      setFormNeutered('unknown'); setFormBirth('2024-01-01'); setFormArrival(todayString())
      setFormHealth('healthy'); setFormBreed(''); setFormTags(''); setFormQuote('')
      setAddPetOpen(false)
    }
  }

  const approveMember = (idx: number) => {
    setFamilyMembers(prev => prev.map((m, i) => i === idx
      ? { ...m, status: 'active', reviewedAt: todayString() }
      : m))
    const target = familyMembers[idx]
    if (target) showToast(`${target.name} 已加入家庭`)
  }

  const rejectMember = (idx: number) => {
    const target = familyMembers[idx]
    setFamilyMembers(prev => prev.filter((_, i) => i !== idx))
    if (target) showToast(`已拒绝 ${target.name} 的申请`)
  }

  const removeMember = (name: string) => {
    setFamilyMembers(prev => prev.filter(m => m.name !== name))
    setConfirmRemove(null)
    showToast(`${name} 已从家庭中移除`)
  }

  const copyFamilyCode = () => {
    Taro.setClipboardData({ data: familyCode })
    showToast(`家庭码 ${familyCode} 已复制`)
  }

  const obSetting = ONBOARDING_SETTINGS[obStep]

  const obBack = () => {
    const prev = ONBOARDING_PREVIOUS[obStep]
    if (prev) setObStep(prev)
  }

  const obSkip = () => {
    setFamilyName('个人空间')
    setBookPage(0)
    setSection('calendar')
    setOnboardingOpen(false)
  }

  const obCompleteCreation = () => {
    setFamilyName(obFamilyName.trim())
    setSection('calendar')
    setOnboardingOpen(false)
    showToast(`${obFamilyName.trim()}已创建`)
  }

  const tabIdx = section === 'album' ? 0 : section === 'calendar' ? 1 : 2

  const pendingCount = familyMembers.filter(m => m.status === 'pending').length
  const memberCount = familyMembers.filter(m => m.status === 'active').length

  return (
    <View className='app' id='app' {...{ 'data-page': String(bookPage) }}>
      {/* 模拟状态栏已删除：微信开发者工具/真机自带系统状态栏，避免重复显示 */}

      {/* 首次使用引导（问询页） */}
      {onboardingOpen && (
        <View className='onboarding-screen' id='onboardingScreen'>
          <View className='onboarding-header'>
            {obSetting.back && <Button className='onboarding-back' onClick={obBack}>‹</Button>}
            <Text className='h2' style={{ visibility: obSetting.title ? 'visible' : 'hidden' }}>{obSetting.title}</Text>
          </View>
          <View className='onboarding-body'>
            <Text className='onboarding-progress span'>{obSetting.progress}</Text>

            {obStep === 'choice' && (
              <View className='onboarding-step onboarding-choice-step'>
                <Text className='h1'>欢迎加入</Text>
                <Text className='onboarding-lead p'>选择加入或者创建你的家庭吧</Text>
                <View className='onboarding-center-card' />
                <View className='onboarding-choice-list'>
                  <Button
                    className={`onboarding-choice${obExit === 'create' ? ' slide-out' : ''}`}
                    onClick={() => {
                      if (obExit) return
                      setObExit('create')
                      setTimeout(() => {
                        setObStep('create-family')
                        setObExit('')
                      }, 340)
                    }}
                  >
                    <Text className='span'><Text className='strong'>创建家庭</Text><Text className='span'>建立一个新家庭，并邀请家人一起维护宠物档案。</Text></Text>
                    <Text className='i'>›</Text>
                  </Button>
                  <Button
                    className={`onboarding-choice${obExit === 'join' ? ' slide-out' : ''}`}
                    onClick={() => {
                      if (obExit) return
                      setObExit('join')
                      setTimeout(() => {
                        setObStep('join-code')
                        setObExit('')
                      }, 340)
                    }}
                  >
                    <Text className='span'><Text className='strong'>加入家庭</Text><Text className='span'>使用家庭码申请加入已有家庭。</Text></Text>
                    <Text className='i'>›</Text>
                  </Button>
                </View>
                <Button className='onboarding-skip' onClick={obSkip}>暂时跳过</Button>
              </View>
            )}

            {obStep === 'create-family' && (
              <View className='onboarding-step'>
                <Text className='h1'>创建家庭</Text>
                <Text className='onboarding-lead p'>填写家人共同看到的家庭名称。</Text>
                <View className='onboarding-form'>
                  <View className='form-field'>
                    <Text className='label'>家庭名称</Text>
                    <Input className='capsule-input' id='onboardingFamilyName' maxlength={20} placeholder='例如：我们家'
                      value={obFamilyName} onInput={(e) => setObFamilyName(e.detail.value)} />
                  </View>
                  <Button className='primary-button' disabled={!obFamilyName.trim()} onClick={() => { if (obFamilyName.trim()) setObStep('create-profile') }}>下一步</Button>
                </View>
              </View>
            )}

            {obStep === 'create-profile' && (
              <View className='onboarding-step'>
                <Text className='h1'>你的信息</Text>
                <Text className='onboarding-lead p'>家人会通过这个名字识别你留下的记录。</Text>
                <View className='onboarding-form'>
                  <View className='onboarding-summary'>家庭：{obFamilyName.trim()}</View>
                  <View className='form-field'>
                    <Text className='label'>名字</Text>
                    <Input className='capsule-input' id='createProfileName' maxlength={20} placeholder='请输入你的名字'
                      value={obProfileName} onInput={(e) => setObProfileName(e.detail.value)} />
                  </View>
                  <Button className='primary-button' disabled={!obProfileName.trim()} onClick={() => { if (obProfileName.trim()) obCompleteCreation() }}>创建家庭</Button>
                </View>
              </View>
            )}

            {obStep === 'join-code' && (
              <View className='onboarding-step'>
                <Text className='h1'>加入家庭</Text>
                <Text className='onboarding-lead p'>向家庭主人获取家庭码并填写。</Text>
                <View className='onboarding-form'>
                  <View className='form-field'>
                    <Text className='label'>家庭码</Text>
                    <Input className='capsule-input' id='joinFamilyCode' maxlength={8} placeholder='请输入家庭码'
                      value={obFamilyCode} onInput={(e) => setObFamilyCode(e.detail.value.toUpperCase())} />
                  </View>
                  <Button className='primary-button' disabled={!obFamilyCode.trim()} onClick={() => { if (obFamilyCode.trim()) setObStep('join-profile') }}>下一步</Button>
                </View>
              </View>
            )}

            {obStep === 'join-profile' && (
              <View className='onboarding-step'>
                <Text className='h1'>你的信息</Text>
                <Text className='onboarding-lead p'>提交后，家庭主人会看到你的名字和加入申请。</Text>
                <View className='onboarding-form'>
                  <View className='onboarding-summary'>家庭码：{obFamilyCode.trim()}</View>
                  <View className='form-field'>
                    <Text className='label'>名字</Text>
                    <Input className='capsule-input' id='joinProfileName' maxlength={20} placeholder='请输入你的名字'
                      value={obJoinName} onInput={(e) => setObJoinName(e.detail.value)} />
                  </View>
                  <Button className='primary-button' disabled={!obJoinName.trim()} onClick={() => { if (obJoinName.trim()) setObStep('waiting') }}>提交加入申请</Button>
                </View>
              </View>
            )}

            {obStep === 'waiting' && (
              <View className='onboarding-step onboarding-waiting'>
                <View className='onboarding-waiting-mark'>···</View>
                <Text className='h1'>等待家庭主人通过</Text>
                <Text className='onboarding-lead p'>申请已提交。家庭主人通过后，你就可以查看并共同维护宠物档案。</Text>
                <View className='onboarding-summary'>家庭码：{obFamilyCode.trim()} · 申请人：{obJoinName.trim()}</View>
                <Button className='primary-button' onClick={() => showToast('家庭主人暂未通过申请')}>刷新申请状态</Button>
                <Button className='secondary-button' onClick={() => setObStep('join-code')}>修改申请信息</Button>
              </View>
            )}
          </View>
        </View>
      )}

      <View className='topbar'>
        <Button className='icon-button' onClick={openFamily}>⌂</Button>
        <Button className='pet-switch' onClick={() => setPetOverlayOpen(true)}>
          <View className='avatar-placeholder' />
          <Text className='strong'>{activePet.name}</Text>
          <Text className='span'>⌄</Text>
        </Button>
        <View className='top-actions'>
          <Button className='icon-button' onClick={() => showToast('更多操作建设中')}>•••</Button>
        </View>
      </View>

      {/* 档案册（书） */}
      {section === 'album' && (
        <View className='module-screen album-screen' id='albumModuleScreen'>
          <View className='module-header'>
            <View className='header-actions'>
              <Button className='module-pet-switch' onClick={() => setPetOverlayOpen(true)}>
                <View className='avatar-placeholder' />
                <Text className='strong'>{activePet.name}</Text>
                <Text className='span'>⌄</Text>
              </Button>
            </View>
          </View>
          <ManualPanel pet={activePet} page={bookPage} onTurn={setBookPage} onToast={showToast}
            onAddRecord={(r) => setCalRecords(prev => [...prev, r])}
            onAddTodo={(t) => setCalTodos(prev => [...prev, t])} />
        </View>
      )}

      {/* AI 问问屏 */}
      {section === 'ai' && (
        <View className='module-screen ai-screen' id='aiModuleScreen'>
          <AskPanel />
        </View>
      )}

      {/* 宠物日历屏 */}
      {section === 'calendar' && (
        <View className='module-screen calendar-screen' id='calendarModuleScreen'>
          <CalPanel onToast={showToast} records={calRecords} todos={calTodos}
            onRecordsChange={setCalRecords} onTodosChange={setCalTodos} />
        </View>
      )}

      {/* 宠物切换弹层 */}
      <View className={`overlay${petOverlayOpen ? ' open' : ''}`} id='petOverlay' onClick={() => setPetOverlayOpen(false)}>
        <View className='sheet' onClick={(e) => e.stopPropagation()}>
          <View className='handle' />
          <View className='sheet-head'>
            <View><Text className='h2'>{familyName}</Text><Text className='p'>选择要查看的宠物档案册</Text></View>
            <Button className='close-button' onClick={() => setPetOverlayOpen(false)}>×</Button>
          </View>
          <View className='pet-list' id='petList'>
            {pets.map(p => {
              const petMeta = p as PetRecord & { _emoji?: string; _avatar?: string }
              return (
                <Button key={p.id} className={`pet-item${p.id === activePetId ? ' selected' : ''}`}
                  onClick={() => choosePet(p.id)}>
                  <View className={`avatar-placeholder${petMeta._avatar ? ' has-photo' : ''}`}>
                    {petMeta._avatar
                      ? <Image className='avatar-photo' src={petMeta._avatar} mode='aspectFill' />
                      : (petMeta._emoji || '')}
                  </View>
                  <View className='pet-name-row'>
                    <Text className='strong'>{p.name}</Text>
                    {p.id === activePetId && <Text className='i pet-check'>✓</Text>}
                  </View>
                  <Text className='span'>{speciesOf(p.type)}</Text>
                </Button>
              )
            })}
          </View>
          <Button className='add-pet' id='addPetButton' onClick={openAddPetFromOverlay}>＋ 添加宠物</Button>
        </View>
      </View>

      {/* 添加宠物表单弹层 */}
      <View className={`overlay${addPetOpen ? ' open' : ''}`} id='addPetOverlay' onClick={closeAddPet}>
        <View className='sheet sheet-tall' onClick={(e) => e.stopPropagation()}>
          <View className='handle' />
          <View className='sheet-head'>
            <View><Text className='h2'>添加宠物</Text><Text className='p'>填写档案信息后加入家庭</Text></View>
            <Button className='close-button' onClick={closeAddPet}>×</Button>
          </View>
          <View className='add-pet-form'>
            <View className='form-field'>
              <Text className='label'>头像</Text>
              <View className='avatar-picker'>
                <View className={`avatar-preview${formAvatar ? ' has-photo' : ''}`}>
                  {formAvatar && <Image className='avatar-photo' src={formAvatar} mode='aspectFill' />}
                </View>
                <Button className='avatar-pick-button' onClick={chooseAvatar}>从相册选择</Button>
              </View>
            </View>

            <View className='form-field'>
              <Text className='label'>名字 *</Text>
              <Input className='capsule-input' id='addPetName' maxlength={12} placeholder='给宠物起个名字'
                value={formName} onInput={(e) => setFormName(e.detail.value)} />
            </View>

            <View className='form-field'>
              <Text className='label'>物种</Text>
              <View className='chip-group'>
                {SPECIES_OPTIONS.map(s => (
                  <Button key={s.id} className={`chip${formSpecies === s.id ? ' selected' : ''}`}
                    onClick={() => setFormSpecies(s.id)}>
                    <Text className='span'>{s.label}</Text>
                  </Button>
                ))}
              </View>
              {formSpecies === 'other' && (
                <Input className='species-other-input capsule-input' maxlength={8} placeholder='填写物种，如：刺猬'
                  value={formSpeciesOther} onInput={(e) => setFormSpeciesOther(e.detail.value)} />
              )}
            </View>

            <View className='form-field'>
              <Text className='label'>品种（可选）</Text>
              <Input className='capsule-input' id='addPetBreed' maxlength={20} placeholder='如：英国短毛猫'
                value={formBreed} onInput={(e) => setFormBreed(e.detail.value)} />
            </View>

            <View className='form-field'>
              <Text className='label'>性别</Text>
              <View className='chip-group chip-group-tight'>
                {GENDER_OPTIONS.map(g => (
                  <Button key={g.id} className={`chip chip-sm${formGender === g.id ? ' selected' : ''}`}
                    onClick={() => setFormGender(g.id)}>
                    <Text className='span'>{g.label}</Text>
                  </Button>
                ))}
              </View>
            </View>

            <View className='form-field'>
              <Text className='label'>绝育</Text>
              <View className='chip-group chip-group-tight'>
                {NEUTERED_OPTIONS.map(n => (
                  <Button key={n.id} className={`chip chip-sm${formNeutered === n.id ? ' selected' : ''}`}
                    onClick={() => setFormNeutered(n.id)}>
                    <Text className='span'>{n.label}</Text>
                  </Button>
                ))}
              </View>
            </View>

            <View className='form-row'>
              <View className='form-field'>
                <Text className='label'>出生日期</Text>
                <DateInput value={formBirth} max={todayString()} onChange={(e) => setFormBirth(e.detail.value)} />
              </View>
              <View className='form-field'>
                <Text className='label'>到家日期</Text>
                <DateInput value={formArrival} max={todayString()} onChange={(e) => setFormArrival(e.detail.value)} />
              </View>
            </View>

            <View className='form-field'>
              <Text className='label'>健康状态</Text>
              <View className='chip-group'>
                {HEALTH_OPTIONS.map(h => (
                  <Button key={h.id} className={`chip${formHealth === h.id ? ' selected' : ''}`}
                    onClick={() => setFormHealth(h.id)}>
                    <Text className='span'>{h.label}</Text>
                  </Button>
                ))}
              </View>
            </View>

            <View className='form-field'>
              <Text className='label'>性格标签</Text>
              <Input className='capsule-input' id='addPetTags' maxlength={40} placeholder='多个用空格或逗号分隔，如：粘人 贪吃'
                value={formTags} onInput={(e) => setFormTags(e.detail.value)} />
            </View>

            <View className='form-field'>
              <Text className='label'>个性寄语（可选）</Text>
              <Input className='capsule-input' id='addPetQuote' maxlength={30} placeholder='一句话介绍它'
                value={formQuote} onInput={(e) => setFormQuote(e.detail.value)} />
            </View>

            <View className='add-pet-form-actions'>
              <Button className='secondary-button' onClick={closeAddPet}>取消</Button>
              <Button className='primary-button' disabled={!formName.trim()} onClick={submitAddPet}>添加到家庭</Button>
            </View>
          </View>
        </View>
      </View>

      {/* 家庭中心 */}
      <View className={`create-screen${familyOpen ? ' open' : ''}`} id='familyScreen'>
        <View className='module-header'>
          <Button className='create-back' onClick={() => setFamilyOpen(false)}>‹</Button>
          <Text className='h1'>家庭</Text>
        </View>
        <View className='family-center-content'>
          <View className='family-center-hero'>
            <Text className='h1' id='familyCenterName'>{familyName}</Text>
            <Text className='p' id='familyCenterSubtitle'>{memberCount} 位家人 · {pets.length} 只宠物</Text>
          </View>
          <View id='familyJoinedView'>
            <View className='family-center-section'>
              <Text className='h3'>家庭信息</Text>
              <View className='family-code-row'><Text className='span'>家庭码</Text><Text className='strong' id='familyCodeValue'>{familyCode}</Text></View>
              <View className='family-code-actions'>
                <Button className='secondary-button' onClick={copyFamilyCode}>复制家庭码</Button>
                <Button className='secondary-button' onClick={() => showToast('把家庭码告诉家人，等待他们加入')}>邀请家人</Button>
              </View>
            </View>
            <View className='family-center-section'>
              <Text className='h3'>家庭宠物</Text>
              {pets.map(p => {
                const petMeta = p as PetRecord & { _emoji?: string; _avatar?: string }
                return (
                  <View key={p.id} className='family-list-row'>
                    <View className='family-list-person'>
                      <View className={`avatar-placeholder${petMeta._avatar ? ' has-photo' : ''}`}>
                        {petMeta._avatar
                          ? <Image className='avatar-photo' src={petMeta._avatar} mode='aspectFill' />
                          : (petMeta._emoji || '')}
                      </View>
                      <Text className='strong'>{p.name}</Text>
                    </View>
                    <Text className='span'>{speciesOf(p.type)}</Text>
                  </View>
                )
              })}
              <Button className='secondary-button' onClick={openAddPetFromFamily}>添加宠物</Button>
            </View>
            <View className='family-center-section'>
              <Text className='h3'>家庭成员</Text>
              <View className='family-member-preview'>
                {familyMembers.filter(m => m.status === 'active').slice(0, 3).map((m, i) => (
                  <View key={i} className='family-member-mini'>
                    <View className='avatar-placeholder'>{m.name.slice(0, 1)}</View>
                    <Text className='strong'>{m.name}</Text>
                  </View>
                ))}
                {familyMembers.filter(m => m.status === 'active').length === 0 && <Text className='family-empty p'>还没有成员</Text>}
              </View>
              <Button className='secondary-button management-button' onClick={() => { setMembersOpen(true); setMembersTab(pendingCount > 0 ? 'applications' : 'list') }}>
                管理成员与加入申请
                {pendingCount > 0 && <Text className='management-dot' />}
              </Button>
            </View>
          </View>
        </View>
      </View>

      {/* 成员管理抽屉 */}
      <View className={`overlay${membersOpen ? ' open' : ''}`} id='membersOverlay' onClick={() => setMembersOpen(false)}>
        <View className='sheet sheet-tall members-sheet' onClick={(e) => e.stopPropagation()}>
          <View className='handle' />
          <View className='sheet-head'>
            <View><Text className='h2'>成员与申请</Text><Text className='p'>管理家庭成员和处理加入申请</Text></View>
            <Button className='close-button' onClick={() => setMembersOpen(false)}>×</Button>
          </View>
          <View className='members-tabs'>
            <Button className={`members-tab${membersTab === 'list' ? ' active' : ''}`}
              onClick={() => setMembersTab('list')}>
              <Text className='span'>全部成员</Text>
            </Button>
            <Button className={`members-tab${membersTab === 'applications' ? ' active' : ''}`}
              onClick={() => setMembersTab('applications')}>
              <Text className='span'>加入申请</Text>
              {pendingCount > 0 && <Text className='members-tab-dot' />}
            </Button>
          </View>
          <View className='members-list'>
            {familyMembers
              .map((m, i) => ({ m, i }))
              .filter(({ m }) => membersTab === 'list' ? m.status === 'active' : m.status === 'pending')
              .map(({ m, i }) => (
                <View key={i} className={`member-row status-${m.status}`}>
                  <View className='avatar-placeholder'>{m.name.slice(0, 1)}</View>
                  <View className='member-info'>
                    <View className='member-name-row'>
                      <Text className='strong'>{m.name}</Text>
                    </View>
                    <View className='member-meta'>
                      {m.status === 'pending' && <>申请于 {m.appliedAt} · 待审核</>}
                      {m.status === 'active' && <>已加入 · {m.reviewedAt}</>}
                    </View>
                  </View>
                  <View className='member-actions'>
                    {m.status === 'pending' && (
                      <>
                        <Button className='member-action approve' onClick={() => approveMember(i)}>通过</Button>
                        <Button className='member-action reject' onClick={() => rejectMember(i)}>拒绝</Button>
                      </>
                    )}
                    {m.status === 'active' && i > 0 && (
                      <Button className='member-action reject' onClick={() => setConfirmRemove(m.name)}>移除</Button>
                    )}
                  </View>
                </View>
              ))}
            {familyMembers.filter(m => membersTab === 'list' ? m.status === 'active' : m.status === 'pending').length === 0 && (
              <View className='members-empty'>{membersTab === 'list' ? '还没有成员' : '没有待处理的申请'}</View>
            )}
          </View>
        </View>
      </View>

      {/* 移除确认弹窗 */}
      {confirmRemove && (
        <View className='confirm-overlay' onClick={() => setConfirmRemove(null)}>
          <View className='confirm-dialog' onClick={(e) => e.stopPropagation()}>
            <Text className='h3'>移除家庭成员</Text>
            <Text className='p'>确定要将「{confirmRemove}」从家庭中移除吗？移除后将不再共享家庭档案。</Text>
            <View className='confirm-actions'>
              <Button className='secondary-button' onClick={() => setConfirmRemove(null)}>取消</Button>
              <Button className='danger-button' onClick={() => removeMember(confirmRemove)}>确认移除</Button>
            </View>
          </View>
        </View>
      )}

      {/* 底部浮动家庭按钮（导航栏左侧，常驻） */}
      <Button className='floating-family-button' onClick={openFamily}>⌂</Button>

      {/* 底部功能切换 */}
      <View className='app-tabs' id='appTabs'>
        <View className='app-tab-indicator' style={{ transform: `translateX(calc((100% + 4px) * ${tabIdx}))` }} />
        <Button className={`app-tab${section === 'album' ? ' active' : ''}`} onClick={() => switchSection('album')}>
          <Text className='app-tab-icon span'>▤</Text><Text className='app-tab-label span'>宠物档案</Text>
        </Button>
        <Button className={`app-tab${section === 'calendar' ? ' active' : ''}`} onClick={() => switchSection('calendar')}>
          <Text className='app-tab-icon span'>▦</Text><Text className='app-tab-label span'>宠物日历</Text>
        </Button>
        <Button className={`app-tab${section === 'ai' ? ' active' : ''}`} onClick={() => switchSection('ai')}>
          <Text className='app-tab-icon span'>✦</Text><Text className='app-tab-label span'>AI 助手</Text>
        </Button>
      </View>

      <View className={`toast${toastMsg ? ' show' : ''}`} id='toast'>{toastMsg}</View>
    </View>
  )
}
