import type { PropsWithChildren } from 'react'
import { useEffect } from 'react'
import Taro from '@tarojs/taro'
import { useBootstrap } from './hooks/use-bootstrap'

import './app.scss'

/**
 * 手写字体（宠物小册模块使用，原 Pet-Manual 在页面级加载；改为 App 级全局加载一次，
 * 避免三个 tab 页各自重复 loadFontFace）。
 * 真机上需把 fonts.gstatic.com 加入 downloadFile 合法域名。
 */
const PET_MANUAL_FONTS: Array<[string, string]> = [
  ['Ma Shan Zheng', 'https://fonts.gstatic.com/s/mashanzheng/v18/NaPecZTRCLxvwo41b4gvzkXaRMQ.ttf'],
  ['ZCOOL KuaiLe', 'https://fonts.gstatic.com/s/zcoolkuaile/v22/tssqApdaRQokwFjFJjvM6h2Wpg.ttf'],
]

function App({ children }: PropsWithChildren<any>) {
  useBootstrap()

  useEffect(() => {
    if (process.env.TARO_ENV === 'weapp') {
      PET_MANUAL_FONTS.forEach(([family, url]) => {
        Taro.loadFontFace({
          family,
          source: `url("${url}")`,
          global: true,
          fail: () => { /* 字体缺失不影响功能，回退系统字体 */ },
        })
      })
    }
  }, [])

  // children 是将要会渲染的页面
  return children
}

export default App
