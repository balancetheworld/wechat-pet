/**
 * API 统一类型定义
 * 对应接口文档 1.2 统一响应结构、1.3 统一分页结构、1.4 通用对象
 */

/** 统一响应结构 */
export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data: T | null
  request_id?: string
}

/** 统一分页结构 */
export interface PaginatedData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  has_more: boolean
}

/** 用户对象（简化） */
export interface ApiUser {
  id: string
  nickname: string
  avatar_asset_id?: number
  avatar_url?: string
  identity: 'guest' | 'member' | 'owner'
  family?: {
    id: string
    name: string
    role: 'member' | 'owner'
  } | null
  profile_completed?: boolean
}

/** 家庭摘要 */
export interface ApiFamilySummary {
  id: string
  name: string
  code: string
}

/** 宠物对象（列表接口仅返回 id/name，其余字段按文档预留） */
export interface ApiPet {
  id: string
  name: string
  species: string
  breed?: string
  gender: 'male' | 'female' | 'unknown'
  neutered: boolean
  birthday: string
  arrived_at: string
  avatar_asset_id?: string
  avatar_url?: string
  health_status?: 'healthy' | 'attention' | 'treatment'
  tags?: string[]
  quote?: string
  created_at?: string
  updated_at?: string
}

/** 创建/更新宠物请求体 */
export interface ApiPetSaveBody {
  name: string
  species: string
  breed?: string
  gender: 'male' | 'female' | 'unknown'
  neutered: boolean
  birthday: string
  arrived_at: string
  avatar_asset_id?: number
  health_status?: 'healthy' | 'attention' | 'treatment'
  tags?: string[]
  quote?: string
}

/** 个性问答 */
export interface ApiPersonalityQA {
  id?: string
  title: string
  summary: string
  detail: string
}

/** 健康摘要 */
export interface ApiHealthSummary {
  overall: string
  allergies: { label: string; value: string; note: string }
  diseases: { label: string; value: string; note: string }
  medications: { label: string; value: string; note: string }
  vaccines: { label: string; value: string; note: string }
}

/** 生日记录 */
export interface ApiBirthday {
  id?: string
  date: string
  age: string
  title: string
  wish: string
  media_label: string
}

/** 体重记录 */
export interface ApiWeight {
  id?: string
  date: string
  value: string
}

/** 成长事件 */
export interface ApiGrowthEvent {
  id?: string
  type: string
  date: string
  title: string
  content: string
  author: string
}

/** 成长足迹聚合 */
export interface ApiGrowth {
  weights: ApiWeight[]
  events: ApiGrowthEvent[]
}

/** 证件 */
export interface ApiDocument {
  id: string
  type: string
  title: string
  asset_id?: number
  asset_url?: string
}

/* ============================================================
 * 以下类型对齐文件三后端（server/internal/app/pet/profile.go）实际契约
 * —— migration 000006_pet_profiles + 泛化资源路由 /pets/:id/:resource
 * ============================================================ */

/** 创建宠物请求体（后端 CreatePetRequest 实际只收 name + avatar_asset_id） */
export interface ApiPetCreateBody {
  name: string
  avatar_asset_id?: string
}

/** 宠物富档案（GET/PATCH /pets/:id/profile，含后端派生字段） */
export interface ApiPetProfile {
  id: string
  name: string
  avatar_asset_id: string
  cover_asset_id: string
  breed: string
  gender: string
  sterilized: boolean
  birthday: string | null
  home_date: string | null
  age: number
  companion_days: number
  next_birthday_days: number | null
}

/** 档案 PATCH 白名单字段（后端 allowed 列表） */
export interface ApiPetProfilePatch {
  name?: string
  avatar_asset_id?: string
  cover_asset_id?: string
  breed?: string
  gender?: string
  sterilized?: boolean
  birthday?: string
  home_date?: string
}

/** 个性问答（pet_questions：question / answer） */
export interface ApiQuestionItem {
  id: string
  question: string
  answer: string
}

/** 性格特质（pet_personality：trait / value） */
export interface ApiTraitItem {
  id: string
  trait: string
  value: string
}

/** 健康档案（pet_health：GET /pets/:id/health） */
export interface ApiHealthRecord {
  status: string
  allergies: string
  long_term_medication: string
}

/** 疾病记录（pet_diseases） */
export interface ApiDiseaseItem {
  id: string
  name: string
  status: string
  details: string
}

/** 疫苗记录（pet_vaccines） */
export interface ApiVaccineItem {
  id: string
  name: string
  vaccinated_at: string
  details: string
}

/** 生日纪念（pet_birthday_records：year / age / summary） */
export interface ApiBirthdayRecord {
  id: string
  year: number
  age: number
  summary: string
}

/** 体重记录（pet_weights：measured_at / weight） */
export interface ApiWeightItem {
  id: string
  measured_at: string
  weight: number
}

/** 成长事件（pet_growth_events：type / occurred_at / recorder / content） */
export interface ApiGrowthEventItem {
  id: string
  type: string
  occurred_at: string
  recorder: string
  content: string
}

/** 证件（pet_certificates：type / name / number） */
export interface ApiCertificateItem {
  id: string
  type: string
  name: string
  number: string
}

/** 成长事件创建体 */
export interface ApiGrowthEventCreateBody {
  type: string
  occurred_at: string
  recorder: string
  content: string
}

/** 资产上传响应（POST /assets/upload） */
export interface ApiUploadResult {
  asset_id: string
}
