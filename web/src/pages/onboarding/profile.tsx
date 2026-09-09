import type { ButtonProps } from '@tarojs/components'
import { Button, Image, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useEffect, useRef, useState } from 'react'
import backgroundImage from '../../assets/background2.png'
import { routes } from '../../constants/routes'
import { silentLogin } from '../../services/auth'
import { updateProfile, uploadAvatar } from '../../services/user'
import { reLaunch } from '../../utils/navigation'
import './profile.scss'

type ChooseAvatarEvent = Parameters<NonNullable<ButtonProps['onChooseAvatar']>>[0]

export default function ProfileOnboarding() {
  const [loading, setLoading] = useState(false)
  const [profileVisible, setProfileVisible] = useState(false)
  const [loginCompleted, setLoginCompleted] = useState(false)
  const [loginFailed, setLoginFailed] = useState(false)
  const [nickname, setNickname] = useState('')
  const [avatarPath, setAvatarPath] = useState('')
  const [avatarAssetID, setAvatarAssetID] = useState('')
  const [choosingAvatar, setChoosingAvatar] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const choosingAvatarRef = useRef(false)

  function handleLogin() {
    if (loading) {
      return
    }
    const alreadyVisible = profileVisible
    setProfileVisible(true)
    setLoading(true)
    setLoginFailed(false)
    void Taro.showToast({ title: '正在打开资料', icon: 'none', duration: 1000 })
    if (alreadyVisible) {
      void login()
    }
  }

  async function login() {
    setLoading(true)
    try {
      const session = await silentLogin()
      setNickname(value => value || session.user.nickname)
      setLoginCompleted(true)
    }
    catch (error) {
      setLoginFailed(true)
      const title = error instanceof Error && /timeout|超时/i.test(error.message) ? '登录超时，请重试' : '登录失败，请重试'
      await Taro.showToast({ title, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (!profileVisible) {
      return
    }
    void login()
  }, [profileVisible])

  useEffect(() => {
    if (!loginCompleted || !avatarPath || avatarAssetID || uploading) {
      return
    }

    async function uploadSelectedAvatar() {
      setUploading(true)
      try {
        const result = await uploadAvatar(avatarPath)
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

    void uploadSelectedAvatar()
  }, [avatarAssetID, avatarPath, loginCompleted, uploading])

  function handleChooseAvatar(event: ChooseAvatarEvent) {
    choosingAvatarRef.current = false
    setChoosingAvatar(false)
    const avatarUrl = event.detail?.avatarUrl
    if (avatarUrl) {
      setAvatarPath(avatarUrl)
      setAvatarAssetID('')
    }
  }

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
      await reLaunch(routes.pages.home)
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
      <Image className="profile-onboarding__background" src={backgroundImage} mode="aspectFill" />
      {profileVisible
        ? (
            <>
              <Text className="profile-onboarding__title">选择头像和名称</Text>
              {loading && <Text className="profile-onboarding__login-hint">正在绑定微信账号...</Text>}
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
              <View className="profile-onboarding__actions">
                <Button
                  className={`profile-onboarding__login${loginFailed ? '' : ' profile-onboarding__login--idle'}`}
                  disabled={loading}
                  onClick={handleLogin}
                >
                  重新登录
                </Button>
                <Button
                  className="profile-onboarding__submit"
                  disabled={!loginCompleted || uploading || submitting}
                  loading={submitting}
                  onClick={handleSubmit}
                >
                  保存资料
                </Button>
              </View>
            </>
          )
        : (
            <>
              <Text className="profile-onboarding__title">欢迎来到宠物小程序</Text>
              <Button
                className="profile-onboarding__login profile-onboarding__login--welcome"
                hoverClass="profile-onboarding__login--welcome-hover"
                onClick={handleLogin}
              >
                登录
              </Button>
            </>
          )}
    </View>
  )
}
