import { Button, Image, Input, ScrollView, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import askBackground from '../../assets/ai-bg.jpg'
import catImage from '../../assets/ai-cat.png'
import AskEventView from '../../components/ask/ask-event'
import { useAskSession } from '../../hooks/use-ask-session'
import './index.scss'

export default function Ask() {
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
    retryProcess,
    reset,
  } = useAskSession()

  const busy = phase === 'creating' || phase === 'thinking' || phase === 'reconnecting' || phase === 'replying'
  const hasError = Boolean(error) && (phase === 'input_error' || phase === 'ambiguous' || phase === 'network_error' || phase === 'failed')
  const hasConversation = conversation.length > 0 || hasError
  const terminal = phase === 'completed' || phase === 'escalated' || phase === 'failed'
  const visibleEventTypes = new Set(['assistant.question', 'fact.completed', 'run.completed', 'risk.escalated', 'run.failed'])

  async function handleSend() {
    const value = draft.trim()
    if (!value) {
      await Taro.showToast({ title: '请先输入问题', icon: 'none' })
      return
    }
    if (value.length > 4000) {
      await Taro.showToast({ title: '问题内容不能超过 4000 个字符', icon: 'none' })
      return
    }
    try {
      if (run?.status === 'waiting_input') {
        await reply(value)
      }
      else {
        await submit(value)
      }
    }
    catch {
      setDraft(value)
    }
  }

  function handlePreset(value: string) {
    setDraft(value)
  }

  function errorText() {
    if (phase === 'input_error') {
      return '请在问题中写出宠物名称后再试。'
    }
    if (phase === 'ambiguous') {
      return '有同名宠物，请补充更具体的信息。'
    }
    if (phase === 'network_error') {
      return '网络连接不稳定，请稍后重试。'
    }
    return '这次没有完成，请重新发起问题。'
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
          <ScrollView className="ask-conversation" scrollY enhanced showScrollbar={false}>
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
                  {turn.events.filter(event => visibleEventTypes.has(event.type)).map(event => (
                    <AskEventView event={event} key={`${turn.runID}-${event.sequence}`} />
                  ))}
                </View>
              ))}
              {busy && (
                <View className="ask-processing">
                  <Text>{phase === 'reconnecting' ? '正在恢复连接' : phase === 'replying' ? '正在整理补充信息' : '正在查看宠物记录'}</Text>
                </View>
              )}
              {hasError && (
                <View className="ask-error-state">
                  <Text>{errorText()}</Text>
                  {phase === 'network_error' && (run?.status === 'queued' || run?.status === 'running') && (
                    <Button className="ask-inline-button" onClick={() => void retryProcess()}>重试</Button>
                  )}
                </View>
              )}
              {terminal && <Button className="ask-new-button" onClick={reset}>问新的问题</Button>}
            </View>
          </ScrollView>
        )}
      </View>
      <View className="ask-input-bar">
        <View className="ask-bottom-row">
          <Button className="ask-add-img" disabled aria-label="添加图片，暂不可用">＋</Button>
          <Input
            className="ask-textarea"
            value={draft}
            maxlength={4000}
            disabled={busy}
            confirmType="send"
            placeholder={phase === 'waiting_input' ? '补充刚才的问题' : '问我关于宠物的问题吧~'}
            onInput={event => setDraft(event.detail.value)}
            onConfirm={() => void handleSend()}
          />
          <Button className={`ask-send-btn${draft.trim() && !busy ? ' active' : ''}`} disabled={!draft.trim() || busy} aria-label="发送" onClick={() => void handleSend()}>➤</Button>
        </View>
      </View>
    </View>
  )
}
