import type { Pet } from '../types/pet'
import { create } from 'zustand'

interface PetStore {
  pets: Pet[]
  currentPetId: string | null
  setPets: (pets: Pet[]) => void
  setCurrentPetId: (petId: string) => void
  clearCurrentPet: () => void
}

export const usePetStore = create<PetStore>(set => ({
  pets: [],
  currentPetId: null,

  setPets(pets) {
    set({ pets })
  },

  setCurrentPetId(petId) {
    set({
      currentPetId: petId,
    })
  },

  clearCurrentPet() {
    set({
      currentPetId: null,
    })
  },
}))
