import { View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import CalPanel from '../../pet-manual/pages/index/CalPanel'
import { useCalStore } from '../../pet-manual/stores/useCalStore'
import { useTabStore } from '../../stores/tab-store'
import '../../pet-manual/styles/pet-manual.scss'

export default function Calendar() {
  const records = useCalStore(s => s.records)
  const todos = useCalStore(s => s.todos)
  const setRecords = useCalStore(s => s.setRecords)
  const setTodos = useCalStore(s => s.setTodos)
  const setActiveTab = useTabStore(s => s.setActiveTab)

  useDidShow(() => {
    // 同步自定义 tabBar 高亮：1=宠物日历
    setActiveTab(1)
  })

  const onToast = (msg: string) => {
    Taro.showToast({ title: msg, icon: 'none' })
  }

  return (
    <View className='app' id='app'>
      <View className='module-screen calendar-screen' id='calendarModuleScreen'>
        <CalPanel
          onToast={onToast}
          records={records}
          todos={todos}
          onRecordsChange={setRecords}
          onTodosChange={setTodos}
        />
      </View>
    </View>
  )
}
