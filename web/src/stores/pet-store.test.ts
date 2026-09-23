import { beforeEach, expect, it } from 'vitest'
import { usePetStore } from './pet-store'

beforeEach(() => {
  usePetStore.setState({ pets: [], currentPetId: null, selectionMode: 'uninitialized' })
})

it('keeps automatic pet selection after pets are refreshed', () => {
  usePetStore.getState().clearCurrentPet()
  usePetStore.getState().setPets([{ id: 'pet-1', name: '旺仔' }])

  const state = usePetStore.getState()
  expect(state.currentPetId).toBeNull()
  expect(state.selectionMode).toBe('automatic')
})
