import { useState, useRef, useCallback, useEffect } from 'react'
import { View, Input, RichText, Image } from '@tarojs/components'
import { ASK_PRESETS, ASK_MOCKS } from './data'
import eyesImg from '../../assets/ai-cat-eyes-check.png'
import aiCatImg from '../../assets/ai-cat.png'

interface HistoryItem { q: string; time: string }
type ElfMood = 'neutral' | 'thinking' | 'happy' | 'wink'

export default function AskPanel() {
  const [imgList, setImgList] = useState<string[]>([])
  const [isThinking, setIsThinking] = useState(false)
  const [history, setHistory] = useState<HistoryItem[]>([])
  const [historyMode, setHistoryMode] = useState(false)
  const [inputValue, setInputValue] = useState('')
  const [placeholder, setPlaceholder] = useState('问我关于宠物的问题吧~')
  const [askedPreset, setAskedPreset] = useState(-1)
  const [answerShow, setAnswerShow] = useState(false)
  const [answerHtml, setAnswerHtml] = useState('')
  const [elfMood, setElfMood] = useState<ElfMood>('neutral')
  const nextAnswerRef = useRef<string | null>(null)
  const pendingQRef = useRef<string | null>(null)
  const inputTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const completeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const sendActive = (inputValue.trim().length > 0 || !!pendingQRef.current) && !isThinking

  /* 输入时精灵眨眼，停止输入 1.2s 后恢复平静 */
  useEffect(() => {
    if (inputTimerRef.current) {
      clearTimeout(inputTimerRef.current)
      inputTimerRef.current = null
    }
    if (!isThinking && !answerShow && inputValue.trim().length > 0) {
      setElfMood('wink')
      inputTimerRef.current = setTimeout(() => {
        setElfMood('neutral')
        inputTimerRef.current = null
      }, 1200)
    }
    return () => {
      if (inputTimerRef.current) {
        clearTimeout(inputTimerRef.current)
        inputTimerRef.current = null
      }
      if (completeTimerRef.current) {
        clearTimeout(completeTimerRef.current)
        completeTimerRef.current = null
      }
    }
  }, [inputValue, isThinking, answerShow])

  const typeAnswer = useCallback((text: string) => {
    let i = 0
    const step = () => {
      if (i < text.length) {
        setAnswerHtml(text.slice(0, i + 1) + '<span class="ab-cursor"></span>')
        i++
        setTimeout(step, 25)
      } else {
        setAnswerHtml(text)
        setElfMood('wink')
        if (completeTimerRef.current) clearTimeout(completeTimerRef.current)
        completeTimerRef.current = setTimeout(() => setElfMood('neutral'), 1400)
      }
    }
    step()
  }, [])

  const sendQuestion = (presetIdx = -1) => {
    let text = inputValue.trim()
    if (presetIdx >= 0) {
      text = ASK_PRESETS[presetIdx].q
      nextAnswerRef.current = ASK_PRESETS[presetIdx].a
      pendingQRef.current = text
    } else if (pendingQRef.current && !text) {
      text = pendingQRef.current
    }
    if (!text || isThinking) return

    if (completeTimerRef.current) {
      clearTimeout(completeTimerRef.current)
      completeTimerRef.current = null
    }

    setHistory(prev => [{ q: text, time: new Date().toTimeString().slice(0, 5) }, ...prev])

    if (presetIdx >= 0) {
      setPlaceholder(text)
      setAskedPreset(presetIdx)
    } else {
      pendingQRef.current = null
      setPlaceholder('问点什么吧~')
      setAskedPreset(-1)
    }
    setInputValue('')
    setIsThinking(true)
    setAnswerShow(true)
    setElfMood('thinking')
    setAnswerHtml('<div class="ab-loading"><span>·</span><span>·</span><span>·</span></div>')

    setTimeout(() => {
      setIsThinking(false)
      setElfMood('happy')
      const answer = nextAnswerRef.current || ASK_MOCKS[Math.floor(Math.random() * ASK_MOCKS.length)]
      nextAnswerRef.current = null
      typeAnswer(answer)
    }, 1500)
  }

  const addImg = () => {
    const emojis = ['📷', '🖼️', '🐾', '🦴', '🌿', '💉']
    const emoji = emojis[Math.floor(Math.random() * emojis.length)]
    setImgList(prev => [...prev, emoji])
  }

  return (
    <View className='ask-content' id='askContent'>
      <View className='ask-stage' id='askStage'>
        <View className={`answer-wrap${answerShow ? ' show' : ''}`} id='answerWrap'>
          <View className='answer-box' id='answerBox'>
            <RichText className='ab-text' id='answerText' nodes={answerHtml} />
          </View>
        </View>
        <View className={`cat-container elf-mood-${elfMood}${isThinking ? ' thinking' : ''}${answerShow ? ' no-float' : ''}`} id='catContainer'>
          <View className='cat-img-frame'>
            {/* 猫咪图片：默认睁眼；思考态与回答完成态都覆盖眯眼笑（仅两条蓝弯线） */}
            <Image className='cat-img-base' src={aiCatImg} mode='aspectFit' />
            {(elfMood === 'thinking' || elfMood === 'happy') && (
              <Image className='cat-eyes-overlay' src={eyesImg} mode='aspectFit' />
            )}
          </View>
        </View>
      </View>

      <View className='preset-row' id='presetRow'>
        {ASK_PRESETS.map((p, i) => (
          <View key={i} className={`preset-chip${askedPreset === i ? ' asked' : ''}`} onClick={() => sendQuestion(i)}>{p.q}</View>
        ))}
      </View>

      <View className={`ask-input-bar${historyMode ? ' ask-history-mode' : ''}`} id='askInputBar'>
        <View className='ask-top-row'>
          <View className='ask-history-btn' id='askHistoryBtn' onClick={() => setHistoryMode(!historyMode)}>◷</View>
        </View>

        <View className={`ask-img-preview${imgList.length ? ' show' : ''}`} id='askImgPreview'>
          {imgList.map((img, i) => (
            <View key={i} className='ask-img-item'>
              {img}
              <View className='remove-x' onClick={() => setImgList(prev => prev.filter((_, j) => j !== i))}>✕</View>
            </View>
          ))}
        </View>

        <View className='ask-bottom-row'>
          <View className={`ask-add-img${imgList.length ? ' has-img' : ''}`} id='askAddImg' onClick={addImg}>＋</View>
          <Input
            className='ask-textarea'
            id='askTextarea'
            value={inputValue}
            placeholder={placeholder}
            maxlength={200}
            onInput={(e) => setInputValue(e.detail.value)}
          />
          <View className={`ask-send-btn${sendActive ? ' active' : ''}`} id='askSendBtn' onClick={() => sendQuestion()}>➤</View>
        </View>

        <View className='ask-history-list' id='askHistoryList'>
          {history.length === 0 ? (
            <View className='ask-history-empty'>还没有提问记录，试着问点什么吧~</View>
          ) : history.map((h, i) => (
            <View key={i} className='ask-hc-item' onClick={() => {
              pendingQRef.current = h.q
              setInputValue('')
              setPlaceholder(h.q)
              setHistoryMode(false)
            }}>
              <View>{h.q}</View>
              <View className='hc-meta'>{h.time}</View>
            </View>
          ))}
        </View>
      </View>
    </View>
  )
}

// trigger 1787059769159
