import { Button, Text, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useState } from 'react'
import PageBackground from '../../../components/page-background'
import { routes } from '../../../constants/routes'
import { getMyJoinApplication } from '../../../services/family'
import { getMe } from '../../../services/user'
import { navigateTo, reLaunch } from '../../../utils/navigation'

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
        await reLaunch(routes.pages.home)
      }
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载申请状态失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  if (status === 'pending') {
    return (
      <View className="themed-page">
        <PageBackground />
        <Text>等待家庭拥有者审核</Text>
        <Button loading={loading} onClick={load}>刷新状态</Button>
      </View>
    )
  }

  if (status === 'rejected') {
    return (
      <View className="themed-page">
        <PageBackground />
        <Text>申请未通过</Text>
        <Button onClick={() => navigateTo(routes.pages.joinFamily)}>重新申请</Button>
      </View>
    )
  }

  return (
    <View className="themed-page">
      <PageBackground />
      <Text>暂无加入申请</Text>
      <Button onClick={() => navigateTo(routes.pages.joinFamily)}>申请加入家庭</Button>
    </View>
  )
}
