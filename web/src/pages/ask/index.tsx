import type { AskOperation } from '../../types/ask'
import { Button, Image, Input, ScrollView, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useCallback, useEffect, useRef, useState } from 'react'
import askBackground from '../../assets/ai-bg.jpg'
import catImage from '../../assets/ai-cat.png'
import AskEventView from '../../components/ask/ask-event'
import AskOperationCard from '../../components/ask/ask-operation'
import { askErrorReport } from '../../components/ask/ask-report'
import { routes } from '../../constants/routes'
import { visibleTurnEvents } from '../../hooks/ask-reducer'
import { useAskSession } from '../../hooks/use-ask-session'
import { abandonAskOperation, confirmAskOperation, executeAskOperation, getAskOperations, uploadAskImage } from '../../services/ask'
import { useAuthStore } from '../../stores/auth-store'
import { reLaunch } from '../../utils/navigation'
import './index.scss'

const nearBottomPx = 80

export default function Ask() {
  const token = useAuthStore(state => state.token)
  const {
    draft,
    phase,
    error,
    session,
    run,
    conversation,
    setDraft,
    submit,
    reply,
    retryConnection,
    stop,
    retry,
    isLiveEvent,
  } = useAskSession()
  const [assetRefs, setAssetRefs] = useState<string[]>([])
  const [uploading, setUploading] = useState(false)
  const [scrollTop, setScrollTop] = useState(0)
  const [operationState, setOperationState] = useState<{ sessionID: string, values: AskOperation[] }>({ sessionID: '', values: [] })
  const [operationBusy, setOperationBusy] = useState(false)

  const scrollTopRef = useRef(0)
  const followRef = useRef(true)
  const metricsRef = useRef({ contentHeight: 0, viewportHeight: 0 })

  const lastTurn = conversation.at(-1)
  const followKey = `${conversation.length}:${lastTurn?.events.length ?? 0}:${lastTurn?.events.at(-1)?.sequence ?? 0}`

  const measureConversation = useCallback(() => {
    const query = Taro.createSelectorQuery()
    query.select('.ask-conversation').boundingClientRect()
    query.select('.ask-conversation-inner').boundingClientRect()
    query.exec((result: Array<{ height: number } | null>) => {
      const viewport = result[0]
      const content = result[1]
      if (!viewport || !content) {
        return
      }
      metricsRef.current = { contentHeight: content.height, viewportHeight: viewport.height }
    })
  }, [])

  const followBottom = useCallback(() => {
    if (!followRef.current) {
      return
    }
    setScrollTop(value => value + 100000)
    measureConversation()
  }, [measureConversation])

  const handleConversationScroll = useCallback((event: { detail: { scrollTop: number } }) => {
    scrollTopRef.current = event.detail.scrollTop
    const { contentHeight, viewportHeight } = metricsRef.current
    if (!contentHeight || !viewportHeight) {
      return
    }
    followRef.current = scrollTopRef.current + viewportHeight >= contentHeight - nearBottomPx
  }, [])

  useEffect(() => {
    if (!conversation.length) {
      return undefined
    }
    measureConversation()
    const immediateTimer = setTimeout(followBottom, 0)
    const layoutTimer = setTimeout(followBottom, 200)
    const revealTimer = setTimeout(followBottom, 600)
    const settleTimer = setTimeout(followBottom, 1000)
    return () => {
      clearTimeout(immediateTimer)
      clearTimeout(layoutTimer)
      clearTimeout(revealTimer)
      clearTimeout(settleTimer)
    }
  }, [conversation.length, followBottom, followKey, measureConversation, operationState.values.length])

  const sessionID = session?.id
  const runStatus = run?.status

  useEffect(() => {
    if (!sessionID) {
      return undefined
    }
    let disposed = false
    getAskOperations(sessionID)
      .then((values) => {
        if (!disposed) {
          setOperationState({ sessionID, values: values.slice().sort((left, right) => left.created_at.localeCompare(right.created_at)) })
        }
      })
      .catch(() => {
        if (!disposed) {
          setOperationState({ sessionID, values: [] })
        }
      })
    return () => {
      disposed = true
    }
  }, [sessionID, runStatus, run?.id])

  const visibleOperations = operationState.sessionID === sessionID ? operationState.values : []

  async function handleOperationConfirm(operation: AskOperation) {
    if (!sessionID || operationBusy) {
      return
    }
    setOperationBusy(true)
    try {
      const confirmed = operation.status === 'pending' ? await confirmAskOperation(sessionID, operation.id, operation.version, operation.preview) : operation
      const executed = await executeAskOperation(sessionID, operation.id, confirmed.version)
      setOperationState(current => ({ ...current, values: current.values.map(value => value.id === executed.id ? executed : value) }))
      await Taro.showToast({ title: executed.status === 'succeeded' ? '已写入日历' : '写入未完成', icon: 'none' })
    }
    catch (error) {
      await Taro.showToast({ title: error instanceof Error ? error.message : '写入失败，请重试', icon: 'none' })
    }
    finally {
      setOperationBusy(false)
    }
  }

  async function handleOperationAbandon(operation: AskOperation) {
    if (!sessionID || operationBusy) {
      return
    }
    setOperationBusy(true)
    try {
      const abandoned = await abandonAskOperation(sessionID, operation.id, operation.version)
      setOperationState(current => ({ ...current, values: current.values.map(value => value.id === abandoned.id ? abandoned : value) }))
    }
    catch (error) {
      await Taro.showToast({ title: error instanceof Error ? error.message : '取消失败，请重试', icon: 'none' })
    }
    finally {
      setOperationBusy(false)
    }
  }

  const busy = phase === 'creating' || phase === 'thinking' || phase === 'reconnecting' || phase === 'replying'
  const hasError = Boolean(error) && (phase === 'input_error' || phase === 'ambiguous' || phase === 'network_error' || phase === 'failed')
  const hasConversation = conversation.length > 0 || hasError
  async function handleSend() {
    if (!token) {
      await Taro.showToast({ title: '请先登录后使用问问', icon: 'none' })
      await reLaunch(routes.pages.profileOnboarding)
      return
    }
    const value = draft.trim()
    if (!value && !assetRefs.length) {
      await Taro.showToast({ title: '请先输入问题', icon: 'none' })
      return
    }
    if (value.length > 4000) {
      await Taro.showToast({ title: '问题内容不能超过 4000 个字符', icon: 'none' })
      return
    }
    try {
      if (run?.status === 'waiting_input') {
        await reply(value, assetRefs)
      }
      else {
        await submit(value, assetRefs)
      }
      setAssetRefs([])
    }
    catch {
      setDraft(value)
    }
  }

  async function handleChooseImage() {
    if (busy || uploading || assetRefs.length >= 4) {
      return
    }
    try {
      const result = await Taro.chooseImage({ count: 4 - assetRefs.length, sizeType: ['compressed'], sourceType: ['album', 'camera'] })
      if (!result.tempFilePaths.length) {
        return
      }
      setUploading(true)
      const uploaded = await Promise.all(result.tempFilePaths.map(uploadAskImage))
      setAssetRefs(previous => [...previous, ...uploaded.map(item => item.asset_id)])
    }
    catch (error) {
      await Taro.showToast({ title: error instanceof Error ? error.message : '图片上传失败', icon: 'none' })
    }
    finally {
      setUploading(false)
    }
  }

  function handlePreset(value: string) {
    setDraft(value)
  }

  function errorText() {
    if (error) {
      return error
    }
    if (phase === 'input_error') {
      return '请在问题中写出宠物名称后再试。'
    }
    if (phase === 'ambiguous') {
      return '有同名宠物，请补充更具体的信息。'
    }
    if (phase === 'network_error') {
      return '网络连接不稳定，请稍后重试。'
    }
    return '问问服务出现未知错误，请稍后重试。'
  }

  const errorReport = TARO_APP_DEBUG
    ? askErrorReport(
        conversation.at(-1)?.input || draft,
        conversation.filter(turn => turn.runID === run?.id).flatMap(turn => turn.events).sort((left, right) => left.sequence - right.sequence),
        errorText(),
        `session_id=${session?.id || '未创建'}，run_id=${run?.id || '未创建'}`,
      )
    : errorText()

  async function copyErrorReport() {
    try {
      await Taro.setClipboardData({ data: errorReport })
    }
    catch {
      await Taro.showToast({ title: '复制失败，请重试', icon: 'none' })
    }
  }

  return (
    <View className={`ask-page${hasConversation ? ' has-conversation' : ''}`} id="askModuleScreen">
      <Image className="ask-background" src={askBackground} mode="aspectFill" />
      <View className="ask-content-area">
        {!hasConversation && (
          <>
            <View className="ask-cat-wrap">
              <Image className="ask-cat" src={catImage} mode="aspectFit" />
            </View>
            <View className="preset-row">
              <View className="preset-chip" onClick={() => handlePreset('上次打疫苗是什么时候')}>疫苗记录</View>
              <View className="preset-chip" onClick={() => handlePreset('上次洗澡是什么时候')}>洗澡记录</View>
              <View className="preset-chip" onClick={() => handlePreset('最近精神不太好')}>健康观察</View>
            </View>
          </>
        )}
        {hasConversation && (
          <ScrollView className="ask-conversation" scrollY enhanced showScrollbar={false} scrollTop={scrollTop} onScroll={handleConversationScroll}>
            <View className="ask-conversation-inner">
              {!!session?.pets?.length && (
                <View className="ask-bound-pets">
                  {session.pets.map(pet => <Text className="ask-bound-pet" key={pet.pet_id}>{pet.pet_name}</Text>)}
                </View>
              )}
              {conversation.map(turn => (
                <View className="ask-turn" key={turn.id}>
                  {turn.input && (
                    <View className="ask-message ask-message--user">
                      <Text>{turn.input}</Text>
                    </View>
                  )}
                  {visibleTurnEvents(turn.events).map(event => (
                    <AskEventView event={event} input={turn.input} events={turn.events} live={isLiveEvent(event)} key={`${turn.runID}-${event.sequence}`} />
                  ))}
                </View>
              ))}
              {visibleOperations.map(operation => (
                <AskOperationCard
                  key={operation.id}
                  operation={operation}
                  busy={operationBusy}
                  onConfirm={value => void handleOperationConfirm(value)}
                  onAbandon={value => void handleOperationAbandon(value)}
                />
              ))}
              {busy && (
                <View className="ask-processing">
                  <Text>{phase === 'reconnecting' ? '正在恢复连接' : phase === 'replying' ? '正在整理补充信息' : '正在查看宠物记录'}</Text>
                </View>
              )}
              {(run?.status === 'queued' || run?.status === 'running') && (
                <Button className="ask-inline-button" onClick={() => void stop()}>停止</Button>
              )}
              {(run?.status === 'failed' || run?.status === 'canceled' || run?.status === 'interrupted') && (
                <Button className="ask-inline-button" onClick={() => void retry()}>重试</Button>
              )}
              {hasError && (
                <View className="ask-error-state">
                  <Text>{errorText()}</Text>
                  {TARO_APP_DEBUG && (
                    <Text className="ask-error-report">{errorReport}</Text>
                  )}
                  <Button className="ask-inline-button" onClick={() => void copyErrorReport()} aria-label="复制错误报告">复制错误报告</Button>
                  {phase === 'network_error' && (run?.status === 'queued' || run?.status === 'running') && (
                    <Button className="ask-inline-button" onClick={retryConnection}>重新连接</Button>
                  )}
                </View>
              )}
            </View>
          </ScrollView>
        )}
      </View>
      <View className="ask-input-bar">
        <View className="ask-bottom-row">
          <Button className={`ask-add-img${assetRefs.length ? ' has-img' : ''}`} disabled={busy || uploading || assetRefs.length >= 4} aria-label="添加图片" onClick={() => void handleChooseImage()}>＋</Button>
          <Input
            className="ask-textarea"
            value={draft}
            maxlength={4000}
            disabled={busy || uploading}
            confirmType="send"
            placeholder={phase === 'waiting_input' ? '补充刚才的问题' : '问我关于宠物的问题吧~'}
            onInput={event => setDraft(event.detail.value)}
            onConfirm={() => void handleSend()}
          />
          <Button className={`ask-send-btn${(draft.trim() || assetRefs.length) && !busy && !uploading ? ' active' : ''}`} disabled={(!draft.trim() && !assetRefs.length) || busy || uploading} aria-label="发送" onClick={() => void handleSend()}>➤</Button>
        </View>
      </View>
    </View>
  )
}
