import type { FamilyMember, JoinApplication } from '../../../types/family'
import { Button, Image, Text, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useState } from 'react'
import PageBackground from '../../../components/page-background'
import { approveApplication, getMembers, getPendingApplications, rejectApplication, removeMember } from '../../../services/family'
import { authorizedAssetURL } from '../../../services/request'
import { useAuthStore } from '../../../stores/auth-store'
import { navigateBack } from '../../../utils/navigation'
import './index.scss'

const roleLabels = {
  owner: '拥有者',
  member: '成员',
} as const

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

  async function confirmAction(title: string, content: string, action: () => Promise<void>) {
    const result = await Taro.showModal({ title, content })
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

  const renderAvatar = (avatar: string, nickname: string) => (
    avatar
      ? <Image className="family-member-card__avatar" src={authorizedAssetURL(avatar)} mode="aspectFill" />
      : <View className="family-member-card__avatar family-member-card__avatar--empty">{(nickname || '家').slice(0, 1)}</View>
  )

  return (
    <View className="themed-page">
      <PageBackground />
      <View className="family-members-header">
        <Button className="family-members-back" hoverClass="family-capsule-press" onClick={() => navigateBack()}>返回</Button>
        <Text className="family-members-title">家庭成员</Text>
        <Button className="family-members-refresh" loading={loading} hoverClass="family-capsule-press" onClick={load}>刷新</Button>
      </View>
      <View className="family-members-list">
        {members.map(member => (
          <View className="family-member-card" key={member.id}>
            {renderAvatar(member.avatar, member.nickname)}
            <View className="family-member-card__info">
              <Text className="family-member-card__name">{member.nickname || '未设置昵称'}</Text>
              <Text className={`family-member-card__role${member.role === 'owner' ? ' family-member-card__role--owner' : ''}`}>{roleLabels[member.role]}</Text>
            </View>
            {identity === 'owner' && member.role !== 'owner' && (
              <Button
                className="family-capsule family-capsule--danger"
                hoverClass="family-capsule-press"
                onClick={() => confirmAction('移除成员', `确定将「${member.nickname || '该成员'}」移出家庭吗？`, () => removeMember(member.id))}
              >
                移除
              </Button>
            )}
          </View>
        ))}
        {members.length === 0 && !loading && <Text className="family-members-empty">还没有家庭成员</Text>}
        {identity === 'owner' && applications.length > 0 && (
          <View className="family-applications">
            <Text className="family-applications__title">
              待审申请（
              {applications.length}
              ）
            </Text>
            {applications.map(application => (
              <View className="family-member-card" key={application.id}>
                {renderAvatar(application.avatar, application.nickname)}
                <View className="family-member-card__info">
                  <Text className="family-member-card__name">{application.nickname || '未设置昵称'}</Text>
                  <Text className="family-member-card__role">申请加入</Text>
                </View>
                <View className="family-member-card__actions">
                  <Button
                    className="family-capsule"
                    hoverClass="family-capsule-press"
                    onClick={() => confirmAction('同意申请', `同意「${application.nickname || '该用户'}」加入家庭吗？`, () => approveApplication(application.id))}
                  >
                    同意
                  </Button>
                  <Button
                    className="family-capsule family-capsule--danger"
                    hoverClass="family-capsule-press"
                    onClick={() => confirmAction('拒绝申请', `拒绝「${application.nickname || '该用户'}」的加入申请吗？`, () => rejectApplication(application.id))}
                  >
                    拒绝
                  </Button>
                </View>
              </View>
            ))}
          </View>
        )}
      </View>
    </View>
  )
}
