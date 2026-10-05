import { Button, Text, View } from '@tarojs/components'
import { useDidShow } from '@tarojs/taro'
import { useCallback, useEffect } from 'react'
import PageBackground from '../../components/page-background'
import { routes } from '../../constants/routes'
import { getPets } from '../../services/pet'
import { useAppStore } from '../../stores/app-store'
import { useAuthStore } from '../../stores/auth-store'
import { useFamilyStore } from '../../stores/family-store'
import { usePetStore } from '../../stores/pet-store'
import { navigateTo, reLaunch } from '../../utils/navigation'
import './index.scss'

export default function Index() {
  const { bootstrapCompleted, bootstrapError, retryBootstrap } = useAppStore()
  const { identity, user } = useAuthStore()
  const { family } = useFamilyStore()
  const { setPets } = usePetStore()

  const loadPets = useCallback(async () => {
    if (identity === 'guest' || !bootstrapCompleted) {
      return
    }
    try {
      setPets(await getPets())
    }
    catch {
      /* 宠物列表加载失败静默: 本页只负责引导进主界面, 主界面会重新拉取 */
    }
  }, [bootstrapCompleted, identity, setPets])

  useEffect(() => {
    void loadPets()
  }, [loadPets])

  useDidShow(() => {
    void loadPets()
  })

  /* 老用户(已登录且有家庭)直接进日历主界面 */
  useEffect(() => {
    if (bootstrapCompleted && !bootstrapError && user && identity !== 'guest' && family) {
      void reLaunch(routes.tabs.calendar)
    }
  }, [bootstrapCompleted, bootstrapError, family, identity, user])

  if (!bootstrapCompleted) {
    return (
      <View className="entry-page">
        <PageBackground />
        <View className="entry-loading">
          <Text className="entry-loading__text">启动中…</Text>
        </View>
      </View>
    )
  }

  if (bootstrapError) {
    return (
      <View className="entry-page">
        <PageBackground />
        <View className="entry-card">
          <Text className="entry-card__title">连接失败</Text>
          <Text className="entry-card__desc">{bootstrapError}</Text>
          <Button className="entry-btn entry-btn--primary" onClick={retryBootstrap}>重新登录</Button>
        </View>
      </View>
    )
  }

  if (user && identity !== 'guest') {
    return (
      <View className="entry-page">
        <PageBackground />
        <View className="entry-loading">
          <Text className="entry-loading__text">正在打开宠物日历…</Text>
        </View>
      </View>
    )
  }

  return (
    <View className="entry-page">
      <PageBackground />
      <View className="entry-hero">
        <Text className="entry-hero__brand">宠物小册</Text>
        <Text className="entry-hero__slogan">和毛孩子一起，记录每一天</Text>
      </View>

      <View className="entry-card">
        <Text className="entry-card__title">还没有加入家庭</Text>
        <Text className="entry-card__desc">
          创建一个属于你们的家庭，或凭家庭码加入已有家庭，就能开始记录宠物档案与日历啦
        </Text>
        <Button
          className="entry-btn entry-btn--primary"
          hoverClass="entry-btn--press"
          onClick={() => navigateTo(routes.pages.createFamily)}
        >
          创建家庭
        </Button>
        <Button
          className="entry-btn entry-btn--ghost"
          hoverClass="entry-btn--press"
          onClick={() => navigateTo(routes.pages.joinFamily)}
        >
          加入家庭
        </Button>
        <Button
          className="entry-btn entry-btn--text"
          hoverClass="entry-btn--press"
          onClick={() => navigateTo(routes.pages.pendingFamily)}
        >
          查看我的申请
        </Button>
      </View>
    </View>
  )
}
