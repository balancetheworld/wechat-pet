import { Button, Image, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useEffect, useState } from 'react'
import calendarIcon from '../assets/icons/calendar-days.svg'
import askIcon from '../assets/icons/message-circle.svg'
import profileIcon from '../assets/icons/notebook-tabs.svg'
import { useAppStore } from '../stores/app-store'
import './index.scss'

const tabs = [
  { pagePath: 'pages/calendar/index', label: '日历', icon: calendarIcon },
  { pagePath: 'pages/ask/index', label: '问问', icon: askIcon },
  { pagePath: 'pages/profile/index', label: '档案', icon: profileIcon },
]

export default function CustomTabBar() {
  const activePath = (Taro.getCurrentInstance()?.router?.path || '').replace(/^\/+/, '').split('?')[0]
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
          <Button
            key={tab.pagePath}
            className={`pet-tab-bar__item${activePath === tab.pagePath ? ' active' : ''}`}
            aria-label={tab.label}
            onClick={() => switchTab(tab.pagePath)}
          >
            <Image className="pet-tab-bar__icon" src={tab.icon} mode="aspectFit" />
          </Button>
        ))}
      </View>
    </View>
  )
}
