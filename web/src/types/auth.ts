import type { FamilyRole, FamilySummary } from './family'

export type Identity = 'guest' | 'member' | 'owner'

export interface User {
  id: string
  nickname: string
  avatarUrl?: string
}

// 身份认证会话,户登录之后保存的整套登录信息。
export interface AuthSession {
  token: string
  user: User
  identity: Identity
  family: FamilySummary | null
  familyRole: FamilyRole | null
}

export interface MeResponseDTO {
  user: {
    id: string
    nickname: string
    avatar: string
    profile_completed: boolean
  }
  family: FamilySummary | null
  identity: Identity
}
