import type { Pet } from '../types/pet'
import { create } from 'zustand'

interface PetStore {
  pets: Pet[]
  currentPetId: string | null
  selectionMode: 'uninitialized' | 'automatic' | 'selected'
  setPets: (pets: Pet[]) => void
  setCurrentPetId: (petId: string) => void
  clearCurrentPet: () => void
}

export const usePetStore = create<PetStore>(set => ({
  pets: [],
  currentPetId: null,
  selectionMode: 'uninitialized',

  setPets(pets) {
    set((state) => {
      if (state.selectionMode === 'uninitialized') {
        return { pets, currentPetId: pets[0]?.id ?? null, selectionMode: pets.length > 0 ? 'selected' : 'uninitialized' }
      }
      if (state.selectionMode === 'selected' && state.currentPetId && !pets.some(pet => pet.id === state.currentPetId)) {
        return { pets, currentPetId: pets[0]?.id ?? null, selectionMode: pets.length > 0 ? 'selected' : 'uninitialized' }
      }
      return { pets }
    })
  },

  setCurrentPetId(petId) {
    set({
      currentPetId: petId,
      selectionMode: 'selected',
    })
  },

  clearCurrentPet() {
    set({
      currentPetId: null,
      selectionMode: 'automatic',
    })
  },
}))
