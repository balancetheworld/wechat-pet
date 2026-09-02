import type { FamilyMember, JoinApplication } from '../../../types/family'
import { Button, Text, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useState } from 'react'
import { approveApplication, getMembers, getPendingApplications, rejectApplication, removeMember } from '../../../services/family'
import { useAuthStore } from '../../../stores/auth-store'

export default function FamilyMembers() {
  const identity = useAuthStore(state => state.identity)
  const [members, setMembers] = useState<FamilyMember[]>([])
  const [applications, setApplications] = useState<JoinApplication[]>([])
  const [loading, setLoading] = useState(false)

  async function load() {
    setLoading(true)
    try {
      const memberList = await getMembers()
      setMembers(memberList)
      if (identity === 'owner') {
        setApplications(await getPendingApplications())
      }
      else {
        setApplications([])
      }
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载家庭成员失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  async function confirmAction(title: string, action: () => Promise<void>) {
    const result = await Taro.showModal({ title, content: '确认继续此操作吗？' })
    if (!result.confirm) {
      return
    }
    try {
      await action()
      await load()
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '操作失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
  }

  useDidShow(() => {
    void load()
  })

  return (
    <View>
      <Button loading={loading} onClick={load}>刷新</Button>
      {members.map(member => (
        <View key={member.id}>
          <Text>{member.nickname || '未设置昵称'}</Text>
          <Text>{member.role}</Text>
          {identity === 'owner' && member.role !== 'owner' && (
            <Button onClick={() => confirmAction('移除成员', () => removeMember(member.id))}>移除</Button>
          )}
        </View>
      ))}
      {identity === 'owner' && applications.map(application => (
        <View key={application.id}>
          <Text>{application.nickname || '未设置昵称'}</Text>
          <Button onClick={() => confirmAction('同意申请', () => approveApplication(application.id))}>同意</Button>
          <Button onClick={() => confirmAction('拒绝申请', () => rejectApplication(application.id))}>拒绝</Button>
        </View>
      ))}
    </View>
  )
}
