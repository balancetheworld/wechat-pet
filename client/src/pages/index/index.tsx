import { Text, View } from '@tarojs/components'
import { useLoad } from '@tarojs/taro'
import './index.scss'

export default function Index() {
  useLoad(() => {})

  return (
    <View className="index">
      <Text>Hello world!</Text>
    </View>
  )
}
