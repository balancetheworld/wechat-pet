/**
 * 加入家庭 —— 完全复刻文件二（Pet-Manual）onboarding 问询向导视觉：
 * 两步（家庭码 1/2 → 你的信息 2/2），带 onboarding-header(back + 标题) +
 * onboarding-progress + onboarding-summary + capsule-input + primary-button。
 *
 * 数据逻辑走文件一真实后端（applyJoinFamily + 可选 updateProfile 昵称）；
 * 视觉类名复用 pet-manual/proto.scss 中文件二样式（已放大为 rpx）。
 */
import { Button, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useState } from 'react'
import { routes } from '../../../constants/routes'
import { applyJoinFamily } from '../../../services/family'
import { updateProfile } from '../../../services/user'
import { navigateBack, navigateTo } from '../../../utils/navigation'
import '../../../pet-manual/styles/pet-manual.scss'
import '../family.scss'

type Step = 'join-code' | 'join-profile'

const STEP_SETTINGS: Record<Step, { title: string; progress: string }> = {
  'join-code': { title: '加入家庭', progress: '1 / 2' },
  'join-profile': { title: '加入家庭', progress: '2 / 2' },
}

export default function JoinFamily() {
  const [step, setStep] = useState<Step>('join-code')
  const [code, setCode] = useState('')
  const [profileName, setProfileName] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const setting = STEP_SETTINGS[step]

  function goBack() {
    if (step === 'join-profile') {
      setStep('join-code')
      return
    }
    navigateBack()
  }

  async function handleSubmit() {
    const value = code.trim().toUpperCase()
    const memberName = profileName.trim()
    if (submitting) return
    setSubmitting(true)
    try {
      await applyJoinFamily({ code: value })
      // 若填写了「你的信息」，一并更新当前用户昵称（头像由完善资料流程单独维护）
      if (memberName) {
        try {
          await updateProfile({ nickname: memberName, avatar_asset_id: '' })
        } catch {
          // 昵称更新失败不影响申请结果
        }
      }
      await Taro.showToast({ title: '申请已提交', icon: 'success' })
      await navigateTo(routes.pages.pendingFamily)
    } catch (error) {
      const message = error instanceof Error ? error.message : '申请失败，请重试'
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

          {step === 'join-code' && (
            <View className='onboarding-step'>
              <Text className='h1'>加入家庭</Text>
              <Text className='onboarding-lead p'>向家庭主人获取家庭码并填写。</Text>
              <View className='onboarding-form'>
                <View className='form-field'>
                  <Text className='label'>家庭码</Text>
                  <Input
                    className='capsule-input'
                    maxlength={32}
                    placeholder='请输入家庭码'
                    value={code}
                    onInput={e => setCode(e.detail.value.toUpperCase())}
                  />
                </View>
                <Button className='primary-button' disabled={!code.trim()} onClick={() => setStep('join-profile')}>下一步</Button>
              </View>
            </View>
          )}

          {step === 'join-profile' && (
            <View className='onboarding-step'>
              <Text className='h1'>你的信息</Text>
              <Text className='onboarding-lead p'>提交后，家庭主人会看到你的名字和加入申请。</Text>
              <View className='onboarding-form'>
                <View className='onboarding-summary'>家庭码：{code.trim()}</View>
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
                <Button className='primary-button' loading={submitting} disabled={submitting} onClick={handleSubmit}>提交加入申请</Button>
              </View>
            </View>
          )}
        </View>
      </View>
    </View>
  )
}
