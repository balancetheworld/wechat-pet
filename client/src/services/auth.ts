import type { AuthSession } from '../types/auth'
import Taro from '@tarojs/taro'
import { useAuthStore } from '../stores/auth-store'
import { request } from './request'

async function loginRequest() {
  const result = await Taro.login()

  if (!result.code) {
    throw new Error('微信登录未返回 code')
  }

  return request<AuthSession>({
    path: '/api/v1/auth/login',
    method: 'POST',
    skipAuthHeader: true,
    skipAuthRefresh: true,
    data: {
      code: result.code,
    },
  })
}

export function login() {
  return loginRequest()
}

export async function silentLogin() {
  const session = await loginRequest()
  await useAuthStore.getState().setSession(session)
  return session
}
