/* 前端展示层的数据结构定义（与 services/types.ts 的 API 结构通过 mappers.ts 转换） */

export interface AskPreset {
  q: string
  a: string
}

export interface CalRecord {
  date: string
  pet: string
  cat: 'medical' | 'daily'
  medType: string | null
  title: string
  text: string
  imgs: string[]
  by: string
  time: string
}

export interface CalTodo {
  id: string
  pet: string
  title: string
  remindDate: string
  cycle: 'once' | 'quarterly' | 'yearly'
  cycleName: string
  status: 'pending' | 'completed'
  by: string
}

export interface PetRecord {
  id: string
  name: string
  title: string
  quote: string
  years: string
  type: string
  birthDate: string
  arrivalDate: string
  tags: string[]
  personalityQuestions: { title: string; summary: string; detail: string }[]
  health: {
    overall: string
    allergies: { label: string; value: string; note: string }
    diseases: { label: string; value: string; note: string }
    medications: { label: string; value: string; note: string }
    vaccines: { label: string; value: string; note: string }
  }
  updated: string
  backFamily: string
  weights: { date: string; value: string }[]
  events: { type: string; date: string; title: string; content: string; author: string }[]
  birthdays: { date: string; age: string; title: string; wish: string; mediaLabel: string }[]
}

export type FamilyRole = '主人' | '管理员' | '成员'
export type FamilyMemberStatus = 'active' | 'pending' | 'rejected'

export interface FamilyMember {
  name: string
  role: FamilyRole
  status: FamilyMemberStatus
  appliedAt: string
  reviewedAt: string
}

export interface FamilyData {
  name: string
  code: string
  members: FamilyMember[]
  pets: { id: string; name: string }[]
}
