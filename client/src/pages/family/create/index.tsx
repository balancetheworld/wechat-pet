/**
 * 创建家庭 —— 完全复刻文件二（Pet-Manual）onboarding 问询向导视觉：
 * 两步（家庭名称 1/2 → 你的信息 2/2），带 onboarding-header(back + 标题) +
 * onboarding-progress + onboarding-summary + capsule-input + primary-button。
 *
 * 数据逻辑走文件一真实后端（createFamily + 可选 updateProfile 昵称）；
 * 视觉类名复用 pet-manual/proto.scss 中文件二样式（已放大为 rpx）。
 */
import { Button, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useState } from 'react'
import { routes } from '../../../constants/routes'
import { createFamily } from '../../../services/family'
import { updateProfile } from '../../../services/user'
import { useAuthStore } from '../../../stores/auth-store'
import { useFamilyStore } from '../../../stores/family-store'
import { navigateBack, switchTab } from '../../../utils/navigation'
import '../../../pet-manual/styles/pet-manual.scss'
import '../family.scss'

type Step = 'create-family' | 'create-profile'

const STEP_SETTINGS: Record<Step, { title: string; progress: string }> = {
  'create-family': { title: '创建家庭', progress: '1 / 2' },
  'create-profile': { title: '创建家庭', progress: '2 / 2' },
}

export default function CreateFamily() {
  const [step, setStep] = useState<Step>('create-family')
  const [familyName, setFamilyName] = useState('')
  const [profileName, setProfileName] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const setting = STEP_SETTINGS[step]

  function goBack() {
    if (step === 'create-profile') {
      setStep('create-family')
      return
    }
    navigateBack()
  }

  async function handleCreate() {
    const name = familyName.trim()
    if (!name) return
    const memberName = profileName.trim()
    if (submitting) return
    setSubmitting(true)
    try {
      const family = await createFamily({ name })
      // 若填写了「你的信息」，一并更新当前用户昵称（头像由完善资料流程单独维护）
      if (memberName) {
        try {
          await updateProfile({ nickname: memberName, avatar_asset_id: '' })
        } catch {
          // 昵称更新失败不影响家庭创建结果
        }
      }
      // 同步身份：创建者即家长（owner），并写入家庭详情
      useFamilyStore.getState().setFamilyDetail(family)
      const user = useAuthStore.getState().user
      useAuthStore.getState().setUserProfile(
        user ? { ...user, nickname: memberName || user.nickname } : { id: '', nickname: memberName, avatarUrl: undefined },
        { id: family.id, name: family.name },
        'owner',
      )
      await Taro.showToast({ title: '家庭已创建', icon: 'success' })
      await switchTab(routes.tabs.home)
    } catch (error) {
      const message = error instanceof Error ? error.message : '创建失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <View className='app onboarding-root' id='app'>
      <View className='onboarding-screen' id='onboardingScreen'>
        <View className='onboarding-header'>
          <Button className='onboarding-back' onClick={goBack}>‹</Button>
          <Text className='h2'>{setting.title}</Text>
        </View>
        <View className='onboarding-body'>
          <Text className='onboarding-progress span'>{setting.progress}</Text>

          {step === 'create-family' && (
            <View className='onboarding-step'>
              <Text className='h1'>创建家庭</Text>
              <Text className='onboarding-lead p'>填写家人共同看到的家庭名称。</Text>
              <View className='onboarding-form'>
                <View className='form-field'>
                  <Text className='label'>家庭名称</Text>
                  <Input
                    className='capsule-input'
                    maxlength={50}
                    placeholder='例如：我们家'
                    value={familyName}
                    onInput={e => setFamilyName(e.detail.value)}
                  />
                </View>
                <Button className='primary-button' disabled={!familyName.trim()} onClick={() => setStep('create-profile')}>下一步</Button>
              </View>
            </View>
          )}

          {step === 'create-profile' && (
            <View className='onboarding-step'>
              <Text className='h1'>你的信息</Text>
              <Text className='onboarding-lead p'>家人会通过这个名字识别你留下的记录。</Text>
              <View className='onboarding-form'>
                <View className='onboarding-summary'>家庭：{familyName.trim()}</View>
                <View className='form-field'>
                  <Text className='label'>名字</Text>
                  <Input
                    className='capsule-input'
                    maxlength={50}
                    placeholder='请输入你的名字'
                    value={profileName}
                    onInput={e => setProfileName(e.detail.value)}
                  />
                </View>
                <Button className='primary-button' loading={submitting} disabled={submitting} onClick={handleCreate}>创建家庭</Button>
              </View>
            </View>
          )}
        </View>
      </View>
    </View>
  )
}
