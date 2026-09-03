/**
 * 家庭中心 —— 完全复刻文件二（Pet-Manual）家庭中心视觉：
 * 全屏滑入页 create-screen（module-header + family-center-hero + 家庭信息/家庭宠物/家庭成员三 section）
 * + 成员管理抽屉 members-sheet（全部成员 / 加入申请 tabs）+ 移除确认 confirm-dialog。
 *
 * 数据逻辑走文件一真实后端（getMembers / getPendingApplications / approveApplication /
 * rejectApplication / removeMember / 复制家庭码）；视觉类名复用 pet-manual/proto.scss
 * 中文件二的样式（已按 ×750/390 放大为 rpx）。
 */
import type { FamilyMember, JoinApplication } from '../../../types/family'
import { Button, Image, Text, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useState } from 'react'
import {
  approveApplication,
  getMembers,
  getPendingApplications,
  rejectApplication,
  removeMember,
} from '../../../services/family'
import * as PetAPI from '../../../pet-manual/services/pet'
import { buildAssetUrl } from '../../../pet-manual/services/asset'
import { useAuthStore } from '../../../stores/auth-store'
import { useFamilyStore } from '../../../stores/family-store'
import { navigateBack } from '../../../utils/navigation'
import '../../../pet-manual/styles/pet-manual.scss'
import '../family.scss'

type MembersTab = 'list' | 'applications'

interface FamilyPet {
  id: string
  name: string
  breed: string
  avatar: string
}

export default function FamilyMembers() {
  const identity = useAuthStore(state => state.identity)
  const family = useFamilyStore(state => state.family)
  const detail = useFamilyStore(state => state.detail)
  const [pets, setPets] = useState<FamilyPet[]>([])
  const [members, setMembers] = useState<FamilyMember[]>([])
  const [applications, setApplications] = useState<JoinApplication[]>([])
  const [loading, setLoading] = useState(false)
  const [membersOpen, setMembersOpen] = useState(false)
  const [membersTab, setMembersTab] = useState<MembersTab>('list')
  const [confirmRemove, setConfirmRemove] = useState<FamilyMember | null>(null)

  const familyName = family?.name || detail?.name || '我们的家'
  const joinCode = detail?.join_code || ''
  const memberCount = members.length
  const petCount = pets.length

  async function load() {
    setLoading(true)
    try {
      const memberList = await getMembers()
      setMembers(memberList)
      if (identity === 'owner') {
        setApplications(await getPendingApplications())
      } else {
        setApplications([])
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : '加载家庭成员失败'
      await Taro.showToast({ title: message, icon: 'none' })
    } finally {
      setLoading(false)
    }
  }

  async function loadPets() {
    try {
      const apiPets = await PetAPI.getPets()
      if (!apiPets || apiPets.length === 0) {
        setPets([])
        return
      }
      // 并行拉取富档案（品种/头像），失败退化为最小信息
      const profiles = await Promise.allSettled(apiPets.map(p => PetAPI.getPetProfile(p.id)))
      const list: FamilyPet[] = apiPets.map((api, i) => {
        const pr = profiles[i]
        const profile = pr.status === 'fulfilled' ? pr.value : null
        return {
          id: api.id,
          name: api.name,
          breed: profile?.breed || '',
          avatar: buildAssetUrl(profile?.avatar_asset_id) || '',
        }
      })
      setPets(list)
    } catch {
      setPets([])
    }
  }

  useDidShow(() => {
    void load()
    void loadPets()
  })

  function avatarText(member: FamilyMember) {
    return member.nickname ? member.nickname.slice(0, 1) : '宠'
  }

  function memberMeta(member: FamilyMember) {
    if (member.role === 'owner') return '家长'
    return '成员'
  }

  async function copyCode() {
    if (!joinCode) return
    await Taro.setClipboardData({ data: joinCode })
    await Taro.showToast({ title: '家庭码已复制', icon: 'success' })
  }

  async function handleApprove(id: string) {
    try {
      await approveApplication(id)
      await load()
      await Taro.showToast({ title: '已通过申请', icon: 'success' })
    } catch (error) {
      const message = error instanceof Error ? error.message : '操作失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
  }

  async function handleReject(id: string) {
    try {
      await rejectApplication(id)
      await load()
      await Taro.showToast({ title: '已拒绝申请', icon: 'success' })
    } catch (error) {
      const message = error instanceof Error ? error.message : '操作失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
  }

  async function handleRemove() {
    if (!confirmRemove) return
    const target = confirmRemove
    setConfirmRemove(null)
    try {
      await removeMember(target.id)
      await load()
      await Taro.showToast({ title: '已移除成员', icon: 'success' })
    } catch (error) {
      const message = error instanceof Error ? error.message : '操作失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
  }

  const pendingCount = applications.length

  return (
    <View className='app family-app' id='app'>
      {/* 家庭中心全屏滑入页 */}
      <View className='create-screen open' id='familyScreen'>
        <View className='module-header'>
          <Button className='create-back' onClick={() => navigateBack()}>‹</Button>
          <Text className='h1'>家庭</Text>
        </View>
        <View className='family-center-content'>
          <View className='family-center-hero'>
            <Text className='h1' id='familyCenterName'>{familyName}</Text>
            <Text className='p' id='familyCenterSubtitle'>{memberCount} 位家人 · {petCount} 只宠物</Text>
          </View>

          <View id='familyJoinedView'>
            <View className='family-center-section'>
              <Text className='h3'>家庭信息</Text>
              <View className='family-code-row'>
                <Text className='span'>家庭码</Text>
                <Text className='strong' id='familyCodeValue'>{joinCode}</Text>
              </View>
              <View className='family-code-actions'>
                <Button className='secondary-button' onClick={copyCode}>复制家庭码</Button>
                <Button className='secondary-button' onClick={() => Taro.showToast({ title: '把家庭码告诉家人，等待他们加入', icon: 'none' })}>邀请家人</Button>
              </View>
            </View>

            <View className='family-center-section'>
              <Text className='h3'>家庭宠物</Text>
              {pets.map(pet => (
                <View key={pet.id} className='family-list-row'>
                  <View className='family-list-person'>
                    <View className={`avatar-placeholder${pet.avatar ? ' has-photo' : ''}`}>
                      {pet.avatar
                        ? <Image className='avatar-photo' src={pet.avatar} mode='aspectFill' />
                        : (pet.name ? pet.name.slice(0, 1) : '宠')}
                    </View>
                    <Text className='strong'>{pet.name}</Text>
                  </View>
                  <Text className='span'>{pet.breed || '宠物'}</Text>
                </View>
              ))}
              {petCount === 0 && <Text className='family-empty p'>还没有宠物</Text>}
            </View>

            <View className='family-center-section'>
              <Text className='h3'>家庭成员</Text>
              <View className='family-member-preview'>
                {members.slice(0, 3).map(member => (
                  <View key={member.id} className='family-member-mini'>
                    <View className='avatar-placeholder'>{avatarText(member)}</View>
                    <Text className='strong'>{member.nickname || '未设置昵称'}</Text>
                  </View>
                ))}
                {members.length === 0 && <Text className='family-empty p'>还没有成员</Text>}
              </View>
              <Button
                className='secondary-button management-button'
                onClick={() => {
                  setMembersOpen(true)
                  setMembersTab(pendingCount > 0 ? 'applications' : 'list')
                }}
              >
                管理成员与加入申请
                {pendingCount > 0 && <Text className='management-dot' />}
              </Button>
            </View>
          </View>
        </View>
      </View>

      {/* 成员管理抽屉 */}
      <View className={`overlay${membersOpen ? ' open' : ''}`} id='membersOverlay' onClick={() => setMembersOpen(false)}>
        <View className='sheet sheet-tall members-sheet' onClick={e => e.stopPropagation()}>
          <View className='handle' />
          <View className='sheet-head'>
            <View><Text className='h2'>成员与申请</Text><Text className='p'>管理家庭成员和处理加入申请</Text></View>
            <Button className='close-button' onClick={() => setMembersOpen(false)}>×</Button>
          </View>
          <View className='members-tabs'>
            <Button className={`members-tab${membersTab === 'list' ? ' active' : ''}`} onClick={() => setMembersTab('list')}>
              <Text className='span'>全部成员</Text>
            </Button>
            <Button className={`members-tab${membersTab === 'applications' ? ' active' : ''}`} onClick={() => setMembersTab('applications')}>
              <Text className='span'>加入申请</Text>
              {pendingCount > 0 && <Text className='members-tab-dot' />}
            </Button>
          </View>
          <View className='members-list'>
            {membersTab === 'list' && members.map(member => (
              <View key={member.id} className='member-row status-active'>
                <View className='avatar-placeholder'>{avatarText(member)}</View>
                <View className='member-info'>
                  <View className='member-name-row'>
                    <Text className='strong'>{member.nickname || '未设置昵称'}</Text>
                  </View>
                  <View className='member-meta'>{memberMeta(member)}</View>
                </View>
                <View className='member-actions'>
                  {identity === 'owner' && member.role !== 'owner' && (
                    <Button className='member-action reject' onClick={() => setConfirmRemove(member)}>移除</Button>
                  )}
                </View>
              </View>
            ))}
            {membersTab === 'list' && members.length === 0 && (
              <View className='members-empty'>{loading ? '加载中…' : '还没有成员'}</View>
            )}

            {membersTab === 'applications' && applications.map(application => (
              <View key={application.id} className='member-row status-pending'>
                <View className='avatar-placeholder'>{application.nickname ? application.nickname.slice(0, 1) : '宠'}</View>
                <View className='member-info'>
                  <View className='member-name-row'>
                    <Text className='strong'>{application.nickname || '未设置昵称'}</Text>
                  </View>
                  <View className='member-meta'>申请加入 · 待审核</View>
                </View>
                <View className='member-actions'>
                  <Button className='member-action approve' onClick={() => handleApprove(application.id)}>通过</Button>
                  <Button className='member-action reject' onClick={() => handleReject(application.id)}>拒绝</Button>
                </View>
              </View>
            ))}
            {membersTab === 'applications' && applications.length === 0 && (
              <View className='members-empty'>没有待处理的申请</View>
            )}
          </View>
        </View>
      </View>

      {/* 移除确认弹窗 */}
      {confirmRemove && (
        <View className='confirm-overlay' onClick={() => setConfirmRemove(null)}>
          <View className='confirm-dialog' onClick={e => e.stopPropagation()}>
            <Text className='h3'>移除家庭成员</Text>
            <Text className='p'>确定要将「{confirmRemove.nickname || '未设置昵称'}」从家庭中移除吗？移除后将不再共享家庭档案。</Text>
            <View className='confirm-actions'>
              <Button className='secondary-button' onClick={() => setConfirmRemove(null)}>取消</Button>
              <Button className='danger-button' onClick={handleRemove}>确认移除</Button>
            </View>
          </View>
        </View>
      )}
    </View>
  )
}
