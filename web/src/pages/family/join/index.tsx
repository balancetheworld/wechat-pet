import { Button, Input, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useState } from 'react'
import PageBackground from '../../../components/page-background'
import { routes } from '../../../constants/routes'
import { applyJoinFamily } from '../../../services/family'
import { navigateTo } from '../../../utils/navigation'

export default function JoinFamily() {
  const [code, setCode] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit() {
    const value = code.trim().toUpperCase()
    if (value.length < 4 || value.length > 32) {
      await Taro.showToast({ title: '家庭码长度无效', icon: 'none' })
      return
    }
    if (submitting) {
      return
    }
    setSubmitting(true)
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
      setSubmitting(false)
    }
  }

  return (
    <View className="themed-page">
      <PageBackground />
      <Text className="themed-page__title">申请加入</Text>
      <Input
        maxlength={32}
        placeholder="请输入家庭码"
        value={code}
        onInput={event => setCode(event.detail.value.toUpperCase())}
      />
      <Button loading={submitting} disabled={submitting} onClick={handleSubmit}>提交申请</Button>
    </View>
  )
}
