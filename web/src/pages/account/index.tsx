import { Button, Image, Text, View } from '@tarojs/components'
import PageBackground from '../../components/page-background'
import { routes } from '../../constants/routes'
import { authorizedAssetURL } from '../../services/request'
import { useAuthStore } from '../../stores/auth-store'
import { useFamilyStore } from '../../stores/family-store'
import { navigateTo, switchTab } from '../../utils/navigation'
import './index.scss'

const identityLabels = {
  guest: '未加入家庭',
  member: '家庭成员',
  owner: '家庭拥有者',
} as const

export default function Account() {
  const { user, identity } = useAuthStore()
  const { family } = useFamilyStore()

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
      </View>
      <View className="account-section">
        <Text className="account-section__title">家庭信息</Text>
        {family
? (
          <View className="account-family">
            <Text className="account-family__name">{family.name}</Text>
            <Button onClick={() => navigateTo(routes.pages.familyMembers)}>查看家庭成员</Button>
          </View>
        )
: (
          <View className="account-family account-family--empty">
            <Text>还没有加入家庭</Text>
            <View className="account-family__actions">
              <Button onClick={() => navigateTo(routes.pages.createFamily)}>创建家庭</Button>
              <Button onClick={() => navigateTo(routes.pages.joinFamily)}>加入家庭</Button>
              <Button onClick={() => navigateTo(routes.pages.pendingFamily)}>查看申请</Button>
            </View>
          </View>
        )}
      </View>
      <Button className="account-back-home" onClick={() => switchTab(routes.tabs.calendar)}>返回日历</Button>
    </View>
  )
}
