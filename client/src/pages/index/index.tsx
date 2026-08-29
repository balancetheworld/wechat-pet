import { Button, Text, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useCallback, useEffect, useState } from 'react'
import { routes } from '../../constants/routes'
import { getPets } from '../../services/pet'
import { useAppStore } from '../../stores/app-store'
import { useAuthStore } from '../../stores/auth-store'
import { useFamilyStore } from '../../stores/family-store'
import { usePetStore } from '../../stores/pet-store'
import { navigateTo, switchTab } from '../../utils/navigation'
import './index.scss'

export default function Index() {
  const { bootstrapCompleted, bootstrapError, retryBootstrap } = useAppStore()
  const { identity, user } = useAuthStore()
  const { family } = useFamilyStore()
  const { pets, setPets } = usePetStore()
  const [loadingPets, setLoadingPets] = useState(false)

  const loadPets = useCallback(async () => {
    if (identity === 'guest' || !bootstrapCompleted) {
      return
    }
    setLoadingPets(true)
    try {
      setPets(await getPets())
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载宠物失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoadingPets(false)
    }
  }, [bootstrapCompleted, identity, setPets])

  useEffect(() => {
    void loadPets()
  }, [loadPets])

  useDidShow(() => {
    void loadPets()
  })

  if (!bootstrapCompleted) {
    return (
      <View className="index">
        <Text>启动中</Text>
      </View>
    )
  }

  if (bootstrapError) {
    return (
      <View className="index">
        <Text>{bootstrapError}</Text>
        <Button onClick={retryBootstrap}>重新登录</Button>
      </View>
    )
  }

  const needsProfileOnboarding = !user?.nickname?.trim() || !user.avatarUrl

  if (needsProfileOnboarding) {
    return (
      <View className="index">
        <Text>请先完善个人资料</Text>
        <Button onClick={() => navigateTo(routes.pages.profileOnboarding)}>完善资料</Button>
      </View>
    )
  }

  if (identity === 'guest') {
    return (
      <View className="index">
        <Text className="title">宠物家庭</Text>
        <Button onClick={() => navigateTo(routes.pages.createFamily)}>创建家庭</Button>
        <Button onClick={() => navigateTo(routes.pages.joinFamily)}>加入家庭</Button>
      </View>
    )
  }

  return (
    <View className="index">
      <Text>{family?.name}</Text>
      <Text className="title">
        你好，
        {user?.nickname}
      </Text>
      <Button onClick={() => navigateTo(routes.pages.familyMembers)}>家庭成员</Button>
      <Button loading={loadingPets} onClick={loadPets}>刷新宠物</Button>
      <Button onClick={() => navigateTo(routes.pages.petEdit)}>创建宠物</Button>
      {pets.map(pet => (
        <View key={pet.id}>
          <Text>{pet.name}</Text>
          <Button onClick={() => navigateTo(`${routes.pages.petDetail}?petId=${encodeURIComponent(pet.id)}`)}>查看详情</Button>
        </View>
      ))}
      <Button onClick={() => switchTab(routes.tabs.calendar)}>日历</Button>
      <Button onClick={() => switchTab(routes.tabs.ask)}>问问</Button>
    </View>
  )
}
