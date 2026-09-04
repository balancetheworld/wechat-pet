import type { Pet, PetProfile } from '../../types/pet'
import { Button, Image, ScrollView, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useCallback, useEffect, useState } from 'react'
import backgroundImage from '../../assets/background1.png'
import passportImage from '../../assets/passport.png'
import { routes } from '../../constants/routes'
import { getPetProfile, getPetResource, getPets } from '../../services/pet'
import { usePetStore } from '../../stores/pet-store'
import { navigateTo } from '../../utils/navigation'
import './index.scss'

interface PersonalityItem {
  id: string
  trait: string
  value: string
}

interface QuestionItem {
  id: string
  question: string
  answer: string
}

interface BirthdayRecord {
  id: string
  year: number
  age: number
  summary: string
}

interface WeightRecord {
  id: string
  measured_at: string
  weight: number
}

interface GrowthEvent {
  id: string
  type: string
  occurred_at: string
  recorder: string
  content: string
}

const pageCount = 4

function formatDate(value?: string) {
  return value || '暂无记录'
}

function formatGender(value: string) {
  if (value === 'male') {
    return '男孩'
  }
  if (value === 'female') {
    return '女孩'
  }
  return value || '暂无记录'
}

export default function Profile() {
  const pets = usePetStore(state => state.pets)
  const currentPetId = usePetStore(state => state.currentPetId)
  const setPets = usePetStore(state => state.setPets)
  const [profile, setProfile] = useState<PetProfile | null>(null)
  const [personality, setPersonality] = useState<PersonalityItem[]>([])
  const [questions, setQuestions] = useState<QuestionItem[]>([])
  const [birthdayRecords, setBirthdayRecords] = useState<BirthdayRecord[]>([])
  const [weights, setWeights] = useState<WeightRecord[]>([])
  const [growthEvents, setGrowthEvents] = useState<GrowthEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [currentPage, setCurrentPage] = useState(0)
  const [touchStartX, setTouchStartX] = useState<number | null>(null)
  const [touchDeltaX, setTouchDeltaX] = useState(0)
  const [turning, setTurning] = useState(false)
  const [coverRatio, setCoverRatio] = useState('1086 / 1448')
  const [turnDirection, setTurnDirection] = useState<'next' | 'previous' | null>(null)
  const [turnTargetPage, setTurnTargetPage] = useState<number | null>(null)
  const [resetting, setResetting] = useState(false)

  const selectedPet = pets.find(item => item.id === currentPetId) || pets[0]

  const loadProfile = useCallback(async (pet: Pet) => {
    setLoading(true)
    try {
      const petProfile = await getPetProfile(pet.id)
      const [personalityItems, questionItems, records, weightItems, eventItems] = await Promise.all([
        getPetResource<PersonalityItem[]>(pet.id, 'personality'),
        getPetResource<QuestionItem[]>(pet.id, 'questions'),
        getPetResource<BirthdayRecord[]>(pet.id, 'birthday-records'),
        getPetResource<WeightRecord[]>(pet.id, 'weights'),
        getPetResource<GrowthEvent[]>(pet.id, 'growth-events'),
      ])
      setProfile(petProfile)
      setPersonality(personalityItems)
      setQuestions(questionItems)
      setBirthdayRecords(records)
      setWeights(weightItems)
      setGrowthEvents(eventItems)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载档案失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    async function load() {
      let pet = selectedPet
      if (!pet) {
        try {
          const petList = await getPets()
          setPets(petList)
          pet = petList[0]
        }
        catch (error) {
          const message = error instanceof Error ? error.message : '加载宠物失败'
          await Taro.showToast({ title: message, icon: 'none' })
          setLoading(false)
          return
        }
      }
      if (pet) {
        await loadProfile(pet)
        return
      }
      setLoading(false)
    }

    void load()
  }, [loadProfile, selectedPet, setPets])

  const goToPage = useCallback((page: number) => {
    setCurrentPage(Math.max(0, Math.min(pageCount, page)))
  }, [])

  const handleTouchStart = useCallback((event: any) => {
    if (turning) {
      return
    }
    setTouchStartX(event.touches[0].clientX)
    setTouchDeltaX(0)
    setTurnDirection(null)
  }, [turning])

  const handleTouchMove = useCallback((event: any) => {
    if (touchStartX === null) {
      return
    }
    const deltaX = event.touches[0].clientX - touchStartX
    if ((deltaX < 0 && currentPage >= pageCount) || (deltaX > 0 && currentPage <= 0)) {
      setTouchDeltaX(0)
      setTurnDirection(null)
      return
    }
    setTouchDeltaX(Math.max(-360, Math.min(360, deltaX)))
    setTurnDirection(deltaX < 0 ? 'next' : 'previous')
  }, [currentPage, touchStartX])

  const handleTouchEnd = useCallback((event: any) => {
    if (touchStartX === null) {
      return
    }
    const deltaX = event.changedTouches[0].clientX - touchStartX
    setTouchStartX(null)
    const rotation = Math.max(-180, Math.min(180, deltaX * 0.5))
    const canTurnNext = rotation <= -35 && currentPage < pageCount
    const canTurnPrevious = rotation >= 35 && currentPage > 0
    if (!canTurnNext && !canTurnPrevious) {
      setTouchDeltaX(0)
      setTurnDirection(null)
      return
    }
    const direction = canTurnNext ? 'next' : 'previous'
    const targetPage = currentPage + (direction === 'next' ? 1 : -1)
    setTurning(true)
    setTurnDirection(direction)
    setTurnTargetPage(targetPage)
    setTouchDeltaX(direction === 'next' ? -360 : 360)
    setTimeout(() => {
      setResetting(true)
      goToPage(targetPage)
      setTouchDeltaX(0)
      setTimeout(() => {
        setTurnTargetPage(null)
        setTurnDirection(null)
        setResetting(false)
        setTurning(false)
      }, 16)
    }, 260)
  }, [currentPage, goToPage, touchStartX])

  const handleCoverLoad = useCallback((event: any) => {
    const width = event.detail?.width
    const height = event.detail?.height
    if (width && height) {
      setCoverRatio(`${width} / ${height}`)
    }
  }, [])

  const renderProfilePage = () => (
    <View className="book-page">
      <ScrollView className="page-content" scrollY>
        <View className="pet-heading">
          <View className="pet-avatar">
            <Text>{profile?.name.slice(0, 1) || '宠'}</Text>
          </View>
          <View>
            <Text className="page-title">{profile?.name || selectedPet?.name || '宠物档案'}</Text>
            <Text className="page-subtitle">基本资料</Text>
          </View>
        </View>
        <View className="info-grid">
          <View className="info-cell">
            <Text>品种</Text>
            <Text>{profile?.breed || '暂无记录'}</Text>
          </View>
          <View className="info-cell">
            <Text>性别</Text>
            <Text>{formatGender(profile?.gender || '')}</Text>
          </View>
          <View className="info-cell">
            <Text>绝育</Text>
            <Text>{profile ? (profile.sterilized ? '已绝育' : '未绝育') : '暂无记录'}</Text>
          </View>
          <View className="info-cell">
            <Text>年龄</Text>
            <Text>{profile?.birthday ? `${profile.age} 岁` : '暂无记录'}</Text>
          </View>
        </View>
        <View className="page-divider" />
        <View className="page-items">
          <View className="page-item">
            <Text className="item-label">生日</Text>
            <Text className="item-value">{formatDate(profile?.birthday)}</Text>
          </View>
          <View className="page-item">
            <Text className="item-label">到家日期</Text>
            <Text className="item-value">{formatDate(profile?.home_date)}</Text>
          </View>
          <View className="page-item">
            <Text className="item-label">陪伴天数</Text>
            <Text className="item-value">{profile?.home_date ? `${profile.companion_days} 天` : '暂无记录'}</Text>
          </View>
          <View className="page-item">
            <Text className="item-label">下次生日</Text>
            <Text className="item-value">{profile?.next_birthday_days === undefined ? '暂无记录' : `${profile.next_birthday_days} 天后`}</Text>
          </View>
        </View>
      </ScrollView>
    </View>
  )

  const renderPersonalityPage = () => (
    <View className="book-page">
      <ScrollView className="page-content" scrollY>
        <Text className="page-title">性格与偏好</Text>
        <Text className="page-subtitle">TA 独一无二的小世界</Text>
        <View className="page-divider" />
        <Text className="section-title">性格标签</Text>
        <View className="tag-list">
          {personality.length > 0
            ? personality.map(item => (
              <Text className="personality-tag" key={item.id}>
                {item.trait}
                {' · '}
                {item.value}
              </Text>
            ))
            : <Text className="empty-text">暂无性格记录</Text>}
        </View>
        <Text className="section-title">事件问答</Text>
        <View className="question-list">
          {questions.length > 0
            ? questions.map(item => (
              <View className="question-item" key={item.id}>
                <Text className="question-text">{item.question}</Text>
                <Text className="answer-text">{item.answer}</Text>
              </View>
            ))
            : <Text className="empty-text">暂无事件问答</Text>}
        </View>
      </ScrollView>
    </View>
  )

  const renderBirthdayPage = () => (
    <View className="book-page">
      <ScrollView className="page-content" scrollY>
        <Text className="page-title">生日纪念册</Text>
        <Text className="page-subtitle">每一岁，都值得珍藏</Text>
        <View className="page-divider" />
        <View className="birthday-date">
          <Text>生日</Text>
          <Text>{formatDate(profile?.birthday)}</Text>
        </View>
        <View className="birthday-list">
          {birthdayRecords.length > 0
            ? birthdayRecords.map(item => (
              <View className="birthday-item" key={item.id}>
                <View className="birthday-year">
                  <Text>{item.year}</Text>
                  <Text>
                    {item.age}
                    {' 岁'}
                  </Text>
                </View>
                <Text className="birthday-summary">{item.summary || '暂无简介'}</Text>
              </View>
            ))
            : <Text className="empty-text">暂无生日纪念</Text>}
        </View>
      </ScrollView>
    </View>
  )

  const renderGrowthPage = () => (
    <View className="book-page">
      <ScrollView className="page-content" scrollY>
        <Text className="page-title">成长足迹</Text>
        <Text className="page-subtitle">点滴成长，都是回忆</Text>
        <View className="page-divider" />
        <Text className="section-title">体重记录</Text>
        <View className="weight-list">
          {weights.length > 0
            ? weights.map(item => (
              <View className="weight-item" key={item.id}>
                <Text>{item.measured_at}</Text>
                <Text>
                  {item.weight}
                  {' kg'}
                </Text>
              </View>
            ))
            : <Text className="empty-text">暂无体重记录</Text>}
        </View>
        <Text className="section-title">成长事件</Text>
        <View className="event-list">
          {growthEvents.length > 0
            ? growthEvents.map(item => (
              <View className="event-item" key={item.id}>
                <Text className="event-meta">
                  {item.occurred_at}
                  {' · '}
                  {item.type}
                  {' · '}
                  {item.recorder}
                </Text>
                <Text className="event-content">{item.content}</Text>
              </View>
            ))
            : <Text className="empty-text">暂无成长事件</Text>}
        </View>
      </ScrollView>
    </View>
  )

  const renderBookPage = (pageIndex: number) => {
    if (pageIndex === 0) {
      return <View className="book-cover"><Image className="cover-image" src={passportImage} mode="scaleToFill" onLoad={handleCoverLoad} /></View>
    }
    if (pageIndex === 1) {
      return renderProfilePage()
    }
    if (pageIndex === 2) {
      return renderPersonalityPage()
    }
    if (pageIndex === 3) {
      return renderBirthdayPage()
    }
    return renderGrowthPage()
  }

  const adjacentPage = turnTargetPage ?? (turnDirection === 'previous'
    ? (currentPage > 0 ? currentPage - 1 : null)
    : (currentPage < pageCount ? currentPage + 1 : null))

  return (
    <View className="archive-page manual-page">
      <Image className="archive-background" src={backgroundImage} mode="aspectFill" />
      <Button className="archive-add-pet" onClick={() => navigateTo(routes.pages.petEdit)}>添加宠物</Button>
      <View className="book-container" style={{ aspectRatio: coverRatio }} onTouchStart={handleTouchStart} onTouchMove={handleTouchMove} onTouchEnd={handleTouchEnd}>
        {adjacentPage !== null && <View className="book-card book-card-next">{renderBookPage(adjacentPage)}</View>}
        <View className={`book-card book-card-current${touchStartX === null && !resetting ? '' : ' dragging'}`} style={{ transform: `rotateY(${Math.max(-180, Math.min(180, touchDeltaX * 0.5))}deg)` }}>
          {renderBookPage(currentPage)}
        </View>
        {loading && <View className="book-loading"><Text>正在加载档案</Text></View>}
        {!loading && !selectedPet && (
          <View className="book-loading">
            <Text>还没有宠物档案</Text>
          </View>
        )}
      </View>
    </View>
  )
}
