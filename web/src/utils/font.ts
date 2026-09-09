import Taro from '@tarojs/taro'

/**
 * 站酷快乐体（HTTPS 字体文件 URL）。
 *
 * 注意：
 * 1. 微信小程序使用自定义字体需要把字体文件放到 HTTPS 服务器/CDN
 *    （如 腾讯云 COS / 阿里云 OSS / 七牛 / jsDelivr 镜像等）。
 * 2. 需要在「微信公众平台 → 开发 → 开发管理 → 服务器域名」中
 *    把该字体域名加到 downloadFile 合法域名白名单。
 * 3. 字体名必须与 CSS `font-family` 中使用的字符串完全一致（区分空格/大小写）。
 */
const FONT_FACE_URL = 'https://fonts.gstatic.com/s/zcoolkuaile/v22/tssqApdaRQokwFjFJjvM6h2Wpg.ttf'

const HANDWRITTEN_FAMILY = 'ZCOOL KuaiLe'

export async function loadHandwrittenFont() {
  try {
    await Taro.loadFontFace({
      family: HANDWRITTEN_FAMILY,
      source: `url("${FONT_FACE_URL}")`,
    })
  }
  catch (error) {
    console.warn('[font] 加载站酷快乐体失败，将使用系统默认字体：', error)
  }
}
