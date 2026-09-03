import type { ButtonProps } from '@tarojs/components'
import { Button, Image, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useRef, useState } from 'react'
import { routes } from '../../constants/routes'
import { updateProfile, uploadAvatar } from '../../services/user'
import { useAuthStore } from '../../stores/auth-store'
import { switchTab } from '../../utils/navigation'
import './profile.scss'

type ChooseAvatarEvent = Parameters<NonNullable<ButtonProps['onChooseAvatar']>>[0]

export default function ProfileOnboarding() {
  const user = useAuthStore(state => state.user)
  const [nickname, setNickname] = useState(user?.nickname ?? '')
  const [avatarPath, setAvatarPath] = useState(user?.avatarUrl ?? '')
  const [avatarAssetID, setAvatarAssetID] = useState('')
  const [uploading, setUploading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  // 头像选择防抖（同步文件三 6cb5b16 修复）：chooseAvatar 弹起期间按钮进入
  // 选择中状态；用户取消选择时 onChooseAvatar 不会回调，靠 onError 兜底复位，
  // 避免按钮永久卡在 loading/disabled。
  const [choosingAvatar, setChoosingAvatar] = useState(false)
  const choosingAvatarRef = useRef(false)

  function handleAvatarClick() {
    if (choosingAvatarRef.current) {
      return
    }
    choosingAvatarRef.current = true
    setChoosingAvatar(true)
  }

  function handleChooseAvatarError() {
    choosingAvatarRef.current = false
    setChoosingAvatar(false)
  }

  async function handleChooseAvatar(event: ChooseAvatarEvent) {
    choosingAvatarRef.current = false
    setChoosingAvatar(false)
    const avatarUrl = event.detail?.avatarUrl
    if (!avatarUrl) {
      return
    }
    setAvatarPath(avatarUrl)
    setUploading(true)
    try {
      const result = await uploadAvatar(avatarUrl)
      setAvatarAssetID(result.asset_id)
    }
    catch {
      // 预览阶段兜底：后端 /assets/upload（COS）尚未实现，上传必然失败。
      // 后端 PATCH /users/me 对 avatar_asset_id 仅做长度校验并原样存储回显，
      // 因此直接用本地临时头像路径作为标识提交，保证资料完善流程可继续。
      // 注意：wxfile:// 临时路径仅本会话有效，接入真实上传服务后此分支自然失效。
      const fallbackId = avatarUrl.length <= 128 ? avatarUrl : `preview-${Date.now()}`
      setAvatarAssetID(fallbackId)
      await Taro.showToast({ title: '上传服务未就绪，已用本地头像继续', icon: 'none' })
    }
    finally {
      setUploading(false)
    }
  }

  async function handleSubmit() {
    const value = nickname.trim()
    if (value.length < 1 || value.length > 50) {
      await Taro.showToast({ title: '昵称长度需为 1-50 个字符', icon: 'none' })
      return
    }
    if (!avatarAssetID) {
      await Taro.showToast({ title: '请先选择头像', icon: 'none' })
      return
    }
    setSubmitting(true)
    try {
      await updateProfile({ nickname: value, avatar_asset_id: avatarAssetID })
      await Taro.showToast({ title: '资料已保存', icon: 'success' })
      await switchTab(routes.tabs.home)
    }
    catch {
      await Taro.showToast({ title: '保存失败，请重试', icon: 'none' })
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <View className="profile-onboarding">
      <Text className="profile-onboarding__title">完善个人资料</Text>
      <Text className="profile-onboarding__hint">头像和昵称仅在你主动选择后保存</Text>
      <Button
        className="profile-onboarding__avatar-button"
        disabled={choosingAvatar || uploading || submitting || Boolean(avatarPath && !avatarAssetID)}
        loading={choosingAvatar || uploading}
        openType="chooseAvatar"
        onClick={handleAvatarClick}
        onChooseAvatar={handleChooseAvatar}
        onError={handleChooseAvatarError}
      >
        {avatarPath ? <Image className="profile-onboarding__avatar" src={avatarPath} /> : '选择头像'}
      </Button>
      <Input
        className="profile-onboarding__input"
        maxlength={50}
        placeholder="请输入昵称"
        type="nickname"
        value={nickname}
        onInput={event => setNickname(event.detail.value)}
      />
      <Button
        className="profile-onboarding__submit"
        disabled={uploading || submitting}
        loading={submitting}
        onClick={handleSubmit}
      >
        保存资料
      </Button>
    </View>
  )
}
