/**
 * 首页 —— 宠物档案册主页
 *
 * 本页由「文件一（wechat-pet）」原朴素首页重写而来：
 * - 保留原首页的登录/引导闸门（bootstrap / 完善资料 / 无家庭 → 创建或加入）；
 * - 通过闸门后，呈现「文件二（Pet-Manual）」宠物档案册视觉：
 *   topbar + 翻页书（ManualPanel）+ 宠物切换弹层 + 添加宠物表单；
 * - 数据流：宠物列表走后端 GET /pets（失败或为空时回退模块内演示 mock），
 *   档案内个性/健康/成长等子资源由 ManualPanel 自行拉取、失败自动回退；
 * - 「成长足迹」新增的记录/待办写入共享 useCalStore，供「日历」tab 展示。
 */
import { useState, useRef, useEffect, useCallback } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button, Input, Image, DateInput } from '../../pet-manual/components/compat'
import ManualPanel from '../../pet-manual/pages/index/ManualPanel'
import { PETS as INIT_PETS, type PetRecord } from '../../pet-manual/pages/index/data'
import * as PetAPI from '../../pet-manual/services/pet'
import { apiProfileToRecord, formToCreatePetBody, formToProfilePatch } from '../../pet-manual/services/mappers'
import { uploadAsset, buildAssetUrl } from '../../pet-manual/services/asset'
import type { ApiPetProfile } from '../../pet-manual/services/types'
import { ApiError, isFamilyError } from '../../pet-manual/services/request'
import { useCalStore } from '../../pet-manual/stores/useCalStore'
import { routes } from '../../constants/routes'
import { navigateTo } from '../../utils/navigation'
import { useAppStore } from '../../stores/app-store'
import { useAuthStore } from '../../stores/auth-store'
import { useFamilyStore } from '../../stores/family-store'
import { useTabStore } from '../../stores/tab-store'
import './index.scss'
import '../../pet-manual/styles/pet-manual.scss'

type PetWithAvatar = PetRecord & { _avatar?: string; _emoji?: string }

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

/** 离线回退时本地构造演示宠物（与后端无网络时也能走完"添加宠物"流程） */
const newPetFromForm = (form: {
  name: string; avatar: string; speciesId: string; speciesOther: string; genderId: string; neuteredId: string;
  birthDate: string; arrivalDate: string; healthId: string; tagsText: string; quote: string
}, familyName: string, idx: number): PetWithAvatar => {
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
    backFamily: `${familyName || '我们家'} · 更新于 ${today}`,
    weights: [],
    events: [],
    birthdays: [],
    _avatar: form.avatar || undefined,
  }
}

const speciesOf = (type: string) => type.split('<br>')[0]

export default function Index() {
  const { bootstrapCompleted, bootstrapError, retryBootstrap } = useAppStore()
  const { identity, user } = useAuthStore()
  const { family } = useFamilyStore()
  const addRecord = useCalStore(s => s.addRecord)
  const addTodo = useCalStore(s => s.addTodo)
  const setActiveTab = useTabStore(s => s.setActiveTab)

  // 预览模式：门禁「暂时跳过」后直接浏览演示档案（mock），⌂ 退出回到门禁
  const [previewMode, setPreviewMode] = useState(false)

  // 档案册数据（真实后端宠物转 PetRecord；_avatar 兜底头像）
  const [pets, setPets] = useState<PetWithAvatar[]>([])
  const [petsLoaded, setPetsLoaded] = useState(false)
  const [activePetId, setActivePetId] = useState('')
  const [bookPage, setBookPage] = useState(0)

  // 弹层与提示
  const [toastMsg, setToastMsg] = useState('')
  const [petOverlayOpen, setPetOverlayOpen] = useState(false)
  const [addPetOpen, setAddPetOpen] = useState(false)

  // 添加宠物表单
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

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const familyName = family?.name || '我们的家'
  const activePet = pets.find(p => p.id === activePetId) || pets[0]

  const showToast = (message: string) => {
    setToastMsg(message)
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setToastMsg(''), 2200)
  }

  const loadPets = useCallback(async () => {
    if (previewMode) {
      // 预览模式：直接使用模块内演示档案，不请求后端
      setPets(INIT_PETS as PetWithAvatar[])
      setActivePetId(prev => (INIT_PETS.some(p => p.id === prev) ? prev : INIT_PETS[0].id))
      setPetsLoaded(true)
      return
    }
    if (identity === 'guest' || !bootstrapCompleted) {
      setPetsLoaded(true)
      return
    }
    try {
      const apiPets = await PetAPI.getPets()
      if (apiPets && apiPets.length > 0) {
        // 列表仅含 id/name，富字段（品种/性别/生日等）逐只并行拉 profile 组装
        const profiles = await Promise.allSettled(apiPets.map(p => PetAPI.getPetProfile(p.id)))
        const records = apiPets.map((api, i) => {
          const pr = profiles[i]
          const profile: ApiPetProfile = pr.status === 'fulfilled'
            ? pr.value
            : {
              // profile 拉取失败时退化为最小档案（仅 id/name），其余字段默认
              id: api.id, name: api.name, avatar_asset_id: '', cover_asset_id: '',
              breed: '', gender: 'unknown', sterilized: false,
              birthday: null, home_date: null, age: 0, companion_days: 0, next_birthday_days: null,
            }
          const r = apiProfileToRecord(profile) as PetWithAvatar
          const avatarUrl = buildAssetUrl(profile.avatar_asset_id)
          if (avatarUrl) r._avatar = avatarUrl
          return r
        })
        setPets(records)
        setActivePetId(prev => (records.some(p => p.id === prev) ? prev : records[0].id))
      } else {
        // 后端正常返回但暂无宠物：展示空态，由用户添加第一只
        setPets([])
        setActivePetId('')
      }
    } catch (err) {
      // 后端未就绪 / 未配家庭：回退模块内演示 mock，保证界面可浏览
      if (err instanceof ApiError) {
        if (isFamilyError(err.code)) {
          showToast('请先创建或加入家庭（当前展示演示档案）')
        } else {
          showToast(err.msg)
        }
      }
      setPets(INIT_PETS as PetWithAvatar[])
      setActivePetId(prev => (INIT_PETS.some(p => p.id === prev) ? prev : INIT_PETS[0].id))
    } finally {
      setPetsLoaded(true)
    }
  }, [identity, bootstrapCompleted, previewMode])

  useEffect(() => {
    void loadPets()
  }, [loadPets])

  useDidShow(() => {
    // 同步自定义 tabBar 高亮：0=宠物档案
    setActiveTab(0)
    void loadPets()
  })

  const enterPreview = () => {
    setPreviewMode(true)
    showToast('预览模式：浏览演示档案')
  }

  const exitPreview = () => {
    setPreviewMode(false)
    showToast('已退出预览，可继续完善资料')
  }

  const choosePet = (id: string) => {
    if (id !== activePetId) {
      setActivePetId(id)
      setBookPage(0)
    }
    setPetOverlayOpen(false)
  }

  const openAddPet = () => setAddPetOpen(true)

  const closeAddPet = () => setAddPetOpen(false)

  const chooseAvatar = () => {
    Taro.chooseImage({
      count: 1,
      sizeType: ['compressed'],
      success: (res) => {
        const path = res.tempFilePaths && res.tempFilePaths[0]
        if (path) setFormAvatar(path)
      },
      fail: () => { /* 用户取消选择 */ },
    })
  }

  const submitAddPet = async () => {
    const name = formName.trim()
    if (!name) { showToast('请填写宠物名字'); return }

    // 本地展示补充字段（后端暂无 species/health_status/tags/quote 列）
    const speciesOption = SPECIES_OPTIONS.find(s => s.id === formSpecies) || SPECIES_OPTIONS[0]
    const speciesLabel = formSpecies === 'other' ? (formSpeciesOther.trim() || '其他') : speciesOption.label
    const tags = formTags.split(/[，,\s]+/).map(t => t.trim()).filter(Boolean).slice(0, 8)

    try {
      // ① 上传头像（可选）：POST /assets/upload → asset_id
      let avatarAssetId: string | undefined
      if (formAvatar) {
        try {
          avatarAssetId = await uploadAsset(formAvatar, 'pet_avatar')
        } catch {
          // 上传失败继续创建，头像仅本地临时展示
        }
      }

      // ② 创建宠物：后端 CreatePetRequest 只收 name + avatar_asset_id
      const created = await PetAPI.createPet(formToCreatePetBody({ name }, avatarAssetId))

      // ③ 补写富档案字段：PATCH /pets/:id/profile（breed/gender/sterilized/birthday/home_date）
      let profile: ApiPetProfile
      try {
        profile = await PetAPI.updatePetProfile(created.id, formToProfilePatch({
          breed: formBreed, genderId: formGender, neuteredId: formNeutered,
          birthDate: formBirth, arrivalDate: formArrival,
        }))
      } catch {
        // 富字段保存失败：宠物已创建，用表单值在本地补全展示
        profile = {
          id: created.id, name, avatar_asset_id: avatarAssetId || '', cover_asset_id: '',
          breed: formBreed.trim(), gender: formGender, sterilized: formNeutered === 'yes',
          birthday: formBirth || null, home_date: formArrival || null,
          age: 0, companion_days: 0, next_birthday_days: null,
        }
      }

      const newPet = apiProfileToRecord(profile, {
        species: speciesLabel,
        tags,
        quote: formQuote.trim(),
        healthStatus: formHealth as 'healthy' | 'attention' | 'treatment',
      }) as PetWithAvatar
      newPet._avatar = buildAssetUrl(avatarAssetId) || formAvatar || undefined

      setPets(prev => [...prev, newPet])
      setActivePetId(newPet.id)
      setBookPage(0)
      showToast(`${name} 已加入家庭`)
    } catch (err) {
      // 后端未就绪时回退到本地创建（开发/演示阶段兼容）
      const newPet = newPetFromForm({
        name, avatar: formAvatar, speciesId: formSpecies, speciesOther: formSpeciesOther, genderId: formGender,
        neuteredId: formNeutered, birthDate: formBirth, arrivalDate: formArrival,
        healthId: formHealth, tagsText: formTags, quote: formQuote,
      }, familyName, pets.length)
      setPets(prev => [...prev, newPet])
      setActivePetId(newPet.id)
      setBookPage(0)
      const errMsg = err instanceof ApiError ? err.msg : '网络异常'
      showToast(`${name} 已加入（离线演示：${errMsg}）`)
    } finally {
      setFormName(''); setFormAvatar(''); setFormSpecies('cat'); setFormSpeciesOther(''); setFormGender('unknown')
      setFormNeutered('unknown'); setFormBirth('2024-01-01'); setFormArrival(todayString())
      setFormHealth('healthy'); setFormBreed(''); setFormTags(''); setFormQuote('')
      setAddPetOpen(false)
    }
  }

  // ─── 闸门：启动 / 报错 / 完善资料 / 无家庭 ───
  if (!bootstrapCompleted) {
    return (
      <View className='app gate' id='app'>
        <Text className='gate-text'>正在打开档案册…</Text>
      </View>
    )
  }

  if (bootstrapError) {
    return (
      <View className='app gate' id='app'>
        <View className='gate-card'>
          <Text className='gate-title'>启动失败</Text>
          <Text className='gate-sub'>{bootstrapError}</Text>
          <Button className='gate-btn primary' onClick={retryBootstrap}>重新登录</Button>
        </View>
      </View>
    )
  }

  const needsProfileOnboarding = !user?.nickname?.trim() || !user.avatarUrl

  // ─── 门禁一：完善资料（文件二 onboarding 问询页视觉） ───
  if (needsProfileOnboarding && !previewMode) {
    return (
      <View className='app onboarding-root' id='app'>
        <View className='onboarding-screen' id='onboardingScreen'>
          <View className='onboarding-header'>
            <Text className='h2' style={{ visibility: 'hidden' }}>完善资料</Text>
          </View>
          <View className='onboarding-body'>
            <View className='onboarding-step onboarding-choice-step'>
              <Text className='h1'>先完善一下资料</Text>
              <Text className='onboarding-lead p'>填好名字和头像，家人才好认出你留下的记录</Text>
              <View className='onboarding-center-card' />
              <View className='onboarding-choice-list'>
                <Button className='onboarding-choice' onClick={() => navigateTo(routes.pages.profileOnboarding)}>
                  <Text className='span'>
                    <Text className='strong'>完善资料</Text>
                    <Text className='span'>填写你的名字和头像，加入家庭一起记录。</Text>
                  </Text>
                  <Text className='i'>›</Text>
                </Button>
              </View>
              <Button className='onboarding-skip' onClick={enterPreview}>暂时跳过，先看看</Button>
            </View>
          </View>
        </View>
      </View>
    )
  }

  // ─── 门禁二：创建或加入家庭（文件二「欢迎加入」选择页视觉） ───
  if (identity === 'guest' && !previewMode) {
    return (
      <View className='app onboarding-root' id='app'>
        <View className='onboarding-screen' id='onboardingScreen'>
          <View className='onboarding-header'>
            <Text className='h2' style={{ visibility: 'hidden' }}>宠物家庭</Text>
          </View>
          <View className='onboarding-body'>
            <View className='onboarding-step onboarding-choice-step'>
              <Text className='h1'>欢迎加入</Text>
              <Text className='onboarding-lead p'>选择加入或者创建你的家庭吧</Text>
              <View className='onboarding-center-card' />
              <View className='onboarding-choice-list'>
                <Button className='onboarding-choice' onClick={() => navigateTo(routes.pages.createFamily)}>
                  <Text className='span'>
                    <Text className='strong'>创建家庭</Text>
                    <Text className='span'>建立一个新家庭，并邀请家人一起维护宠物档案。</Text>
                  </Text>
                  <Text className='i'>›</Text>
                </Button>
                <Button className='onboarding-choice' onClick={() => navigateTo(routes.pages.joinFamily)}>
                  <Text className='span'>
                    <Text className='strong'>加入家庭</Text>
                    <Text className='span'>使用家庭码申请加入已有家庭。</Text>
                  </Text>
                  <Text className='i'>›</Text>
                </Button>
              </View>
              <Button className='onboarding-skip' onClick={enterPreview}>暂时跳过</Button>
            </View>
          </View>
        </View>
      </View>
    )
  }

  const renderAvatar = (pet: PetWithAvatar | undefined, fallbackName: string) => {
    if (!pet) return ''
    if (pet._avatar) return null
    return pet._emoji || fallbackName.slice(0, 1)
  }

  // ─── 档案册主页 ───
  return (
    <View className='app' id='app'>
      <View className='topbar'>
        <Button className='icon-button' onClick={previewMode ? exitPreview : () => navigateTo(routes.pages.familyMembers)}>⌂</Button>
        <Button className='pet-switch' onClick={() => setPetOverlayOpen(true)}>
          <View className='avatar-placeholder'>
            {renderAvatar(activePet, activePet?.name || '')}
          </View>
          <Text className='strong'>{activePet?.name || '选择宠物'}</Text>
          <Text className='span'>⌄</Text>
        </Button>
        <View className='top-actions'>
          <Button className='icon-button' onClick={() => showToast('更多功能建设中')}>•••</Button>
        </View>
      </View>

      <View className='module-screen album-screen' id='albumModuleScreen'>
        <View className='module-header'>
          <View className='header-actions'>
            <Button className='module-pet-switch' onClick={() => setPetOverlayOpen(true)}>
              <View className='avatar-placeholder'>
                {renderAvatar(activePet, activePet?.name || '')}
              </View>
              <Text className='strong'>{activePet?.name || '选择宠物'}</Text>
              <Text className='span'>⌄</Text>
            </Button>
          </View>
        </View>

        {!petsLoaded ? (
          <View className='gate-in-screen'><Text className='gate-text'>正在打开档案册…</Text></View>
        ) : !activePet ? (
          <View className='empty-book'>
            <Text className='gate-emoji'>📖</Text>
            <Text className='empty-book-title'>还没有宠物档案</Text>
            <Text className='gate-sub'>添加第一只毛孩子，开始记录它的故事</Text>
            <Button className='primary-button' onClick={openAddPet}>添加宠物</Button>
          </View>
        ) : (
          <ManualPanel
            pet={activePet}
            page={bookPage}
            onTurn={setBookPage}
            onToast={showToast}
            onAddRecord={addRecord}
            onAddTodo={addTodo}
          />
        )}
      </View>

      {/* 宠物切换弹层 */}
      <View className={`overlay${petOverlayOpen ? ' open' : ''}`} id='petOverlay' onClick={() => setPetOverlayOpen(false)}>
        <View className='sheet' onClick={(e) => e.stopPropagation()}>
          <View className='handle' />
          <View className='sheet-head'>
            <View><Text className='h2'>{familyName}</Text><Text className='p'>选择要查看的宠物档案册</Text></View>
            <Button className='close-button' onClick={() => setPetOverlayOpen(false)}>×</Button>
          </View>
          <View className='pet-list' id='petList'>
            {pets.map(p => (
              <Button key={p.id} className={`pet-item${p.id === activePetId ? ' selected' : ''}`}
                onClick={() => choosePet(p.id)}>
                <View className={`avatar-placeholder${p._avatar ? ' has-photo' : ''}`}>
                  {p._avatar
                    ? <Image className='avatar-photo' src={p._avatar} mode='aspectFill' />
                    : (p._emoji || p.name.slice(0, 1))}
                </View>
                <View className='pet-name-row'>
                  <Text className='strong'>{p.name}</Text>
                  {p.id === activePetId && <Text className='i pet-check'>✓</Text>}
                </View>
                <Text className='span'>{speciesOf(p.type)}</Text>
              </Button>
            ))}
          </View>
          <Button className='add-pet' id='addPetButton' onClick={() => { setPetOverlayOpen(false); setTimeout(openAddPet, 220) }}>＋ 添加宠物</Button>
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

      <View className={`toast${toastMsg ? ' show' : ''}`} id='toast'>{toastMsg}</View>
    </View>
  )
}
