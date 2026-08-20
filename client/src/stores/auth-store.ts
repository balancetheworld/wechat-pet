import type { AuthSession, Identity, User } from '../types/auth'
import Taro from '@tarojs/taro'
import { create } from 'zustand'

interface AuthStore {
  token: string
  user: User | null
  identity: Identity
  setSession: (session: AuthSession) => Promise<void>
  clearSession: () => Promise<void>
  hydrate: () => Promise<void>
}

const storageKey = 'auth_token'

export const useAuthStore = create<AuthStore>(set => ({
  token: '',
  user: null,
  identity: 'guest',

  async setSession(session) {
    set({
      token: session.token,
      user: session.user,
      identity: session.identity,
    })

    await Taro.setStorage({
      key: storageKey,
      data: session.token,
    })
  },

  async clearSession() {
    set({
      token: '',
      user: null,
      identity: 'guest',
    })

    await Taro.removeStorage({
      key: storageKey,
    })
  },

  async hydrate() {
    try {
      const result = await Taro.getStorage<string>({
        key: storageKey,
      })

      set({
        token: result.data,
      })
    }
    catch {
      set({
        token: '',
      })
    }
  },
}))
