import { Image, Text, View } from '@tarojs/components'
import { useCallback, useEffect, useRef, useState } from 'react'
import cat2Image from '../../assets/cat2.png'
import { routes } from '../../constants/routes'
import { useAppStore } from '../../stores/app-store'
import { switchTab } from '../../utils/navigation'
import './index.scss'

const CALENDAR_HINT = '后续点击右上角的“账户”按钮，可以修改或加入家庭哦'
const PROFILE_HINT = '点击右上角的按钮，可以添加或切换小动物的档案哦'
const OUTRO_LINES = [
  '先聊到这，其他的功能就让人自己来探索吧',
  '以后可以多来对话界面找猫聊天哦，猫可以快速告诉人小动物的信息和医疗情况',
  '记不住的医疗活动直接告诉猫，猫来提醒你',
  '那么，下次见喵！',
]

function sleep(ms: number) {
  return new Promise<void>(resolve => setTimeout(resolve, ms))
}

export function FloatingGuide() {
  const guideStage = useAppStore(state => state.guideStage)
  const setGuideStage = useAppStore(state => state.setGuideStage)
  const [text, setText] = useState('')
  const [leaving, setLeaving] = useState(false)
  const [waitingTap, setWaitingTap] = useState(false)
  const cancelledRef = useRef(false)
  const tapResolverRef = useRef<(() => void) | null>(null)

  /* 收尾语逐句播放: 打完一句后挂起, 等用户点击任意处再继续下一句 */
  function waitTap(): Promise<void> {
    return new Promise<void>((resolve) => {
      tapResolverRef.current = resolve
      setWaitingTap(true)
    })
  }

  function resolveTap() {
    const resolve = tapResolverRef.current
    if (!resolve) {
      return
    }
    tapResolverRef.current = null
    setWaitingTap(false)
    resolve()
  }

  const typeText = useCallback(async (value: string, speed = 60) => {
    for (let i = 0; i <= value.length; i += 1) {
      if (cancelledRef.current) {
        return
      }
      setText(value.slice(0, i))
      await sleep(speed)
    }
  }, [])

  const eraseText = useCallback(async (value: string, speed = 30) => {
    for (let i = value.length; i >= 0; i -= 1) {
      if (cancelledRef.current) {
        return
      }
      setText(value.slice(0, i))
      await sleep(speed)
    }
  }, [])

  useEffect(() => {
    if (!guideStage) {
      return
    }
    /* 每次阶段切换取消上一段未完成的打字, 立即开始新阶段的台词 */
    cancelledRef.current = false
    let disposed = false
    void (async () => {
      if (guideStage === 'calendar') {
        await typeText(CALENDAR_HINT)
      }
      else if (guideStage === 'profile') {
        await typeText(PROFILE_HINT)
      }
      else {
        /* 收尾语: 每句打完后等用户点击任意处, 再擦除并继续下一句 */
        for (const line of OUTRO_LINES) {
          await typeText(line)
          await waitTap()
          if (disposed || cancelledRef.current) {
            return
          }
          await eraseText(line)
        }
        setLeaving(true)
        await sleep(420)
        if (!disposed) {
          setGuideStage(null)
        }
      }
    })()
    return () => {
      disposed = true
      cancelledRef.current = true
    }
  }, [guideStage, setGuideStage, typeText, eraseText])

  /* 尚无指引或指引未开始时不渲染 */
  if (!guideStage) {
    return null
  }

  /* 等待用户点击任意处的阶段: 铺一层透明捕获层
     (前两个板块整段等待; 收尾语只在每句打完后的等待期铺层) */
  const showCatcher = guideStage === 'calendar' || guideStage === 'profile' || (guideStage === 'outro' && waitingTap)

  function handleCatch() {
    if (guideStage === 'calendar') {
      /* 第 7 轮: 点击任意处后切换到档案页继续第 8 轮
         (switchTab 保住 tabBar 页面栈, 避免整页销毁重建造成空屏卡顿) */
      setGuideStage('profile')
      void switchTab(routes.tabs.profile)
    }
    else if (guideStage === 'profile') {
      setGuideStage('outro')
    }
    else if (guideStage === 'outro') {
      resolveTap()
    }
  }

  return (
    <View className="floating-guide">
      {showCatcher && <View className="floating-guide__catcher" onClick={handleCatch} />}
      <View className={`floating-guide__stage${leaving ? ' floating-guide__stage--out' : ''}`}>
        <View className="floating-guide__bubble">
          <Text className="floating-guide__text">{text}</Text>
        </View>
        <Image className="floating-guide__cat" src={cat2Image} mode="aspectFit" />
      </View>
    </View>
  )
}
