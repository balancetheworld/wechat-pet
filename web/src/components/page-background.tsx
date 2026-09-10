import { Image } from '@tarojs/components'
import backgroundImage from '../assets/background2.jpg'

export default function PageBackground() {
  return <Image className="page-background" src={backgroundImage} mode="aspectFill" />
}
