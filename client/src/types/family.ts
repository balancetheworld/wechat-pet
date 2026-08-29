export interface FamilySummary {
  id: string
  name: string
}

export type FamilyRole = 'member' | 'owner'

export interface FamilyDetail extends FamilySummary {
  join_code: string
  role: FamilyRole
}

export interface FamilyMember {
  id: string
  user_id: string
  nickname: string
  avatar: string
  role: FamilyRole
  status: 'active'
}

export interface JoinApplication {
  id: string
  family_id: string
  family_name: string
  user_id: string
  nickname: string
  avatar: string
  status: 'pending' | 'active' | 'rejected'
}

export interface CreateFamilyRequest {
  name: string
}

export interface JoinFamilyRequest {
  code: string
}
