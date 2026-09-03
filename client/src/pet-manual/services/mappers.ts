/**
 * API 数据 ↔ 前端 PetRecord 转换器
 *
 * 对齐文件三后端（profile.go / migration 000006）实际字段：
 * - 富档案：ApiPetProfile（breed/gender/sterilized/birthday/home_date）
 * - 个性问答：pet_questions（question/answer）
 * - 性格特质：pet_personality（trait/value）
 * - 健康：pet_health + pet_diseases + pet_vaccines
 * - 生日：pet_birthday_records（year/age/summary）
 * - 成长：pet_weights（measured_at/weight）+ pet_growth_events
 *
 * 后端暂无 species / health_status / tags / quote 列，
 * 这些字段由前端本地维护（表单/展示层），不落库 —— 见 docs/pet-manual-merge-log.md。
 */

import type {
  ApiPetProfile,
  ApiQuestionItem,
  ApiTraitItem,
  ApiHealthRecord,
  ApiDiseaseItem,
  ApiVaccineItem,
  ApiBirthdayRecord,
  ApiWeightItem,
  ApiGrowthEventItem,
  ApiPetCreateBody,
  ApiPetProfilePatch,
  ApiPetSaveBody,
} from './types'
import type { PetRecord } from '../pages/index/data'

const GENDER_LABEL: Record<string, string> = { male: '公', female: '母', unknown: '未知', '公': '公', '母': '母' }

/** 生成今天日期字符串 YYYY-MM-DD */
function todayStr(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** 本地展示补充字段（后端无对应列，仅前端展示用） */
export interface PetRecordExtras {
  species?: string
  tags?: string[]
  quote?: string
  healthStatus?: 'healthy' | 'attention' | 'treatment'
}

/**
 * ApiPetProfile → PetRecord
 * 缺失字段用默认值填充（新宠物无历史数据）
 */
export function apiProfileToRecord(profile: ApiPetProfile, extras?: PetRecordExtras): PetRecord {
  const genderLabel = GENDER_LABEL[profile.gender] || '未知'
  const neuteredLabel = profile.sterilized ? '已绝育' : '未绝育'
  const genderLine = (profile.gender === 'unknown' || !profile.gender) ? '性别未知' : `${genderLabel} · ${neuteredLabel}`

  const typeLines = [extras?.species || profile.breed, genderLine].filter(Boolean)

  return {
    id: profile.id,
    name: profile.name,
    title: `${profile.name}的成长小册`,
    quote: extras?.quote || `${profile.name}，是我们家的小宝贝。`,
    years: profile.birthday ? `${profile.birthday.slice(0, 4)} — 至今` : '未知 — 至今',
    type: typeLines.join('<br>'),
    birthDate: profile.birthday || '',
    arrivalDate: profile.home_date || todayStr(),
    tags: (extras?.tags && extras.tags.length) ? extras.tags : ['新成员'],
    personalityQuestions: [],
    health: {
      overall: extras?.healthStatus === 'healthy' ? '健康'
        : extras?.healthStatus === 'attention' ? '需关注'
        : extras?.healthStatus === 'treatment' ? '治疗中'
        : '未记录',
      allergies: { label: '过敏信息', value: '未记录', note: '入住后补充' },
      diseases: { label: '既往疾病', value: '未记录', note: '入住后补充' },
      medications: { label: '长期用药', value: '未记录', note: '入住后补充' },
      vaccines: { label: '最近疫苗', value: '未记录', note: '入住后补充' },
    },
    updated: `更新于 ${todayStr()}`,
    backFamily: `更新于 ${todayStr()}`,
    weights: [],
    events: [],
    birthdays: [],
  }
}

/** 个性问答（pet_questions）→ 说明书列表 */
export function questionsToPersonality(items: ApiQuestionItem[]): PetRecord['personalityQuestions'] {
  return items.map(q => ({
    title: q.question,
    summary: q.answer.length > 16 ? `${q.answer.slice(0, 16)}…` : q.answer,
    detail: q.answer,
  }))
}

/** 性格特质（pet_personality）→ 标签（trait 即标签名） */
export function traitsToTags(items: ApiTraitItem[]): string[] {
  return items.map(t => t.trait).filter(Boolean)
}

/** 健康档案（pet_health + diseases + vaccines）→ 健康资料页结构 */
export function healthRecordToPetHealth(
  health: ApiHealthRecord,
  diseases: ApiDiseaseItem[],
  vaccines: ApiVaccineItem[],
): PetRecord['health'] {
  const latestVaccine = [...vaccines].sort((a, b) => b.vaccinated_at.localeCompare(a.vaccinated_at))[0]
  return {
    overall: health.status || '未记录',
    allergies: {
      label: '过敏信息',
      value: health.allergies || '未记录',
      note: health.allergies ? '来自健康档案' : '入住后补充',
    },
    diseases: {
      label: '既往疾病',
      value: diseases.length ? `${diseases.length} 条记录` : '未记录',
      note: diseases.length ? diseases[0].name : '入住后补充',
    },
    medications: {
      label: '长期用药',
      value: health.long_term_medication || '无',
      note: health.long_term_medication ? '来自健康档案' : '目前未使用',
    },
    vaccines: {
      label: '最近疫苗',
      value: latestVaccine ? latestVaccine.name : '未记录',
      note: latestVaccine ? latestVaccine.vaccinated_at : '入住后补充',
    },
  }
}

/** 生日纪念（pet_birthday_records）→ 生日纪念册结构
 *  后端仅 year/age/summary 三列：date=年份、title=summary，
 *  wish/mediaLabel 无对应字段，用固定文案占位 */
export function birthdayRecordsToBirthdays(items: ApiBirthdayRecord[]): PetRecord['birthdays'] {
  return items.map(r => ({
    date: String(r.year),
    age: `${r.age}岁`,
    title: r.summary || `${r.age}岁生日`,
    wish: '生日快乐，继续健康长大',
    mediaLabel: '🎂 生日纪念',
  }))
}

/** 体重记录（pet_weights）→ 成长足迹体重结构（按日期倒序，[0] 为最新）
 *  后端 weight 为 float32 列（4.2 会存成 4.1999998…），展示前四舍五入到 2 位小数 */
export function weightItemsToWeights(items: ApiWeightItem[]): PetRecord['weights'] {
  return items
    .map(w => ({ date: w.measured_at, value: String(Math.round(w.weight * 100) / 100) }))
    .sort((a, b) => b.date.localeCompare(a.date))
}

/** 成长事件（pet_growth_events）→ 成长足迹时间线结构 */
export function growthItemsToEvents(items: ApiGrowthEventItem[]): PetRecord['events'] {
  return items.map(e => ({
    type: e.type,
    date: e.occurred_at,
    title: e.content.length > 14 ? `${e.content.slice(0, 14)}…` : e.content,
    content: e.content,
    author: e.recorder,
  }))
}

/**
 * 前端表单 → 创建宠物请求体（后端只收 name + avatar_asset_id）
 */
export function formToCreatePetBody(
  form: { name: string },
  avatarAssetId?: string,
): ApiPetCreateBody {
  return {
    name: form.name.trim(),
    avatar_asset_id: avatarAssetId || undefined,
  }
}

/**
 * 前端表单 → 富档案 PATCH 体（breed/gender/sterilized/birthday/home_date）
 */
export function formToProfilePatch(form: {
  breed: string
  genderId: string
  neuteredId: string
  birthDate: string
  arrivalDate: string
}): ApiPetProfilePatch {
  const patch: ApiPetProfilePatch = {}
  if (form.breed.trim()) patch.breed = form.breed.trim()
  if (form.genderId) patch.gender = form.genderId
  patch.sterilized = form.neuteredId === 'yes'
  if (form.birthDate) patch.birthday = form.birthDate
  if (form.arrivalDate) patch.home_date = form.arrivalDate
  return patch
}

/**
 * @deprecated 旧接口文档版表单体（后端 CreatePetRequest 已确认只收 name + avatar_asset_id）。
 * 保留仅供退役的 pet-manual 单页入口回挂时参考，勿在新代码中使用。
 */
export function formToApiPetBody(form: {
  name: string
  speciesId: string
  speciesOther: string
  breed: string
  genderId: string
  neuteredId: string
  birthDate: string
  arrivalDate: string
  healthId: string
  tagsText: string
  quote: string
}): ApiPetSaveBody {
  const SPECIES_MAP: Record<string, string> = {
    cat: '猫', dog: '狗', rabbit: '兔', hamster: '仓鼠', bird: '鸟', other: '',
  }
  const species = form.speciesId === 'other'
    ? (form.speciesOther.trim() || '其他')
    : (SPECIES_MAP[form.speciesId] || '其他')

  return {
    name: form.name.trim(),
    species,
    breed: form.breed.trim() || undefined,
    gender: form.genderId as 'male' | 'female' | 'unknown',
    neutered: form.neuteredId === 'yes',
    birthday: form.birthDate,
    arrived_at: form.arrivalDate,
    health_status: form.healthId as 'healthy' | 'attention' | 'treatment',
    tags: form.tagsText.split(/[，,\s]+/).map(t => t.trim()).filter(Boolean).slice(0, 8),
    quote: form.quote.trim() || undefined,
  }
}
