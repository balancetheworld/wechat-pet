import type { AuthSession, Identity, User } from '../types/auth'
import type { FamilyRole, FamilySummary } from '../types/family'
import Taro from '@tarojs/taro'
import { create } from 'zustand'
import { queryClient } from '../services/query-client'
import { useAskStore } from './ask-store'
import { useFamilyStore } from './family-store'

interface AuthStore {
  token: string
  user: User | null
  identity: Identity
  family: FamilySummary | null
  familyRole: FamilyRole | null
  setUserProfile: (user: User, family: FamilySummary | null, identity: Identity) => void
  setSession: (session: AuthSession) => Promise<void>
  clearSession: () => Promise<void>
  hydrate: () => Promise<void>
}

const storageKey = 'auth_token'

export const useAuthStore = create<AuthStore>((set, get) => ({
  token: '',
  user: null,
  identity: 'guest',
  family: null,
  familyRole: null,

  setUserProfile(user, family, identity) {
    const familyRole: FamilyRole | null = family && (identity === 'member' || identity === 'owner') ? identity : null
    set({
      user,
      family,
      familyRole,
      identity,
    })
    if (family && familyRole) {
      useFamilyStore.getState().setFamily(family, familyRole)
    }
    else {
      useFamilyStore.getState().clearFamily()
    }
  },

  async setSession(session) {
    const current = get()
    if (current.user?.id !== session.user.id || current.family?.id !== session.family?.id) {
      queryClient.clear()
      useAskStore.getState().reset()
    }
    set({
      token: session.token,
      user: session.user,
      identity: session.identity,
      family: session.family,
      familyRole: session.familyRole,
    })

    if (session.family && session.familyRole) {
      useFamilyStore.getState().setFamily(session.family, session.familyRole)
    }
    else {
      useFamilyStore.getState().clearFamily()
    }

    await Taro.setStorage({
      key: storageKey,
      data: session.token,
    })
  },

  async clearSession() {
    queryClient.clear()
    useAskStore.getState().reset()
    set({
      token: '',
      user: null,
      identity: 'guest',
      family: null,
      familyRole: null,
    })
    useFamilyStore.getState().clearFamily()

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
