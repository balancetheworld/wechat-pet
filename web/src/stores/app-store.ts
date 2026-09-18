import { create } from 'zustand'

interface AppStore {
  bootstrapCompleted: boolean
  globalLoading: boolean
  bootstrapError: string
  retryKey: number
  calendarFormVisible: boolean
  /* 首次登录悬浮猫指引的跨页阶段: calendar=日历页提示, profile=档案页提示, outro=档案页收尾语 */
  guideStage: 'calendar' | 'profile' | 'outro' | null
  setBootstrapCompleted: (completed: boolean) => void
  setGlobalLoading: (loading: boolean) => void
  setBootstrapError: (message: string) => void
  setCalendarFormVisible: (visible: boolean) => void
  setGuideStage: (stage: 'calendar' | 'profile' | 'outro' | null) => void
  retryBootstrap: () => void
}

export const useAppStore = create<AppStore>(set => ({
  bootstrapCompleted: false,
  globalLoading: false,
  bootstrapError: '',
  retryKey: 0,
  calendarFormVisible: false,
  guideStage: null,

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

  setGuideStage(stage) {
    set({
      guideStage: stage,
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
