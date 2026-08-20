import type { FamilyRole, FamilySummary } from '../types/family'
import { create } from 'zustand'

interface FamilyStore {
  family: FamilySummary | null
  role: FamilyRole | null
  setFamily: (family: FamilySummary, role: FamilyRole) => void
  clearFamily: () => void
}

export const useFamilyStore = create<FamilyStore>(set => ({
  family: null,
  role: null,

  setFamily(family, role) {
    set({
      family,
      role,
    })
  },

  clearFamily() {
    set({
      family: null,
      role: null,
    })
  },
}))
