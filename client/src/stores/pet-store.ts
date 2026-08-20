 import { create } from 'zustand'

  interface PetStore {
    currentPetId: string | null
    setCurrentPetId: (petId: string) => void
    clearCurrentPet: () => void
  }

  export const usePetStore = create<PetStore>(set => ({
    currentPetId: null,

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
