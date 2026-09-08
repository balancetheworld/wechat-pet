import { Button, Image, Input, Picker, Text, View } from '@tarojs/components'
import Taro, { useRouter } from '@tarojs/taro'
import { useEffect, useMemo, useState } from 'react'
import PageBackground from '../../../components/page-background'
import { createPet, getPet, getPetProfile, updatePet } from '../../../services/pet'
import { navigateBack } from '../../../utils/navigation'
import './index.scss'

/* ============ 选项常量(沿用 Pet-Manual 模式) ============ */
const SPECIES_OPTIONS = [
  { id: 'cat', label: '猫' },
  { id: 'dog', label: '狗' },
  { id: 'bird', label: '鸟' },
  { id: 'rabbit', label: '兔' },
  { id: 'fish', label: '鱼' },
  { id: 'other', label: '其他' },
]

const GENDER_OPTIONS = [
  { id: 'male', label: '男孩' },
  { id: 'female', label: '女孩' },
  { id: 'unknown', label: '未知' },
]

const NEUTERED_OPTIONS = [
  { id: 'yes', label: '已绝育' },
  { id: 'no', label: '未绝育' },
  { id: 'unknown', label: '未知' },
]

const HEALTH_OPTIONS = [
  { id: 'healthy', label: '健康' },
  { id: 'subhealthy', label: '亚健康' },
  { id: 'sick', label: '生病中' },
]

function todayString() {
  const d = new Date()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

export default function PetEdit() {
  const { params } = useRouter<{ petId?: string }>()
  const petID = params.petId
  const isEdit = Boolean(petID)

  /* --- 表单字段 --- */
  const [formName, setFormName] = useState('')
  const [formAvatar, setFormAvatar] = useState('')
  const [formSpecies, setFormSpecies] = useState('cat')
  const [formSpeciesOther, setFormSpeciesOther] = useState('')
  const [formGender, setFormGender] = useState('unknown')
  const [formNeutered, setFormNeutered] = useState('unknown')
  const [formBirth, setFormBirth] = useState('2024-01-01')
  const [formArrival, setFormArrival] = useState(() => todayString())
  const [formHealth, setFormHealth] = useState('healthy')
  const [formBreed, setFormBreed] = useState('')

  const [loading, setLoading] = useState(isEdit)
  const [submitting, setSubmitting] = useState(false)
  /* --- 输入框聚焦状态: placeholder 聚焦时直接隐藏 --- */
  const [focused, setFocused] = useState<Record<string, boolean>>({})
  const focusOn = (key: string) => () => {
    setFocused(f => ({ ...f, [key]: true }))
  }
  const focusOff = (key: string) => () => {
    setFocused(f => ({ ...f, [key]: false }))
  }
  const inputCls = (key: string) => `capsule-input${focused[key] ? ' focused' : ''}`
  const inputPh = (key: string, text: string) => (focused[key] ? '' : text)

  /* --- 编辑模式: 拉取并预填所有字段 --- */
  useEffect(() => {
    if (!petID) {
      return
    }
    const id = petID
    async function load() {
      try {
        const [pet, profile] = await Promise.all([getPet(id), getPetProfile(id).catch(() => null)])
        setFormName(pet.name)
        if (profile) {
          /* 把后端字段映射到表单字段 (兼容空值, 不强行覆盖默认值) */
          if (profile.breed) {
            setFormBreed(profile.breed)
          }
          if (profile.gender === 'male' || profile.gender === 'female' || profile.gender === 'unknown') {
            setFormGender(profile.gender)
          }
          if (profile.sterilized === true) {
            setFormNeutered('yes')
          }
          else if (profile.sterilized === false) {
            setFormNeutered('no')
          }
          if (profile.birthday) {
            setFormBirth(profile.birthday)
          }
          if (profile.home_date) {
            setFormArrival(profile.home_date)
          }
        }
        /* 优先用本地缓存的额外数据 (物种/头像/健康状态) 覆盖, 因为这些字段后端暂未持久化 */
        try {
          const cache = await Taro.getStorage({ key: `pet-extra-${pet.name}` })
          const data = cache.data as {
            avatar?: string
            species?: string
            speciesOther?: string
            gender?: string
            neutered?: string
            birth?: string
            arrival?: string
            health?: string
            breed?: string
          } | undefined
          if (data) {
            if (data.avatar) {
              setFormAvatar(data.avatar)
            }
            if (data.species) {
              setFormSpecies(data.species)
            }
            if (data.speciesOther) {
              setFormSpeciesOther(data.speciesOther)
            }
            if (data.gender) {
              setFormGender(data.gender)
            }
            if (data.neutered) {
              setFormNeutered(data.neutered)
            }
            if (data.birth) {
              setFormBirth(data.birth)
            }
            if (data.arrival) {
              setFormArrival(data.arrival)
            }
            if (data.health) {
              setFormHealth(data.health)
            }
            if (data.breed) {
              setFormBreed(data.breed)
            }
          }
        }
        catch {
          /* 没有缓存时静默忽略 */
        }
      }
      catch (error) {
        const message = error instanceof Error ? error.message : '加载宠物失败'
        await Taro.showToast({ title: message, icon: 'none' })
      }
      finally {
        setLoading(false)
      }
    }
    void load()
  }, [petID])

  /* --- 选择头像 --- */
  async function chooseAvatar() {
    try {
      const res = await Taro.chooseImage({
        count: 1,
        sizeType: ['compressed'],
        sourceType: ['album', 'camera'],
      })
      const path = res.tempFilePaths?.[0]
      if (path) {
        setFormAvatar(path)
      }
    }
    catch {
      // 用户取消或权限拒绝, 静默
    }
  }

  /* --- 提交 --- */
  async function handleSubmit() {
    const value = formName.trim()
    if (value.length < 1 || value.length > 12) {
      await Taro.showToast({ title: '名字长度需为 1-12 个字符', icon: 'none' })
      return
    }
    if (submitting || loading) {
      return
    }
    setSubmitting(true)
    try {
      // 后端目前只接收 name, 其他字段先本地缓存
      const extraData = {
        avatar: formAvatar,
        species: formSpecies,
        speciesOther: formSpeciesOther,
        gender: formGender,
        neutered: formNeutered,
        birth: formBirth,
        arrival: formArrival,
        health: formHealth,
        breed: formBreed,
      }
      try {
        await Taro.setStorage({ key: `pet-extra-${value}`, data: extraData })
      }
      catch {
        /* 缓存失败不影响主流程 */
      }

      if (petID) {
        await updatePet(petID, { name: value })
      }
      else {
        await createPet({ name: value })
      }
      await Taro.showToast({ title: isEdit ? '已保存' : '已添加到家庭', icon: 'success' })
      await navigateBack()
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '保存失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setSubmitting(false)
    }
  }

  /* --- 出生日期不能晚于今天 --- */
  const maxDate = useMemo(() => todayString(), [])

  return (
    <View className="themed-page pet-edit-page">
      <PageBackground />

      {/* 标题区 */}
      <View className="pet-edit-header">
        <Text className="h2">{isEdit ? '编辑宠物' : '添加宠物'}</Text>
        <Text className="p">{isEdit ? '更新档案信息' : '填写档案信息后加入家庭'}</Text>
      </View>

      <View className="add-pet-form">
        {/* 头像 */}
        <View className="form-field">
          <Text className="label">头像</Text>
          <View className="avatar-picker">
            <View className={`avatar-preview${formAvatar ? ' has-photo' : ''}`}>
              {formAvatar && <Image className="avatar-photo" src={formAvatar} mode="aspectFill" />}
            </View>
            <Button className="avatar-pick-button" onClick={chooseAvatar}>从相册选择</Button>
          </View>
        </View>

        {/* 名字 */}
        <View className="form-field">
          <Text className="label">名字 *</Text>
          <Input
            className={inputCls('name')}
            maxlength={12}
            placeholder={inputPh('name', '给宠物起个名字')}
            value={formName}
            onInput={event => setFormName(event.detail.value)}
            onFocus={focusOn('name')}
            onBlur={focusOff('name')}
          />
        </View>

        {/* 物种 */}
        <View className="form-field">
          <Text className="label">物种</Text>
          <View className="chip-group">
            {SPECIES_OPTIONS.map(s => (
              <Button
                key={s.id}
                className={`chip${formSpecies === s.id ? ' selected' : ''}`}
                onClick={() => setFormSpecies(s.id)}
              >
                <Text className="span">{s.label}</Text>
              </Button>
            ))}
          </View>
          {formSpecies === 'other' && (
            <Input
              className={inputCls('speciesOther')}
              maxlength={8}
              placeholder={inputPh('speciesOther', '填写物种，如：刺猬')}
              value={formSpeciesOther}
              onInput={event => setFormSpeciesOther(event.detail.value)}
              onFocus={focusOn('speciesOther')}
              onBlur={focusOff('speciesOther')}
            />
          )}
        </View>

        {/* 品种 */}
        <View className="form-field">
          <Text className="label">品种（可选）</Text>
          <Input
            className={inputCls('breed')}
            maxlength={20}
            placeholder={inputPh('breed', '如：英国短毛猫')}
            value={formBreed}
            onInput={event => setFormBreed(event.detail.value)}
            onFocus={focusOn('breed')}
            onBlur={focusOff('breed')}
          />
        </View>

        {/* 性别 */}
        <View className="form-field">
          <Text className="label">性别</Text>
          <View className="chip-group chip-group-tight">
            {GENDER_OPTIONS.map(g => (
              <Button
                key={g.id}
                className={`chip chip-sm${formGender === g.id ? ' selected' : ''}`}
                onClick={() => setFormGender(g.id)}
              >
                <Text className="span">{g.label}</Text>
              </Button>
            ))}
          </View>
        </View>

        {/* 绝育 */}
        <View className="form-field">
          <Text className="label">绝育</Text>
          <View className="chip-group chip-group-tight">
            {NEUTERED_OPTIONS.map(n => (
              <Button
                key={n.id}
                className={`chip chip-sm${formNeutered === n.id ? ' selected' : ''}`}
                onClick={() => setFormNeutered(n.id)}
              >
                <Text className="span">{n.label}</Text>
              </Button>
            ))}
          </View>
        </View>

        {/* 出生 / 到家 */}
        <View className="form-row">
          <View className="form-field">
            <Text className="label">出生日期</Text>
            <Picker
              mode="date"
              value={formBirth}
              end={maxDate}
              onChange={event => setFormBirth(event.detail.value)}
            >
              <View className="capsule-input date-view">{formBirth || '选择日期'}</View>
            </Picker>
          </View>
          <View className="form-field">
            <Text className="label">到家日期</Text>
            <Picker
              mode="date"
              value={formArrival}
              end={maxDate}
              onChange={event => setFormArrival(event.detail.value)}
            >
              <View className="capsule-input date-view">{formArrival || '选择日期'}</View>
            </Picker>
          </View>
        </View>

        {/* 健康 */}
        <View className="form-field">
          <Text className="label">健康状态</Text>
          <View className="chip-group">
            {HEALTH_OPTIONS.map(h => (
              <Button
                key={h.id}
                className={`chip${formHealth === h.id ? ' selected' : ''}`}
                onClick={() => setFormHealth(h.id)}
              >
                <Text className="span">{h.label}</Text>
              </Button>
            ))}
          </View>
        </View>

        {/* 操作 */}
        <View className="add-pet-form-actions">
          <Button className="secondary-button" onClick={() => { void navigateBack() }}>取消</Button>
          <Button
            className="primary-button"
            disabled={!formName.trim() || submitting || loading}
            onClick={handleSubmit}
          >
            {isEdit ? '保存修改' : '添加到家庭'}
          </Button>
        </View>
      </View>
    </View>
  )
}
