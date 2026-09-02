import { Image, Input, Text, View } from '@tarojs/components'
import askBackground from '../../assets/ai-bg.jpg'
import catImage from '../../assets/ai-cat.png'
import { routes } from '../../constants/routes'
import { navigateTo } from '../../utils/navigation'
import './index.scss'

export default function Ask() {
  return (
    <View className="ask-page" id="askModuleScreen">
      <Image className="ask-background" src={askBackground} mode="aspectFill" />
      <View className="module-header">
        <View className="module-account-button" onClick={() => navigateTo(routes.pages.account)}>账户</View>
        <Text className="module-header-title">问问</Text>
        <View className="module-header-spacer" />
      </View>
      <View className="ask-stage">
        <View className="ask-welcome">
          <Text className="ask-welcome__eyebrow">PET ASSISTANT</Text>
          <Text className="ask-welcome__title">今天想问 TA 什么？</Text>
          <Text className="ask-welcome__subtitle">从宠物档案与日常记录开始聊起</Text>
        </View>
        <Image className="ask-cat" src={catImage} mode="aspectFit" />
      </View>
      <View className="preset-row">
        <View className="preset-chip">疫苗提醒</View>
        <View className="preset-chip">饮食建议</View>
        <View className="preset-chip">健康记录</View>
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
