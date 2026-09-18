import type { ButtonProps } from '@tarojs/components'
import { Button, Image, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useCallback, useEffect, useRef, useState } from 'react'
import bg1Image from '../../assets/bg1.jpg'
import cat2Image from '../../assets/cat2.png'
import catImage from '../../assets/cat.png'
import logoImage from '../../assets/logo.png'
import { routes } from '../../constants/routes'
import { silentLogin } from '../../services/auth'
import { applyJoinFamily, createFamily } from '../../services/family'
import { updateProfile, uploadAvatar } from '../../services/user'
import { useAuthStore } from '../../stores/auth-store'
import { useFamilyStore } from '../../stores/family-store'
import { navigateTo, reLaunch, switchTab } from '../../utils/navigation'
import './profile.scss'

type ChooseAvatarEvent = Parameters<NonNullable<ButtonProps['onChooseAvatar']>>[0]

export default function ProfileOnboarding() {
  const [loading, setLoading] = useState(false)
  const [loginCompleted, setLoginCompleted] = useState(false)
  const [nickname, setNickname] = useState('')
  const [avatarPath, setAvatarPath] = useState('')
  const [avatarAssetID, setAvatarAssetID] = useState('')
  const [choosingAvatar, setChoosingAvatar] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [speechText, setSpeechText] = useState('')
  const [helloDisabled, setHelloDisabled] = useState(false)
  const [helloMounted, setHelloMounted] = useState(true)
  const [helloOut, setHelloOut] = useState(false)
  const [loginMounted, setLoginMounted] = useState(false)
  const [authDisabled, setAuthDisabled] = useState(false)
  const [loginOut, setLoginOut] = useState(false)
  const [ackMounted, setAckMounted] = useState(false)
  const [ackOut, setAckOut] = useState(false)
  const [ackDisabled, setAckDisabled] = useState(false)
  const [profileMounted, setProfileMounted] = useState(false)
  const [profileSubmitted, setProfileSubmitted] = useState(false)
  const [avatarOut, setAvatarOut] = useState(false)
  const [fieldsOut, setFieldsOut] = useState(false)
  const [familyStep, setFamilyStep] = useState<'choice' | 'create' | 'join' | null>(null)
  const [familyName, setFamilyName] = useState('')
  const [familyCode, setFamilyCode] = useState('')
  const [familySubmitting, setFamilySubmitting] = useState(false)
  const choosingAvatarRef = useRef(false)

  function handleLogin() {
    if (loading || authDisabled) {
      return
    }
    void login()
  }

  async function login() {
    setLoading(true)
    setAuthDisabled(true)
    try {
      const session = await silentLogin()
      /* 已有家庭的老用户直接进入主界面, 跳过头像和选家庭步骤 */
      if (session.family && session.identity !== 'guest') {
        await reLaunch(routes.tabs.calendar)
        return
      }
      setNickname(value => value || session.user.nickname)
      setLoginCompleted(true)
      void playLoginSuccessFlow()
    }
    catch (error) {
      setAuthDisabled(false)
      const title = error instanceof Error && /timeout|超时/i.test(error.message) ? '登录超时，请重试' : '登录失败，请重试'
      await Taro.showToast({ title, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  function sleep(ms: number) {
    return new Promise<void>(resolve => setTimeout(resolve, ms))
  }

  const typeText = useCallback(async (text: string, speed = 70) => {
    for (let i = 0; i <= text.length; i += 1) {
      setSpeechText(text.slice(0, i))
      await sleep(speed)
    }
  }, [])

  const eraseText = useCallback(async (text: string, speed = 35) => {
    for (let i = text.length; i >= 0; i -= 1) {
      setSpeechText(text.slice(0, i))
      await sleep(speed)
    }
  }, [])

  async function playLoginSuccessFlow() {
    await eraseText('要使用需要先登录哦')
    await typeText('猫猜人是第一次加入我们吧')
    await sleep(300)
    await eraseText('猫猜人是第一次加入我们吧')
    await typeText('让猫带人先了解下吧')
    await sleep(250)
    setLoginOut(true)
    await sleep(400)
    setLoginMounted(false)
    setAckMounted(true)
  }

  async function handleAckClick() {
    if (ackDisabled) {
      return
    }
    setAckDisabled(true)
    await eraseText('让猫带人先了解下吧')
    await typeText('选择人的昵称和头像吧')
    await sleep(200)
    setAckOut(true)
    await sleep(400)
    setAckMounted(false)
    setProfileMounted(true)
  }

  function handleHelloClick() {
    if (helloDisabled) {
      return
    }
    void (async () => {
      setHelloDisabled(true)
      await eraseText('你好喵')
      await typeText('要使用需要先登录哦')
      await sleep(250)
      setHelloOut(true)
      await sleep(400)
      setHelloMounted(false)
      setLoginMounted(true)
    })()
  }

  useEffect(() => {
    void typeText('你好喵')
  }, [typeText])

  /* 冷启动时若本地已有登录态, 静默续登并直接进入主界面 */
  useEffect(() => {
    async function resumeSession() {
      if (!useAuthStore.getState().token) {
        return
      }
      try {
        const session = await silentLogin()
        if (session.family && session.identity !== 'guest') {
          await reLaunch(routes.tabs.calendar)
        }
      }
      catch {
        /* 静默续登失败(登录过期等), 留在本页走正常登录流程 */
      }
    }
    void resumeSession()
  }, [])

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
      void playProfileSubmitFlow()
    }
    catch {
      setSubmitting(false)
      await Taro.showToast({ title: '保存失败，请重试', icon: 'none' })
    }
  }

  async function playProfileSubmitFlow() {
    setProfileSubmitted(true)
    await typeText('人可以选择创建自己的家庭，或者加入已有的家庭哦')
    await sleep(250)
    setAvatarOut(true)
    setFieldsOut(true)
    await sleep(400)
    setProfileMounted(false)
    setFamilyStep('choice')
  }

  async function handleCreateFamily() {
    const value = familyName.trim()
    if (value.length < 1 || value.length > 50) {
      await Taro.showToast({ title: '家庭名称长度需为 1-50 个字符', icon: 'none' })
      return
    }
    if (familySubmitting) {
      return
    }
    setFamilySubmitting(true)
    try {
      const family = await createFamily({ name: value })
      useFamilyStore.getState().setFamilyDetail(family)
      const user = useAuthStore.getState().user
      if (user) {
        useAuthStore.getState().setUserProfile(user, { id: family.id, name: family.name }, 'owner')
      }
      await Taro.showToast({ title: '家庭已创建', icon: 'success' })
      /* 首次登录悬浮猫指引: 建完家庭进入日历页后开始第 7-8 轮跨页指引 */
      Taro.setStorageSync('pet-first-guide', 'calendar')
      await reLaunch(routes.pages.home)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '创建失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setFamilySubmitting(false)
    }
  }

  function handleBackToFamilyChoice() {
    setFamilyStep('choice')
    setFamilyName('')
    setFamilyCode('')
  }

  function handleSkip() {
    void switchTab(routes.tabs.calendar)
  }

  async function handleJoinFamily() {
    const value = familyCode.trim().toUpperCase()
    if (value.length < 4 || value.length > 32) {
      await Taro.showToast({ title: '家庭码长度无效', icon: 'none' })
      return
    }
    if (familySubmitting) {
      return
    }
    setFamilySubmitting(true)
    try {
      await applyJoinFamily({ code: value })
      await Taro.showToast({ title: '申请已提交', icon: 'success' })
      await navigateTo(routes.pages.pendingFamily)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '申请失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setFamilySubmitting(false)
    }
  }

  return (
    <View className="profile-onboarding">
      <Image className="profile-onboarding__bg" src={bg1Image} mode="aspectFill" />
      <Image className="profile-onboarding__cat" src={profileSubmitted || !loginCompleted ? catImage : cat2Image} mode="scaleToFill" />
      <Image className="profile-onboarding__logo" src={logoImage} mode="scaleToFill" />
      <View className="profile-onboarding__bubble">
        <Text className="profile-onboarding__bubble-text">{speechText}</Text>
      </View>
      {helloMounted && (
        <Button
          className={`profile-onboarding__hello${helloOut ? ' profile-onboarding__hello--out' : ''}${helloDisabled ? ' profile-onboarding__hello--disabled' : ''}`}
          onClick={handleHelloClick}
        >
          你好
        </Button>
      )}

      {helloMounted && (
        <Button className="profile-onboarding__skip" onClick={handleSkip}>
          跳过
        </Button>
      )}

      {loginMounted && (
        <Button
          className={`profile-onboarding__auth${loginOut ? ' profile-onboarding__auth--out' : ''}${authDisabled ? ' profile-onboarding__auth--disabled' : ''}`}
          onClick={handleLogin}
        >
          授权微信登录
        </Button>
      )}

      {ackMounted && (
        <Button
          className={`profile-onboarding__hello${ackOut ? ' profile-onboarding__hello--out' : ''}${ackDisabled ? ' profile-onboarding__hello--disabled' : ''}`}
          onClick={handleAckClick}
        >
          好哦
        </Button>
      )}

      {profileMounted && (
        <View className="profile-onboarding__profile">
          <Button
            className={`profile-onboarding__avatar-button${avatarOut ? ' profile-onboarding__avatar-button--out-left' : ''}`}
            disabled={choosingAvatar || uploading || submitting || Boolean(avatarPath && !avatarAssetID)}
            loading={choosingAvatar || uploading || (submitting && !profileSubmitted)}
            openType="chooseAvatar"
            onClick={handleAvatarClick}
            onChooseAvatar={handleChooseAvatar}
            onError={handleChooseAvatarError}
          >
            {avatarPath ? <Image className="profile-onboarding__avatar" src={avatarPath} /> : '选择头像'}
          </Button>
          <View className={`profile-onboarding__profile-fields${fieldsOut ? ' profile-onboarding__profile-fields--out-right' : ''}`}>
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
              disabled={!loginCompleted || uploading || submitting}
              loading={submitting && !profileSubmitted}
              onClick={handleSubmit}
            >
              保存资料
            </Button>
          </View>
        </View>
      )}

      {familyStep === 'choice' && (
        <View className="profile-onboarding__family">
          <Button
            className="profile-onboarding__create-btn"
            onClick={() => setFamilyStep('create')}
          >
            创建家庭
          </Button>
          <Button
            className="profile-onboarding__join-btn"
            onClick={() => setFamilyStep('join')}
          >
            加入家庭
          </Button>
        </View>
      )}

      {familyStep === 'choice' && (
        <Button className="profile-onboarding__skip" onClick={handleSkip}>
          跳过
        </Button>
      )}

      {familyStep === 'create' && (
        <View className="profile-onboarding__family-form">
          <Input
            className="profile-onboarding__input"
            maxlength={50}
            placeholder="请输入家庭名称"
            value={familyName}
            onInput={event => setFamilyName(event.detail.value)}
          />
          <Button
            className="profile-onboarding__family-submit"
            disabled={familySubmitting}
            loading={familySubmitting}
            onClick={handleCreateFamily}
          >
            创建家庭
          </Button>
          <Button
            className="profile-onboarding__family-back"
            onClick={handleBackToFamilyChoice}
          >
            返回
          </Button>
        </View>
      )}

      {familyStep === 'join' && (
        <View className="profile-onboarding__family-form">
          <Input
            className="profile-onboarding__input"
            maxlength={32}
            placeholder="请输入家庭码"
            value={familyCode}
            onInput={event => setFamilyCode(event.detail.value.toUpperCase())}
          />
          <Button
            className="profile-onboarding__family-submit"
            disabled={familySubmitting}
            loading={familySubmitting}
            onClick={handleJoinFamily}
          >
            提交申请
          </Button>
          <Button
            className="profile-onboarding__family-back"
            onClick={handleBackToFamilyChoice}
          >
            返回
          </Button>
        </View>
      )}
    </View>
  )
}
