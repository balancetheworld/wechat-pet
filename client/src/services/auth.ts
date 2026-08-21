import type { AuthSession, Identity, User } from '../types/auth'
import type { FamilyRole, FamilySummary } from '../types/family'
import Taro from '@tarojs/taro'
import { useAuthStore } from '../stores/auth-store'
import { request } from './request'

interface LoginResponseDTO {
  token: string
  user: {
    id: string
    nickname: string
    avatar: string
    profile_completed: boolean
  }
  family: FamilySummary | null
  identity: Identity
}

function toSession(value: LoginResponseDTO): AuthSession {
  const user: User = {
    id: value.user.id,
    nickname: value.user.nickname,
    avatarUrl: value.user.avatar || undefined,
  }
  const familyRole: FamilyRole | null = value.family && (value.identity === 'member' || value.identity === 'owner')
    ? value.identity
    : null
  return {
    token: value.token,
    user,
    identity: value.identity,
    family: value.family,
    familyRole,
  }
}

export async function loginByWeChat(code: string) {
  if (!code.trim()) {
    throw new Error('微信登录未返回 code')
  }

  const response = await request<LoginResponseDTO>({
    path: '/api/v1/auth/login',
    method: 'POST',
    skipAuthHeader: true,
    skipAuthRefresh: true,
    data: {
      code,
    },
  })
  const session = toSession(response)
  await useAuthStore.getState().setSession(session)
  return session
}

export async function silentLogin() {
  const result = await Taro.login()
  if (!result.code) {
    throw new Error('微信登录未返回 code')
  }
  return loginByWeChat(result.code)
}
