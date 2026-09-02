import { Image, ScrollView, Text, View } from '@tarojs/components'
import { useCallback, useMemo, useState } from 'react'
import backgroundImage from '../../assets/background1.png'
import passportImage from '../../assets/passport.png'
import { usePetStore } from '../../stores/pet-store'
import './index.scss'

interface ManualPage {
  num: string
  title: string
  subtitle: string
  items: { label: string, value: string, emoji: string }[]
}

function createPages(name: string): ManualPage[] {
  return [
    {
      num: '01',
      title: '身份与证件',
      subtitle: '宠物的基本身份信息',
      items: [
        { label: '名字', value: name, emoji: '🐾' },
        { label: '品种', value: '暂无记录', emoji: '🐕' },
        { label: '性别', value: '暂无记录', emoji: '♀' },
        { label: '出生日期', value: '暂无记录', emoji: '🎂' },
        { label: '芯片号', value: '暂无记录', emoji: '📟' },
      ],
    },
    {
      num: '02',
      title: '个性说明书',
      subtitle: '了解 TA 的小脾气',
      items: [
        { label: '性格', value: '暂无记录', emoji: '😊' },
        { label: '爱好', value: '暂无记录', emoji: '☀️' },
        { label: '害怕的事', value: '暂无记录', emoji: '💭' },
        { label: '小习惯', value: '暂无记录', emoji: '🐾' },
      ],
    },
    {
      num: '03',
      title: '健康档案',
      subtitle: '守护 TA 的每一天',
      items: [
        { label: '体重', value: '暂无记录', emoji: '⚖️' },
        { label: '疫苗', value: '暂无记录', emoji: '💉' },
        { label: '驱虫', value: '暂无记录', emoji: '🛡️' },
        { label: '绝育情况', value: '暂无记录', emoji: '✂️' },
        { label: '最近体检', value: '暂无记录', emoji: '📋' },
      ],
    },
    {
      num: '04',
      title: '生日纪念',
      subtitle: '重要的日子',
      items: [
        { label: '出生日期', value: '暂无记录', emoji: '🎂' },
        { label: '到家日期', value: '暂无记录', emoji: '🏠' },
        { label: '下一次生日', value: '暂无记录', emoji: '🎉' },
      ],
    },
    {
      num: '05',
      title: '成长记录',
      subtitle: '点滴成长，都是回忆',
      items: [
        { label: '成长足迹', value: '暂无记录', emoji: '👣' },
        { label: '重要时刻', value: '暂无记录', emoji: '📷' },
        { label: '珍藏记忆', value: '暂无记录', emoji: '💌' },
      ],
    },
  ]
}

export default function Profile() {
  const pet = usePetStore(state => state.pets[0])
  const name = pet?.name || '宠物'
  const pages = useMemo(() => createPages(name), [name])
  const [currentPage, setCurrentPage] = useState(0)
  const [touchStartX, setTouchStartX] = useState<number | null>(null)
  const [touchDeltaX, setTouchDeltaX] = useState(0)
  const [turning, setTurning] = useState(false)
  const [coverRatio, setCoverRatio] = useState('1086 / 1448')
  const [turnDirection, setTurnDirection] = useState<'next' | 'previous' | null>(null)
  const [resetting, setResetting] = useState(false)

  const goToPage = useCallback((page: number) => {
    setCurrentPage(Math.max(0, Math.min(pages.length, page)))
  }, [pages.length])

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
    if ((deltaX < 0 && currentPage >= pages.length) || (deltaX > 0 && currentPage <= 0)) {
      setTouchDeltaX(0)
      setTurnDirection(null)
      return
    }
    setTouchDeltaX(Math.max(-360, Math.min(360, deltaX)))
    setTurnDirection(deltaX < 0 ? 'next' : 'previous')
  }, [currentPage, pages.length, touchStartX])

  const handleTouchEnd = useCallback((event: any) => {
    if (touchStartX === null) {
      return
    }

    const deltaX = event.changedTouches[0].clientX - touchStartX
    setTouchStartX(null)
    const rotation = Math.max(-180, Math.min(180, deltaX * 0.5))
    const canTurnNext = rotation <= -35 && currentPage < pages.length
    const canTurnPrevious = rotation >= 35 && currentPage > 0
    if (canTurnNext || canTurnPrevious) {
      const direction = canTurnNext ? 'next' : 'previous'
      setTurning(true)
      setTurnDirection(direction)
      setTouchDeltaX(direction === 'next' ? -360 : 360)
      setTimeout(() => {
        setResetting(true)
        goToPage(currentPage + (direction === 'next' ? 1 : -1))
        setTouchDeltaX(0)
        setTurnDirection(null)
        setTurning(false)
        setTimeout(() => setResetting(false), 0)
      }, 240)
      return
    }
    setTouchDeltaX(0)
    setTurnDirection(null)
  }, [currentPage, goToPage, pages.length, touchStartX])

  const handleCoverLoad = useCallback((event: any) => {
    const width = event.detail?.width
    const height = event.detail?.height
    if (width && height) {
      setCoverRatio(`${width} / ${height}`)
    }
  }, [])

  const renderBookPage = (pageIndex: number) => {
    if (pageIndex === 0) {
      return <View className="book-cover"><Image className="cover-image" src={passportImage} mode="scaleToFill" onLoad={handleCoverLoad} /></View>
    }

    if (pageIndex === pages.length) {
      return (
        <View className="book-back-cover">
          <Text className="back-emoji">📖</Text>
          <Text className="back-title">成长还在继续...</Text>
          <Text className="back-sub">新的回忆会慢慢写进这里</Text>
          <View className="back-restart" onClick={() => goToPage(0)}>回到封面</View>
        </View>
      )
    }

    const page = pages[pageIndex - 1]
    return (
      <View className="book-page" key={pageIndex}>
        <Text className="page-num">{page.num}</Text>
        <ScrollView className="page-content" scrollY>
          <Text className="page-title">{page.title}</Text>
          <Text className="page-subtitle">{page.subtitle}</Text>
          <View className="page-divider" />
          <View className="page-items">
            {page.items.map(item => (
              <View key={item.label} className="page-item">
                <View className="item-emoji"><Text>{item.emoji}</Text></View>
                <View className="item-text">
                  <Text className="item-label">{item.label}</Text>
                  <Text className="item-value">{item.value}</Text>
                </View>
              </View>
            ))}
          </View>
        </ScrollView>
      </View>
    )
  }

  const adjacentPage = turnDirection === 'previous'
    ? (currentPage > 0 ? currentPage - 1 : null)
    : (currentPage < pages.length ? currentPage + 1 : null)

  return (
    <View className="archive-page manual-page">
      <Image className="archive-background" src={backgroundImage} mode="aspectFill" />
      <View className="book-container" style={{ aspectRatio: coverRatio }} onTouchStart={handleTouchStart} onTouchMove={handleTouchMove} onTouchEnd={handleTouchEnd}>
        {adjacentPage !== null && <View className="book-card book-card-next">{renderBookPage(adjacentPage)}</View>}
        <View className={`book-card book-card-current${touchStartX === null && !resetting ? '' : ' dragging'}`} style={{ transform: `rotateY(${Math.max(-180, Math.min(180, touchDeltaX * 0.5))}deg)` }}>
          {renderBookPage(currentPage)}
        </View>
      </View>
    </View>
  )
}
