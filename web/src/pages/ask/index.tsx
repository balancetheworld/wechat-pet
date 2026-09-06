import { Image, Input, View } from '@tarojs/components'
import askBackground from '../../assets/ai-bg.jpg'
import catImage from '../../assets/ai-cat.png'
import './index.scss'

export default function Ask() {
  return (
    <View className="ask-page" id="askModuleScreen">
      <Image className="ask-background" src={askBackground} mode="aspectFill" />
      <View className="ask-cat-wrap">
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
