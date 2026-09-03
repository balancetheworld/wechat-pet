import { View } from '@tarojs/components'
import { useDidShow } from '@tarojs/taro'
import AskPanel from '../../pet-manual/pages/index/AskPanel'
import { useTabStore } from '../../stores/tab-store'
import '../../pet-manual/styles/pet-manual.scss'

export default function Ask() {
  const setActiveTab = useTabStore(s => s.setActiveTab)

  useDidShow(() => {
    // 同步自定义 tabBar 高亮：2=AI 助手
    setActiveTab(2)
  })

  return (
    <View className='app' id='app'>
      <View className='module-screen ai-screen' id='aiModuleScreen'>
        <AskPanel />
      </View>
    </View>
  )
}
