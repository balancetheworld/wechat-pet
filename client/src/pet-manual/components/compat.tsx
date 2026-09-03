/**
 * 双端兼容组件（H5 / 微信小程序）
 *
 * 背景：本项目最初按 H5 原型直接在 JSX 里写 HTML 标签（h1/span/strong/button/input...），
 * 编译微信小程序时改成了 Taro 组件（View/Text/Button/Input/Image），导致两端的
 * DOM 结构与样式命中规则都发生了变化。
 *
 * 本文件提供"双端 shim"：
 *  - H5 端：渲染回原生 HTML 标签（并剥掉伴生 class，如 'h1'/'span'），
 *           DOM 与最初编译前的版本完全一致，原版 CSS 直接命中，渲染像素级一致；
 *  - 小程序端：渲染 Taro 组件并保留伴生 class（className='h1' 等），
 *           由 proto.scss 中的伴生选择器（.cover h1, .cover .h1）补回样式。
 *
 * 事件适配：Taro Input 的回调读取 e.detail.value，原生 input 读取 e.target.value，
 * 此处统一把原生事件包一层 detail，业务代码无需关心平台差异。
 */
import * as React from 'react'
import {
  View as TaroView,
  Text as TaroText,
  Button as TaroButton,
  Input as TaroInput,
  Image as TaroImage,
  Picker as TaroPicker,
  RichText as TaroRichText,
  ScrollView as TaroScrollView,
} from '@tarojs/components'

const IS_H5 = process.env.TARO_ENV === 'h5'

/** 有伴生 class 的文本标签（Text 组件在 H5 端还原为对应原生标签） */
const TEXT_TAGS = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'p', 'span', 'strong', 'i', 'em', 'b', 'u', 'small', 'time', 'label']

type AnyProps = Record<string, any>

/** View：H5 -> <div>；小程序 -> Taro View */
export const View = (props: AnyProps) => {
  if (!IS_H5) return <TaroView {...props} />
  const { children, ...rest } = props
  return <div {...rest}>{children}</div>
}

/**
 * Text：带伴生 class（如 'h1'/'span'/'strong'）时，H5 端还原为对应原生标签并剥掉该 class；
 * 不带伴生 class 时两端口径一致，直接用 Taro Text。
 */
export const Text = (props: AnyProps) => {
  const { className, children, ...rest } = props
  if (!IS_H5) return <TaroText className={className} {...rest}>{children}</TaroText>
  const cls = typeof className === 'string' ? className : ''
  const parts = cls.split(/\s+/).filter(Boolean)
  const tag = parts.find((c: string) => TEXT_TAGS.includes(c))
  if (!tag) return <TaroText className={className} {...rest}>{children}</TaroText>
  const kept = parts.filter((c: string) => c !== tag).join(' ')
  return React.createElement(tag, { className: kept || undefined, ...rest }, children)
}

/** Button：H5 -> 原生 <button>；小程序 -> Taro Button */
export const Button = (props: AnyProps) => {
  const { children, ...rest } = props
  if (!IS_H5) return <TaroButton {...rest}>{children}</TaroButton>
  return <button {...rest}>{children}</button>
}

/** Input：H5 -> 原生 <input>（事件包一层 detail 兼容 e.detail.value）；小程序 -> Taro Input */
export const Input = (props: AnyProps) => {
  const {
    className, id, value, placeholder, maxlength, maxLength, style, type, disabled,
    onInput, onFocus, onBlur, onConfirm, ...rest
  } = props
  if (!IS_H5) return <TaroInput {...props} />
  const adapt = (e: any) => {
    e.detail = { value: e.target.value }
    return e
  }
  return (
    <input
      {...rest}
      className={className}
      id={id}
      value={value}
      placeholder={placeholder}
      maxLength={maxlength ?? maxLength}
      style={style}
      type={type}
      disabled={disabled}
      onInput={onInput ? (e: any) => onInput(adapt(e)) : undefined}
      onFocus={onFocus ? (e: any) => onFocus(adapt(e)) : undefined}
      onBlur={onBlur ? (e: any) => onBlur(adapt(e)) : undefined}
    />
  )
}

/**
 * Image：
 *  - className 含 'avatar-photo' 时：H5 端还原为原型的"背景图 div"（avatar-photo
 *    在 proto.scss 中为绝对定位铺满父元素，圆角随父元素），小程序端仍用 Taro Image
 *    （WXSS 不支持本地路径 background-image）；
 *  - 其余情况：H5 -> 原生 <img>（mode 映射 object-fit）。
 */
export const Image = (props: AnyProps) => {
  const { className, src, mode, style, ...rest } = props
  if (!IS_H5) return <TaroImage className={className} src={src} mode={mode} style={style} {...rest} />
  const cls = typeof className === 'string' ? className : ''
  if (/(^|\s)avatar-photo(\s|$)/.test(cls)) {
    return (
      <div
        className={cls}
        style={{
          backgroundImage: src ? `url("${src}")` : undefined,
          backgroundSize: 'cover',
          backgroundPosition: 'center',
          backgroundRepeat: 'no-repeat',
          ...style,
        }}
      />
    )
  }
  const objectFit = mode === 'aspectFill' ? 'cover' : mode === 'aspectFit' ? 'contain' : 'fill'
  return <img className={cls} src={src} style={{ objectFit, ...style }} {...rest} />
}

/**
 * ScrollView：可滚动容器。
 *  - H5 端：普通 <div>（滚动由 CSS 的 overflow/max-height 规则驱动，与原型完全一致）；
 *  - 小程序端：Taro ScrollView（原生 scroll-view 组件）——小程序里普通 view 的
 *              CSS overflow 滚动在真机/工具上不可靠，原生 scroll-view 才能保证触摸滑动。
 */
export const ScrollView = (props: AnyProps) => {
  const { children, ...rest } = props
  if (!IS_H5) return <TaroScrollView scrollY enhanced showScrollbar={false} {...rest}>{children}</TaroScrollView>
  return <div {...rest}>{children}</div>
}

/**
 * Html：富文本容器（如 pet.type 里的 "猫<br>英短"）。
 *  - H5 端：className 含伴生 class（如 'p'）时还原为对应原生标签并剥掉该 class，
 *           与 Text 组件口径一致；HTML 直接写进容器（不多包一层节点）；
 *  - 小程序端：容器用 Taro View（保留伴生 class），内容由 RichText 渲染。
 */
export const Html = ({ className, html, ...rest }: AnyProps) => {
  if (!IS_H5) {
    return (
      <TaroView className={className} {...rest}>
        <TaroRichText nodes={html} />
      </TaroView>
    )
  }
  const cls = typeof className === 'string' ? className : ''
  const parts = cls.split(/\s+/).filter(Boolean)
  const tag = parts.find((c: string) => TEXT_TAGS.includes(c)) || 'div'
  const kept = parts.filter((c: string) => c !== tag).join(' ')
  return React.createElement(tag, { className: kept || undefined, ...rest, dangerouslySetInnerHTML: { __html: html } })
}

/**
 * DateInput：日期选择。
 *  - H5 端：原生 <input type='date'>（与编译前原型一致）；
 *  - 小程序端：Taro Picker(mode='date') 包一个展示 View。
 * 回调统一为 Taro 口径：onChange(e) => e.detail.value。
 */
export const DateInput = ({ value, max, min, onChange, className = 'capsule-input' }: AnyProps) => {
  if (!IS_H5) {
    // 小程序 Picker(mode='date') 的区间参数是 start/end（对应 H5 的 min/max），此处做映射
    return (
      <TaroPicker mode='date' value={value} start={min} end={max} onChange={onChange}>
        <TaroView className='capsule-input date-view'>{value}</TaroView>
      </TaroPicker>
    )
  }
  return (
    <input
      type='date'
      className={className}
      value={value}
      max={max}
      min={min}
      onChange={(e: any) => {
        if (onChange) onChange({ detail: { value: e.target.value } })
      }}
    />
  )
}
