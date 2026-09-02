import { Image, Text, View } from '@tarojs/components'
import backgroundImage from '../../assets/background1.png'
import { routes } from '../../constants/routes'
import { navigateTo } from '../../utils/navigation'
import './index.scss'

export default function Calendar() {
  return (
    <View className="cal-page" id="calendarModuleScreen">
      <Image className="cal-background" src={backgroundImage} mode="aspectFill" />
      <View className="module-header">
        <View className="module-account-button" onClick={() => navigateTo(routes.pages.account)}>账户</View>
        <Text className="module-header-title">日历</Text>
        <View className="module-header-spacer" />
      </View>
      <View className="cal-calendar-card">
        <View className="cal-calendar-head">
          <View>
            <Text className="cal-month-en">SEPTEMBER</Text>
            <Text className="cal-month-cn">九 月</Text>
          </View>
          <View className="cal-today-pill">今天</View>
        </View>
        <View className="cal-week-row">
          {['日', '一', '二', '三', '四', '五', '六'].map(day => <Text key={day}>{day}</Text>)}
        </View>
        <View className="cal-days-grid">
          {Array.from({ length: 28 }, (_, index) => (
            <View key={index} className={`cal-day${index === 1 ? ' active' : ''}`}>
              <Text>{index + 1}</Text>
              {index === 1 && <View className="cal-day-dot" />}
            </View>
          ))}
        </View>
      </View>
      <View className="cal-day-card">
        <View className="cal-day-header">
          <View>
            <Text className="cal-day-title">9 月 2 日 · 星期三</Text>
            <Text className="cal-day-subtitle">记录毛孩子的每一天</Text>
          </View>
          <View className="cal-day-add">＋</View>
        </View>
        <View className="cal-empty-state">
          <Text className="cal-empty-state__icon">🐾</Text>
          <Text className="cal-empty-state__title">今天还没有记录</Text>
          <Text className="cal-empty-state__text">用一条小事，留住今天的陪伴</Text>
        </View>
      </View>
    </View>
  )
}
