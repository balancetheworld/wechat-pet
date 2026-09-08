import { Button, Image, Input, View } from '@tarojs/components'
import Taro, { useDidHide, useDidShow } from '@tarojs/taro'
import { useRef, useState } from 'react'
import askBackground from '../../assets/ai-bg.jpg'
import catImage from '../../assets/ai-cat.png'
import { routes } from '../../constants/routes'
import { switchTab } from '../../utils/navigation'
import './index.scss'

export default function Ask() {
  const [navHidden, setNavHidden] = useState(false)
  const navAnimationTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useDidShow(() => {
    if (navAnimationTimer.current) {
      clearTimeout(navAnimationTimer.current)
    }
    setNavHidden(false)
    Taro.eventCenter.trigger('ask-page-visibility', true)
    navAnimationTimer.current = setTimeout(() => {
      setNavHidden(true)
      navAnimationTimer.current = null
    }, 16)
  })

  useDidHide(() => {
    if (navAnimationTimer.current) {
      clearTimeout(navAnimationTimer.current)
      navAnimationTimer.current = null
    }
    setNavHidden(false)
    Taro.eventCenter.trigger('ask-page-visibility', false)
  })

  return (
    <View className={`ask-page${navHidden ? ' nav-hidden' : ''}`} id="askModuleScreen">
      <Image className="ask-background" src={askBackground} mode="aspectFill" />
      <Button className="ask-nav-arrow ask-nav-arrow--left" aria-label="前往日历" onClick={() => void switchTab(routes.tabs.calendar)}>
        ‹
      </Button>
      <Button className="ask-nav-arrow ask-nav-arrow--right" aria-label="前往档案" onClick={() => void switchTab(routes.tabs.profile)}>
        ›
      </Button>
      <View className="ask-content-area">
        <View className="ask-cat-wrap">
          <Image className="ask-cat" src={catImage} mode="aspectFit" />
        </View>
        <View className="preset-row">
          <View className="preset-chip">疫苗提醒</View>
          <View className="preset-chip">饮食建议</View>
          <View className="preset-chip">健康记录</View>
        </View>
      </View>
      <View className="ask-input-bar">
        <View className="ask-bottom-row">
          <View className="ask-add-img">＋</View>
          <Input className="ask-textarea" placeholder="问我关于宠物的问题吧~" />
          <View className="ask-send-btn">➤</View>
        </View>
      </View>
    </View>
  )
}
