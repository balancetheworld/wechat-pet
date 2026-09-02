import { Button, Text, View } from '@tarojs/components'
import Taro, { useDidShow } from '@tarojs/taro'
import { useCallback, useEffect, useState } from 'react'
import PageBackground from '../../components/page-background'
import { routes } from '../../constants/routes'
import { getPets } from '../../services/pet'
import { useAppStore } from '../../stores/app-store'
import { useAuthStore } from '../../stores/auth-store'
import { useFamilyStore } from '../../stores/family-store'
import { usePetStore } from '../../stores/pet-store'
import { navigateTo, reLaunch, switchTab } from '../../utils/navigation'
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

  useEffect(() => {
    if (bootstrapCompleted && !bootstrapError && user && identity !== 'guest') {
      void reLaunch(routes.tabs.calendar)
    }
  }, [bootstrapCompleted, bootstrapError, identity, user])

  if (!bootstrapCompleted) {
    return (
      <View className="index">
        <PageBackground />
        <Text>启动中</Text>
      </View>
    )
  }

  if (bootstrapError) {
    return (
      <View className="index">
        <PageBackground />
        <Text>{bootstrapError}</Text>
        <Button onClick={retryBootstrap}>重新登录</Button>
      </View>
    )
  }

  if (user && identity !== 'guest') {
    return (
      <View className="index">
        <PageBackground />
        <Text>正在打开宠物日历</Text>
      </View>
    )
  }

  if (identity === 'guest') {
    return (
      <View className="index">
        <PageBackground />
        <View className="index__header">
          <Text className="title">宠物家庭</Text>
        </View>
        <Button onClick={() => navigateTo(routes.pages.createFamily)}>创建家庭</Button>
        <Button onClick={() => navigateTo(routes.pages.joinFamily)}>加入家庭</Button>
      </View>
    )
  }

  return (
    <View className="index">
      <PageBackground />
      <View className="index__header">
        <View>
          <Text>{family?.name}</Text>
          <Text className="title">
            你好，
            {user?.nickname}
          </Text>
        </View>
      </View>
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
