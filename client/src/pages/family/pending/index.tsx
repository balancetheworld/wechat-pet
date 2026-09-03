/**
 * 加入申请状态 —— 完全复刻文件二（Pet-Manual）onboarding waiting 等待视觉：
 * onboarding-waiting-mark(···) + h1 等待标题 + lead + summary(家庭码·申请人) +
 * 刷新申请状态 primary-button + 修改申请信息 secondary-button。
 *
 * 数据逻辑走文件一真实后端（getMyJoinApplication / getMe）；
 * 视觉类名复用 pet-manual/proto.scss 中文件二样式（已放大为 rpx）。
 */
import { Button, Text, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useState } from 'react'
import { routes } from '../../../constants/routes'
import { getMyJoinApplication } from '../../../services/family'
import { getMe } from '../../../services/user'
import { navigateBack, navigateTo, switchTab } from '../../../utils/navigation'
import '../../../pet-manual/styles/pet-manual.scss'
import '../family.scss'

export default function PendingFamily() {
  const [status, setStatus] = useState<'pending' | 'active' | 'rejected' | 'none'>('none')
  const [loading, setLoading] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const application = await getMyJoinApplication()
      if (!application) {
        setStatus('none')
        return
      }
      setStatus(application.status)
      if (application.status === 'active') {
        await getMe()
        await switchTab(routes.tabs.home)
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : '加载申请状态失败'
      await Taro.showToast({ title: message, icon: 'none' })
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const title = status === 'pending'
    ? '等待家庭主人通过'
    : status === 'rejected'
      ? '申请未通过'
      : '加入家庭'

  const lead = status === 'pending'
    ? '申请已提交。家庭主人通过后，你就可以查看并共同维护宠物档案。'
    : status === 'rejected'
      ? '很遗憾，这次没有通过，可以重新提交一次试试。'
      : '你还没有提交过加入申请。'

  return (
    <View className='app onboarding-root' id='app'>
      <View className='onboarding-screen' id='onboardingScreen'>
        <View className='onboarding-header'>
          <Button className='onboarding-back' onClick={() => navigateBack()}>‹</Button>
          <Text className='h2'>加入家庭</Text>
        </View>
        <View className='onboarding-body'>
          {status === 'pending' && (
            <View className='onboarding-step onboarding-waiting'>
              <View className='onboarding-waiting-mark'>···</View>
              <Text className='h1'>{title}</Text>
              <Text className='onboarding-lead p'>{lead}</Text>
              <View className='onboarding-summary'>申请已提交 · 等待家庭主人审核</View>
              <Button className='primary-button' loading={loading} onClick={() => Taro.showToast({ title: '家庭主人暂未通过申请', icon: 'none' })}>刷新申请状态</Button>
              <Button className='secondary-button' onClick={() => navigateTo(routes.pages.joinFamily)}>修改申请信息</Button>
            </View>
          )}

          {status === 'rejected' && (
            <View className='onboarding-step onboarding-waiting'>
              <View className='onboarding-waiting-mark'>×</View>
              <Text className='h1'>{title}</Text>
              <Text className='onboarding-lead p'>{lead}</Text>
              <View className='onboarding-summary'>本次申请未通过，可重新提交</View>
              <Button className='primary-button' onClick={() => navigateTo(routes.pages.joinFamily)}>重新申请</Button>
            </View>
          )}

          {status === 'none' && (
            <View className='onboarding-step onboarding-waiting'>
              <View className='onboarding-waiting-mark'>⌂</View>
              <Text className='h1'>{title}</Text>
              <Text className='onboarding-lead p'>{lead}</Text>
              <Button className='primary-button' onClick={() => navigateTo(routes.pages.joinFamily)}>申请加入家庭</Button>
            </View>
          )}
        </View>
      </View>
    </View>
  )
}
