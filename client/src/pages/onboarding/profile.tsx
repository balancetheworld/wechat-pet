import type { ButtonProps } from '@tarojs/components'
import { Button, Image, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useState } from 'react'
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

  async function handleChooseAvatar(event: ChooseAvatarEvent) {
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
      setAvatarPath('')
      await Taro.showToast({ title: '头像上传失败，请重试', icon: 'none' })
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
        disabled={uploading || submitting}
        loading={uploading}
        openType="chooseAvatar"
        onChooseAvatar={handleChooseAvatar}
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
