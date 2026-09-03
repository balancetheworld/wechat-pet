/**
 * 宠物档案模块 API
 *
 * 对齐文件三后端（server/internal/httpapi/pet/routes.go）实际路由：
 * - 基础：  POST /pets · GET /pets（返回数组）· GET/PATCH/DELETE /pets/{id}
 * - 富档案：GET/PATCH /pets/{id}/profile（breed/gender/sterilized/birthday/home_date/avatar）
 * - 派生：  GET /pets/{id}/dates（age/companion_days/next_birthday_days）
 * - 泛化资源路由 /pets/{id}/{resource}：
 *   questions（question/answer）· personality（trait/value）·
 *   health（status/allergies/long_term_medication，PUT 更新）·
 *   diseases · vaccines · certificates · birthday-records ·
 *   weights（measured_at/weight）· growth-events（type/occurred_at/recorder/content）
 *
 * 规则：
 * - family_id 来自 RequireFamily 中间件，不放 body
 * - 所有 detail endpoint 检查 pet 属于当前 family
 * - delete 使用逻辑删除
 * - 附件先上传取得 asset_id（services/asset.ts），再随 body 提交
 */

import { request } from './request'
import type {
  ApiPet,
  ApiPetCreateBody,
  ApiPetProfile,
  ApiPetProfilePatch,
  ApiPetSaveBody,
  ApiQuestionItem,
  ApiTraitItem,
  ApiHealthRecord,
  ApiDiseaseItem,
  ApiVaccineItem,
  ApiBirthdayRecord,
  ApiWeightItem,
  ApiGrowthEventItem,
  ApiGrowthEventCreateBody,
  ApiCertificateItem,
  PaginatedData,
} from './types'

/** 生成 UUID（用于 Idempotency-Key） */
function uuid(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) return crypto.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

// ─── 宠物基础 ───

/**
 * 获取家庭宠物列表
 * GET /pets —— 后端 List 直接返回数组（非分页结构），此处做双结构兼容
 */
export async function getPets(): Promise<ApiPet[]> {
  const data = await request<ApiPet[] | PaginatedData<ApiPet>>('GET', '/pets')
  if (Array.isArray(data)) return data
  return (data && data.items) || []
}

/**
 * 获取宠物详情
 * GET /pets/{pet_id}
 */
export async function getPetDetail(petId: string): Promise<ApiPet> {
  return request<ApiPet>('GET', `/pets/${petId}`)
}

/**
 * 创建宠物
 * POST /pets —— 后端 CreatePetRequest 实际只收 name + avatar_asset_id，
 * 富字段（品种/性别/绝育/生日/到家日）在创建后走 updatePetProfile 补写
 */
export async function createPet(body: ApiPetCreateBody): Promise<ApiPet> {
  return request<ApiPet>('POST', '/pets', body as unknown as Record<string, unknown>, {
    idempotencyKey: uuid(),
  })
}

/**
 * 更新宠物（仅 name）
 * PATCH /pets/{pet_id}
 */
export async function updatePet(petId: string, body: { name: string }): Promise<ApiPet> {
  return request<ApiPet>('PATCH', `/pets/${petId}`, body as unknown as Record<string, unknown>)
}

/**
 * 删除宠物（逻辑删除）
 * DELETE /pets/{pet_id}
 */
export async function deletePet(petId: string): Promise<void> {
  await request<null>('DELETE', `/pets/${petId}`)
}

// ─── 富档案（migration 000006_pet_profiles）───

/**
 * 获取宠物富档案（含后端派生的 age / companion_days / next_birthday_days）
 * GET /pets/{pet_id}/profile
 */
export async function getPetProfile(petId: string): Promise<ApiPetProfile> {
  return request<ApiPetProfile>('GET', `/pets/${petId}/profile`)
}

/**
 * 更新宠物富档案（白名单字段见 ApiPetProfilePatch）
 * PATCH /pets/{pet_id}/profile —— 返回更新后的完整档案
 */
export async function updatePetProfile(petId: string, body: ApiPetProfilePatch): Promise<ApiPetProfile> {
  return request<ApiPetProfile>('PATCH', `/pets/${petId}/profile`, body as unknown as Record<string, unknown>)
}

/**
 * 获取派生日期信息（生日/到家/年龄/陪伴天数/下次生日倒计时）
 * GET /pets/{pet_id}/dates
 */
export async function getPetDates(petId: string): Promise<{
  birthday: string | null
  home_date: string | null
  age: number
  companion_days: number
  next_birthday_days: number | null
}> {
  return request('GET', `/pets/${petId}/dates`)
}

// ─── 泛化档案资源（/pets/{pet_id}/{resource}）───

/** 个性问答列表 GET /pets/{id}/questions */
export async function getPetQuestions(petId: string): Promise<ApiQuestionItem[]> {
  return request<ApiQuestionItem[]>('GET', `/pets/${petId}/questions`)
}

/** 性格特质列表 GET /pets/{id}/personality */
export async function getPetTraits(petId: string): Promise<ApiTraitItem[]> {
  return request<ApiTraitItem[]>('GET', `/pets/${petId}/personality`)
}

/** 健康档案 GET /pets/{id}/health */
export async function getPetHealth(petId: string): Promise<ApiHealthRecord> {
  return request<ApiHealthRecord>('GET', `/pets/${petId}/health`)
}

/** 更新健康档案 PUT /pets/{id}/health */
export async function updatePetHealth(petId: string, body: ApiHealthRecord): Promise<ApiHealthRecord> {
  return request<ApiHealthRecord>('PUT', `/pets/${petId}/health`, body as unknown as Record<string, unknown>)
}

/** 疾病记录列表 GET /pets/{id}/diseases */
export async function getPetDiseases(petId: string): Promise<ApiDiseaseItem[]> {
  return request<ApiDiseaseItem[]>('GET', `/pets/${petId}/diseases`)
}

/** 疫苗记录列表 GET /pets/{id}/vaccines */
export async function getPetVaccines(petId: string): Promise<ApiVaccineItem[]> {
  return request<ApiVaccineItem[]>('GET', `/pets/${petId}/vaccines`)
}

/** 证件列表 GET /pets/{id}/certificates */
export async function getPetCertificates(petId: string): Promise<ApiCertificateItem[]> {
  return request<ApiCertificateItem[]>('GET', `/pets/${petId}/certificates`)
}

/** 生日纪念列表 GET /pets/{id}/birthday-records */
export async function getPetBirthdayRecords(petId: string): Promise<ApiBirthdayRecord[]> {
  return request<ApiBirthdayRecord[]>('GET', `/pets/${petId}/birthday-records`)
}

/** 体重记录列表 GET /pets/{id}/weights */
export async function getPetWeights(petId: string): Promise<ApiWeightItem[]> {
  return request<ApiWeightItem[]>('GET', `/pets/${petId}/weights`)
}

/** 成长事件列表 GET /pets/{id}/growth-events */
export async function getPetGrowthEvents(petId: string): Promise<ApiGrowthEventItem[]> {
  return request<ApiGrowthEventItem[]>('GET', `/pets/${petId}/growth-events`)
}

/** 创建成长事件 POST /pets/{id}/growth-events */
export async function createGrowthEvent(petId: string, body: ApiGrowthEventCreateBody): Promise<ApiGrowthEventItem> {
  return request<ApiGrowthEventItem>('POST', `/pets/${petId}/growth-events`, body as unknown as Record<string, unknown>)
}

export type { ApiPetSaveBody }
