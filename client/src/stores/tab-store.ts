/**
 * 底部导航（custom-tab-bar）共享状态
 *
 * 自定义 tabBar 组件实例在每个 tab 页各自独立（微信为每页创建一个实例），
 * 无法通过单一组件 state 感知「当前处于哪个 tab」。因此用全局 zustand store：
 * 每个 tab 页在 useDidShow 时写入 activeTab，四个实例共同订阅，保证高亮/滑块一致。
 */
import { create } from 'zustand'

interface TabStoreState {
  /** 当前激活的 tab 下标：0=宠物档案 1=宠物日历 2=AI 助手 3=我的 */
  activeTab: number
  setActiveTab: (idx: number) => void
}

export const useTabStore = create<TabStoreState>(set => ({
  activeTab: 0,
  setActiveTab: idx => set({ activeTab: idx }),
}))
