import { Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useEffect, useRef, useState } from 'react'
import { useAppStore } from '../stores/app-store'
import './index.scss'

const tabs = [
  { pagePath: 'pages/calendar/index', label: '日历', icon: '▦' },
  { pagePath: 'pages/ask/index', label: '问问', icon: '✦' },
  { pagePath: 'pages/profile/index', label: '档案', icon: '▤' },
]

export default function CustomTabBar() {
  const activePath = (Taro.getCurrentInstance()?.router?.path || '').replace(/^\/+/, '').split('?')[0]
  const calendarFormVisible = useAppStore(state => state.calendarFormVisible)
  const [eventHidden, setEventHidden] = useState(false)
  const [askPageHidden, setAskPageHidden] = useState(activePath === 'pages/ask/index')
  const [askPageLeaving, setAskPageLeaving] = useState(false)
  const askHideTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    const handleCalendarFormVisibility = (visible: boolean) => {
      setEventHidden(visible)
    }
    const handleAskPageVisibility = (visible: boolean) => {
      if (askHideTimer.current) {
        clearTimeout(askHideTimer.current)
        askHideTimer.current = null
      }
      if (visible) {
        setAskPageLeaving(true)
        askHideTimer.current = setTimeout(() => {
          setAskPageHidden(true)
          setAskPageLeaving(false)
          askHideTimer.current = null
        }, 220)
        return
      }
      setAskPageLeaving(false)
      setAskPageHidden(false)
    }
    Taro.eventCenter.on('calendar-form-visibility', handleCalendarFormVisibility)
    Taro.eventCenter.on('ask-page-visibility', handleAskPageVisibility)
    return () => {
      Taro.eventCenter.off('calendar-form-visibility', handleCalendarFormVisibility)
      Taro.eventCenter.off('ask-page-visibility', handleAskPageVisibility)
      if (askHideTimer.current) {
        clearTimeout(askHideTimer.current)
      }
    }
  }, [])

  if (askPageHidden) {
    return null
  }

  function switchTab(pagePath: string) {
    if (pagePath === activePath)
      return

    void Taro.switchTab({ url: `/${pagePath}` })
  }

  return (
    <View className={`pet-tab-bar${calendarFormVisible || eventHidden ? ' hidden' : ''}${askPageLeaving ? ' ask-leaving' : ''}`}>
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
