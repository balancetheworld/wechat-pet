import type { FamilyMember, JoinApplication } from '../../types/family'
import { Button, Image, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useEffect, useRef, useState } from 'react'
import PageBackground from '../../components/page-background'
import { routes } from '../../constants/routes'
import { approveApplication, getCurrentFamily, getMembers, getPendingApplications, rejectApplication, removeMember } from '../../services/family'
import { authorizedAssetURL } from '../../services/request'
import { useAuthStore } from '../../stores/auth-store'
import { useFamilyStore } from '../../stores/family-store'
import { navigateTo, reLaunch, switchTab } from '../../utils/navigation'
import './index.scss'

const identityLabels = {
  guest: '未加入家庭',
  member: '家庭成员',
  owner: '家庭拥有者',
} as const

const roleLabels = {
  owner: '拥有者',
  member: '成员',
} as const

export default function Account() {
  const { user, identity, clearSession } = useAuthStore()
  const { family } = useFamilyStore()
  /* 家庭码: 已加入家庭时拉取详情, 供新成员凭码申请加入 */
  const [joinCode, setJoinCode] = useState('')
  /* 家庭成员: 直接内嵌在本页展示, 不再跳转成员管理页 */
  const [members, setMembers] = useState<FamilyMember[]>([])
  /* 待审申请: 仅拥有者可见, 通过后成员立即进入列表 */
  const [pendingApplications, setPendingApplications] = useState<JoinApplication[]>([])
  /* 成员管理态: 仅拥有者可开启, 开启后每行尾部出现移除小×(自己的行除外) */
  const [managingMembers, setManagingMembers] = useState(false)
  /* 退出登录流程锁: 两次确认期间防止重复触发 */
  const loggingOutRef = useRef(false)

  useEffect(() => {
    if (!family) {
      setJoinCode('')
      return
    }
    let cancelled = false
    getCurrentFamily()
      .then((detail) => {
        if (!cancelled) {
          setJoinCode(detail.join_code || '')
        }
      })
      .catch(() => {
        /* 拉取失败静默: 家庭码行不显示 */
      })
    return () => {
      cancelled = true
    }
  }, [family])

  useEffect(() => {
    if (!family) {
      setMembers([])
      setPendingApplications([])
      return
    }
    let cancelled = false
    getMembers()
      .then((list) => {
        if (!cancelled) {
          setMembers(Array.isArray(list) ? list : [])
        }
      })
      .catch(() => {
        /* 拉取失败静默: 成员列表显示占位文案 */
      })
    if (identity === 'owner') {
      getPendingApplications()
        .then((list) => {
          if (!cancelled) {
            setPendingApplications(Array.isArray(list) ? list : [])
          }
        })
        .catch(() => {
          /* 拉取失败静默 */
        })
    }
    return () => {
      cancelled = true
    }
  }, [family, identity])

  function copyJoinCode() {
    if (!joinCode) {
      return
    }
    Taro.setClipboardData({ data: joinCode })
  }

  async function handleApprove(applicationID: string) {
    try {
      await approveApplication(applicationID)
      setPendingApplications(previous => previous.filter(item => item.id !== applicationID))
      const list = await getMembers()
      setMembers(Array.isArray(list) ? list : [])
      await Taro.showToast({ title: '已通过申请', icon: 'success' })
    }
    catch {
      await Taro.showToast({ title: '操作失败,请重试', icon: 'none' })
    }
  }

  async function handleReject(applicationID: string) {
    try {
      await rejectApplication(applicationID)
      setPendingApplications(previous => previous.filter(item => item.id !== applicationID))
      await Taro.showToast({ title: '已拒绝申请', icon: 'none' })
    }
    catch {
      await Taro.showToast({ title: '操作失败,请重试', icon: 'none' })
    }
  }

  /* 移除成员: 确认弹窗 → 后端 remove(仅拥有者可调, 且不能移除自己) → 刷新列表 */
  async function handleRemoveMember(member: FamilyMember) {
    const confirmed = await Taro.showModal({
      title: '移除成员',
      content: `确定要将「${member.nickname || '该成员'}」移出家庭吗？移出后 TA 将无法查看宠物档案与日历`,
      cancelText: '取消',
      confirmText: '移除',
    })
    if (!confirmed.confirm) {
      return
    }
    try {
      await removeMember(member.id)
      const list = await getMembers()
      setMembers(Array.isArray(list) ? list : [])
      await Taro.showToast({ title: '已移除', icon: 'success' })
      /* 全部成员被移除后自动退出管理态 */
      if (!Array.isArray(list) || list.filter(item => item.user_id !== user?.id).length === 0) {
        setManagingMembers(false)
      }
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '移除失败,请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
  }

  /* 退出登录: 两次确认 (产品要求), 并在两次确认期间加锁防止重复触发 */
  async function handleLogout() {
    if (loggingOutRef.current) {
      return
    }
    loggingOutRef.current = true
    try {
      const first = await Taro.showModal({
        title: '退出登录',
        content: '确定要退出当前账号吗？',
        cancelText: '取消',
        confirmText: '下一步',
      })
      if (!first.confirm) {
        return
      }
      const second = await Taro.showModal({
        title: '再次确认',
        content: '退出后需要重新登录才能查看宠物档案与日历，确定要退出吗？',
        cancelText: '取消',
        confirmText: '确认退出',
      })
      if (!second.confirm) {
        return
      }
      await clearSession()
      /* 退出后回到登录引导页 (重新走登录动画 → 昵称头像 → 创建/加入家庭),
         不再跳到旧版首页 /pages/index/index */
      await reLaunch(routes.pages.profileOnboarding)
    }
    finally {
      loggingOutRef.current = false
    }
  }

  return (
    <View className="account-page">
      <PageBackground />
      <View className="account-header">
        <Text className="account-header__title">账户与家庭</Text>
      </View>
      <View className="account-user">
        {user?.avatarUrl ? <Image className="account-user__avatar" src={authorizedAssetURL(user.avatarUrl)} mode="aspectFill" /> : <View className="account-user__avatar account-user__avatar--empty">头像</View>}
        <View className="account-user__info">
          <Text className="account-user__name">{user?.nickname || '未设置昵称'}</Text>
          <Text className="account-user__identity">{identityLabels[identity]}</Text>
        </View>
        {/* 家庭名放在用户卡右侧, 家庭信息卡内不再重复展示 */}
        {family && <Text className="account-user__family">{family.name}</Text>}
      </View>
      <View className="account-section">
        <Text className="account-section__title">家庭信息</Text>
        {family
? (
          <View className="account-family">
            <View className="account-code">
              <Text className="account-family__code-label">家庭码</Text>
              <View className="account-code__box">
                <Text className="account-code__value">{joinCode}</Text>
              </View>
              <Button className="account-code__copy" hoverClass="account-press" onClick={copyJoinCode}>复制</Button>
            </View>
          </View>
        )
: (
          <View className="account-family account-family--empty">
            <Text>还没有加入家庭</Text>
            <View className="account-family__actions">
              <Button className="account-btn" hoverClass="account-press" onClick={() => navigateTo(routes.pages.createFamily)}>创建家庭</Button>
              <Button className="account-btn" hoverClass="account-press" onClick={() => navigateTo(routes.pages.joinFamily)}>加入家庭</Button>
              <Button className="account-btn" hoverClass="account-press" onClick={() => navigateTo(routes.pages.pendingFamily)}>查看申请</Button>
            </View>
          </View>
        )}
      </View>
      {family && (
        <View className="account-section">
          <View className="account-section__head">
            <Text className="account-section__title">家庭成员</Text>
            {identity === 'owner' && (
              <Text
                className={`account-members__manage${managingMembers ? ' account-members__manage--active' : ''}`}
                onClick={() => setManagingMembers(previous => !previous)}
              >
                {managingMembers ? '完成' : '管理'}
              </Text>
            )}
          </View>
          <View className="account-members">
            {members.map(member => (
              <View className="account-member" key={member.id}>
                {member.avatar
                  ? <Image className="account-member__avatar" src={authorizedAssetURL(member.avatar)} mode="aspectFill" />
                  : <View className="account-member__avatar account-member__avatar--empty">{(member.nickname || '成').slice(0, 1)}</View>}
                <Text className="account-member__name">{member.nickname || '成员'}</Text>
                <Text className={`account-member__role${member.role === 'owner' ? ' account-member__role--owner' : ''}`}>{roleLabels[member.role]}</Text>
                {managingMembers && member.user_id !== user?.id && (
                  <Text
                    className="account-member__del"
                    onClick={() => void handleRemoveMember(member)}
                  >
                    ×
                  </Text>
                )}
              </View>
            ))}
            {members.length === 0 && <View className="account-members__empty">暂无成员信息</View>}
            {identity === 'owner' && pendingApplications.length > 0 && (
              <View className="account-pending">
                <Text className="account-pending__title">待审核申请</Text>
                {pendingApplications.map(application => (
                  <View className="account-member" key={application.id}>
                    {application.avatar
                      ? <Image className="account-member__avatar" src={authorizedAssetURL(application.avatar)} mode="aspectFill" />
                      : <View className="account-member__avatar account-member__avatar--empty">{(application.nickname || '申').slice(0, 1)}</View>}
                    <Text className="account-member__name">{application.nickname || '申请人'}</Text>
                    <View className="account-pending__actions">
                      <Button className="account-pending__btn account-pending__btn--ok" hoverClass="account-press" onClick={() => void handleApprove(application.id)}>通过</Button>
                      <Button className="account-pending__btn account-pending__btn--no" hoverClass="account-press" onClick={() => void handleReject(application.id)}>拒绝</Button>
                    </View>
                  </View>
                ))}
              </View>
            )}
          </View>
        </View>
      )}
      <View className="account-footer">
        <Button className="account-btn account-btn--wide" hoverClass="account-press" onClick={() => switchTab(routes.tabs.calendar)}>返回日历</Button>
        <Button className="account-btn account-btn--wide account-btn--danger" hoverClass="account-press" onClick={() => void handleLogout()}>退出登录</Button>
      </View>
    </View>
  )
}
