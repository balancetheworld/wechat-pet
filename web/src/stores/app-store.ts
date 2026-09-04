import { create } from 'zustand'

interface AppStore {
  bootstrapCompleted: boolean
  globalLoading: boolean
  bootstrapError: string
  retryKey: number
  calendarFormVisible: boolean
  setBootstrapCompleted: (completed: boolean) => void
  setGlobalLoading: (loading: boolean) => void
  setBootstrapError: (message: string) => void
  setCalendarFormVisible: (visible: boolean) => void
  retryBootstrap: () => void
}

export const useAppStore = create<AppStore>(set => ({
  bootstrapCompleted: false,
  globalLoading: false,
  bootstrapError: '',
  retryKey: 0,
  calendarFormVisible: false,

  setBootstrapCompleted(completed) {
    set({
      bootstrapCompleted: completed,
    })
  },

  setGlobalLoading(loading) {
    set({
      globalLoading: loading,
    })
  },

  setBootstrapError(message) {
    set({
      bootstrapError: message,
    })
  },

  setCalendarFormVisible(visible) {
    set({
      calendarFormVisible: visible,
    })
  },

  retryBootstrap() {
    set(state => ({
      bootstrapCompleted: false,
      bootstrapError: '',
      retryKey: state.retryKey + 1,
    }))
  },
}))
