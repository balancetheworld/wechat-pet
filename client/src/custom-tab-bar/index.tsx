/**
 * 自定义 tabBar —— 复刻文件二（Pet-Manual 原型）底部导航视觉
 *
 * - 悬浮玻璃胶囊 `.app-tabs`（居中底部、带蓝色滑动指示器），三枚 tab：
 *   宠物档案 / 宠物日历 / AI 助手（与文件二原型一致，「我的」已删除）；
 * - 左下角 `.floating-family-button`（⌂）进入家庭中心页；
 * - 激活态由全局 useTabStore 驱动（各 tab 页 useDidShow 时写入），
 *   每页独立的 tabBar 实例通过订阅保持一致；
 * - custom-tab-bar 组件在微信中是隔离容器（styleIsolation: isolated），
 *   根节点必须占满该容器，子元素使用 absolute 定位。
 */
import Taro from '@tarojs/taro'
import { Text, View } from '@tarojs/components'
import { useTabStore } from '../stores/tab-store'
import { routes } from '../constants/routes'
import { navigateTo } from '../utils/navigation'
import './index.scss'

const TABS = [
  { path: '/pages/index/index', icon: '▤', label: '宠物档案' },
  { path: '/pages/calendar/index', icon: '▦', label: '宠物日历' },
  { path: '/pages/ask/index', icon: '✦', label: 'AI 助手' },
] as const

export default function CustomTabBar() {
  const activeTab = useTabStore(s => s.activeTab)

  const onTab = (idx: number) => {
    if (idx === activeTab) return
    Taro.switchTab({ url: TABS[idx].path })
  }

  const onFamily = () => {
    navigateTo(routes.pages.familyMembers)
  }

  // 滑块 translate：列步进 = 列宽 174.36 + 间距 11.54 = 185.9（rpx，与 index.scss 一致）
  const indicatorStyle = {
    transform: `translateX(${activeTab * 185.9}rpx)`,
  }

  return (
    <View className='custom-tab-bar'>
      {/* 家庭中心入口（左下角悬浮按钮，对应文件二 floating-family-button） */}
      <View className='floating-family-button' onClick={onFamily}>⌂</View>

      {/* 底部功能切换胶囊 */}
      <View className='app-tabs'>
        <View className='app-tab-indicator' style={indicatorStyle} />
        {TABS.map((tab, idx) => (
          <View
            key={tab.path}
            className={`app-tab${idx === activeTab ? ' active' : ''}`}
            onClick={() => onTab(idx)}
          >
            <Text className='app-tab-icon'>{tab.icon}</Text>
            <Text className='app-tab-label'>{tab.label}</Text>
          </View>
        ))}
      </View>
    </View>
  )
}
