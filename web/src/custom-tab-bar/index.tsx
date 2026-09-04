import { Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useEffect, useState } from 'react'
import { useAppStore } from '../stores/app-store'
import './index.scss'

const tabs = [
  { pagePath: 'pages/calendar/index', label: '日历', icon: '▦' },
  { pagePath: 'pages/ask/index', label: '问问', icon: '✦' },
  { pagePath: 'pages/profile/index', label: '档案', icon: '▤' },
]

export default function CustomTabBar() {
  const activePath = Taro.getCurrentInstance()?.router?.path || ''
  const calendarFormVisible = useAppStore(state => state.calendarFormVisible)
  const [eventHidden, setEventHidden] = useState(false)

  useEffect(() => {
    const handleCalendarFormVisibility = (visible: boolean) => {
      setEventHidden(visible)
    }
    Taro.eventCenter.on('calendar-form-visibility', handleCalendarFormVisibility)
    return () => {
      Taro.eventCenter.off('calendar-form-visibility', handleCalendarFormVisibility)
    }
  }, [])

  function switchTab(pagePath: string) {
    if (pagePath === activePath)
      return

    void Taro.switchTab({ url: `/${pagePath}` })
  }

  return (
    <View className={`pet-tab-bar${calendarFormVisible || eventHidden ? ' hidden' : ''}`}>
      <View className="pet-tab-bar__tabs">
        {tabs.map(tab => (
          <View
            key={tab.pagePath}
            className={`pet-tab-bar__item${activePath === tab.pagePath ? ' active' : ''}`}
            onClick={() => switchTab(tab.pagePath)}
          >
            <Text className="pet-tab-bar__icon">{tab.icon}</Text>
            <Text className="pet-tab-bar__label">{tab.label}</Text>
          </View>
        ))}
      </View>
    </View>
  )
}
