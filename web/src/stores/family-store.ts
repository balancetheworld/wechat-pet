import type { FamilyDetail, FamilyRole, FamilySummary } from '../types/family'
import { create } from 'zustand'

interface FamilyStore {
  family: FamilySummary | null
  detail: FamilyDetail | null
  role: FamilyRole | null
  setFamily: (family: FamilySummary, role: FamilyRole) => void
  setFamilyDetail: (family: FamilyDetail) => void
  clearFamily: () => void
}

export const useFamilyStore = create<FamilyStore>(set => ({
  family: null,
  detail: null,
  role: null,

  setFamily(family, role) {
    set({
      family,
      detail: null,
      role,
    })
  },

  setFamilyDetail(family) {
    set({
      family: {
        id: family.id,
        name: family.name,
      },
      detail: family,
      role: family.role,
    })
  },

  clearFamily() {
    set({
      family: null,
      detail: null,
      role: null,
    })
  },
}))
