import { create } from 'zustand'

interface AskStore {
  draft: string
  activeSessionId: string | null
  activeRunId: string | null
  activeRunVersion: number
  lastEventSequence: number
  setDraft: (draft: string) => void
  setActiveExecution: (sessionID: string, runID: string, runVersion: number, lastEventSequence: number) => void
  setLastEventSequence: (sequence: number) => void
  reset: () => void
}

const initialState = {
  draft: '',
  activeSessionId: null,
  activeRunId: null,
  activeRunVersion: 0,
  lastEventSequence: 0,
}

export const useAskStore = create<AskStore>(set => ({
  ...initialState,

  setDraft(draft) {
    set({ draft })
  },

  setActiveExecution(sessionID, runID, runVersion, lastEventSequence) {
    set(state => ({
      activeSessionId: sessionID,
      activeRunId: runID,
      activeRunVersion: runVersion,
      lastEventSequence: state.activeRunId === runID ? Math.max(state.lastEventSequence, lastEventSequence) : lastEventSequence,
    }))
  },

  setLastEventSequence(sequence) {
    set({ lastEventSequence: sequence })
  },

  reset() {
    set(initialState)
  },
}))
