import { Button, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useState } from 'react'
import PageBackground from '../../../components/page-background'
import { routes } from '../../../constants/routes'
import { createFamily } from '../../../services/family'
import { useAuthStore } from '../../../stores/auth-store'
import { useFamilyStore } from '../../../stores/family-store'
import { reLaunch } from '../../../utils/navigation'

export default function CreateFamily() {
  const [name, setName] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit() {
    const value = name.trim()
    if (value.length < 1 || value.length > 50) {
      await Taro.showToast({ title: '家庭名称长度需为 1-50 个字符', icon: 'none' })
      return
    }
    if (submitting) {
      return
    }
    setSubmitting(true)
    try {
      const family = await createFamily({ name: value })
      useFamilyStore.getState().setFamilyDetail(family)
      const user = useAuthStore.getState().user
      if (user) {
        useAuthStore.getState().setUserProfile(user, { id: family.id, name: family.name }, 'owner')
      }
      await Taro.showToast({ title: '家庭已创建', icon: 'success' })
      await reLaunch(routes.tabs.calendar)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '创建失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <View className="themed-page">
      <PageBackground />
      <Text className="themed-page__title">创建家庭</Text>
      <Input
        maxlength={50}
        placeholder="请输入家庭名称"
        value={name}
        onInput={event => setName(event.detail.value)}
      />
      <Button loading={submitting} disabled={submitting} onClick={handleSubmit}>创建</Button>
    </View>
  )
}
