import type { AskAnalysisResult, AskAnswerGroup, AskAssistantResult, AskDeltaResult, AskEvent, AskFactResult, AskFailedResult, AskFamilyPetsResult, AskProgressResult, AskQuestionResult, AskRiskResult, AskTaskCoverage } from '../../types/ask'
import { Button, RichText, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { micromark } from 'micromark'
import { useCallback, useEffect, useState } from 'react'
import { assistantPreviewText } from '../../hooks/ask-reducer'
import { askFailureTitle } from './ask-failure'
import { askErrorReport } from './ask-report'
import './ask-event.scss'

const emptyEvents: AskEvent[] = []

const factLabels: Record<AskFactResult['fact_type'], string> = {
  bath: '洗澡记录',
  vaccine: '疫苗记录',
  deworming: '驱虫记录',
  checkup: '体检记录',
  visit: '就医记录',
  medication: '用药记录',
}

function pad(value: number) {
  return String(value).padStart(2, '0')
}

function formatOccurredAt(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日 ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function FactResult({ data }: { data: AskFactResult }) {
  return (
    <View className="ask-result ask-result--fact">
      <Text className="ask-result-title">{factLabels[data.fact_type]}</Text>
      <View className="ask-fact-list">
        {data.items.map(item => (
          <View className="ask-fact-item" key={item.pet_id}>
            <View className="ask-fact-heading">
              <Text className="ask-fact-pet">{item.pet_name}</Text>
              <Text className={`ask-fact-status${item.found ? '' : ' is-empty'}`}>{item.found ? '已找到' : '暂无记录'}</Text>
            </View>
            {item.found && (
              <View className="ask-fact-detail">
                <Text className="ask-fact-time">{formatOccurredAt(item.occurred_at)}</Text>
                {item.content && <Text className="ask-fact-content">{item.content}</Text>}
              </View>
            )}
          </View>
        ))}
      </View>
    </View>
  )
}

function FamilyPetsResult({ data }: { data: AskFamilyPetsResult }) {
  return (
    <View className="ask-result ask-result--family-pets">
      <Text className="ask-result-title">
        家里的宠物（
        {data.count}
        只）
      </Text>
      <View className="ask-fact-list">
        {data.pets.map(pet => (
          <View className="ask-fact-item" key={pet.pet_id}>
            <Text className="ask-fact-pet">{pet.pet_name}</Text>
          </View>
        ))}
      </View>
    </View>
  )
}

function AnalysisSection({ title, values }: { title: string, values: string[] }) {
  return (
    <View className="ask-analysis-section">
      <Text className="ask-analysis-label">{title}</Text>
      {values.map(value => <Text className="ask-analysis-line" key={value}>{value}</Text>)}
    </View>
  )
}

function AnalysisResult({ data }: { data: AskAnalysisResult }) {
  return (
    <View className="ask-result ask-result--analysis">
      <Text className="ask-result-title">当前判断</Text>
      <Text className="ask-assessment">{data.current_assessment}</Text>
      <AnalysisSection title="我观察到的情况" values={data.observations} />
      <AnalysisSection title="可能原因" values={data.possible_causes} />
      <AnalysisSection title="接下来怎么做" values={data.home_actions} />
      <AnalysisSection title="哪些情况需要就医" values={data.escalation_conditions} />
    </View>
  )
}

function ProgressResult({ data }: { data: AskProgressResult }) {
  const completed = data.stage === 'intent_completed'
  return (
    <View className={`ask-progress${completed ? ' ask-progress--completed' : ''}`}>
      <Text className="ask-progress-dot" />
      <Text>{data.message}</Text>
    </View>
  )
}

function DeltaResult({ delta, onDone }: { delta: string, onDone?: () => void }) {
  const characters = Array.from(delta)
  const characterCount = characters.length
  const [visibleCount, setVisibleCount] = useState(0)

  useEffect(() => {
    if (visibleCount >= characterCount) {
      onDone?.()
      return undefined
    }
    const timer = setInterval(() => {
      setVisibleCount(value => value >= characterCount ? value : Math.min(value + Math.ceil((characterCount - value) / 10), characterCount))
    }, 45)
    return () => clearInterval(timer)
  }, [characterCount, onDone, visibleCount])

  return (
    <View className="ask-copy-result">
      <RichText className="ask-markdown ask-delta" nodes={micromark(characters.slice(0, visibleCount).join(''))} />
      <Button className="ask-copy-button" onClick={() => void copyText(delta)} aria-label="复制回答">复制</Button>
    </View>
  )
}

function AssistantGroups({ groups, coverage, answer }: { groups: AskAnswerGroup[], coverage: AskTaskCoverage[], answer: string }) {
  if (!groups.length) {
    return <DeltaResult delta={answer} />
  }
  return (
    <View className="ask-answer-groups">
      {groups.map(group => (
        <View className="ask-answer-group" key={group.group_key}>
          <View className="ask-answer-subjects">
            {group.subjects.map(subject => <Text key={subject.subject_key}>{subject.kind === 'pet' ? `宠物 ${subject.pet_id}` : subject.description}</Text>)}
          </View>
          {(group.segments ?? []).map(segment => (
            <View className="ask-answer-segment" key={segment.segment_key}>
              <RichText className="ask-markdown" nodes={micromark(segment.text)} />
              {!!segment.evidence_refs?.length && <Text className="ask-answer-evidence">{segment.evidence_refs.map(ref => ref.source_type).join('、')}</Text>}
            </View>
          ))}
          {(group.risks ?? []).map(risk => (
            <View className="ask-answer-risk" key={`${risk.group_key}-${risk.subject_key}`}>
              <Text>{`风险：${risk.level}`}</Text>
              {!!risk.uncertainty && <Text>{risk.uncertainty}</Text>}
              {!!risk.evidence?.length && <Text className="ask-answer-evidence">{risk.evidence.map(ref => ref.source_type).join('、')}</Text>}
            </View>
          ))}
        </View>
      ))}
      {coverage.filter(item => item.incomplete_reason).map(item => (
        <View className="ask-answer-coverage" key={item.task_key}>
          <Text>{`未完成：${item.incomplete_reason}`}</Text>
        </View>
      ))}
    </View>
  )
}

function AssistantResult({ data, preview }: { data: AskAssistantResult, preview: string }) {
  const [previewing, setPreviewing] = useState(preview !== '')
  const handlePreviewDone = useCallback(() => setPreviewing(false), [])
  if (previewing) {
    return <DeltaResult delta={preview} onDone={handlePreviewDone} />
  }
  return <AssistantGroups groups={data.groups ?? []} coverage={data.coverage ?? []} answer={data.answer} />
}

async function copyText(value: string) {
  try {
    await Taro.setClipboardData({ data: value })
  }
  catch {
    await Taro.showToast({ title: '复制失败，请重试', icon: 'none' })
  }
}

export default function AskEventView({ event, input = '', events = emptyEvents, live = false }: { event: AskEvent, input?: string, events?: AskEvent[], live?: boolean }) {
  if (event.type === 'run.progress') {
    return <ProgressResult data={event.data as AskProgressResult} />
  }
  if (event.type === 'assistant.delta') {
    return <DeltaResult delta={(event.data as AskDeltaResult).delta} />
  }
  if (event.type === 'fact.completed') {
    const data = event.data as AskFactResult
    const content = data.items.map(item => `${item.pet_name}：${item.found ? `${formatOccurredAt(item.occurred_at)} ${item.content}` : '暂无记录'}`).join('\n')
    return (
      <View className="ask-copy-result">
        <FactResult data={data} />
        <Button className="ask-copy-button" onClick={() => void copyText(content)} aria-label="复制回答">复制</Button>
      </View>
    )
  }
  if (event.type === 'family.pets.completed') {
    const data = event.data as AskFamilyPetsResult
    return (
      <View className="ask-copy-result">
        <FamilyPetsResult data={data} />
        <Button className="ask-copy-button" onClick={() => void copyText(`家里的宠物（${data.count}只）：${data.pets.map(pet => pet.pet_name).join('、')}`)} aria-label="复制回答">复制</Button>
      </View>
    )
  }
  if (event.type === 'assistant.question') {
    const data = event.data as AskQuestionResult
    return (
      <View className="ask-message ask-message--assistant">
        <RichText className="ask-markdown" nodes={micromark(data.question)} />
        <Button className="ask-copy-button" onClick={() => void copyText(data.question)} aria-label="复制回答">复制</Button>
      </View>
    )
  }
  if (event.type === 'assistant.completed') {
    const data = event.data as AskAssistantResult
    const preview = live && data.groups?.length ? assistantPreviewText(events, event.run_id) : ''
    return (
      <View className="ask-message ask-message--assistant">
        <AssistantResult data={data} preview={preview} />
        <Button className="ask-copy-button" onClick={() => void copyText(data.answer)} aria-label="复制回答">复制</Button>
      </View>
    )
  }
  if (event.type === 'risk.escalated') {
    const data = event.data as AskRiskResult
    return (
      <View className="ask-result ask-result--risk">
        <Text className="ask-risk-level">需要立即处理</Text>
        <Text className="ask-result-title">{data.message}</Text>
        <Text className="ask-risk-action">{data.action}</Text>
        <Button className="ask-copy-button" onClick={() => void copyText(`${data.message}\n${data.action}`)} aria-label="复制回答">复制</Button>
      </View>
    )
  }
  if (event.type === 'run.completed') {
    const data = event.data as AskAnalysisResult
    const content = [data.current_assessment, ...data.observations, ...data.possible_causes, ...data.home_actions, ...data.escalation_conditions].join('\n')
    return (
      <View className="ask-copy-result">
        <AnalysisResult data={data} />
        <Button className="ask-copy-button" onClick={() => void copyText(content)} aria-label="复制回答">复制</Button>
      </View>
    )
  }
  if (event.type === 'run.failed') {
    const data = event.data as AskFailedResult
    const message = data.message || `错误代码：${data.error_code || 'unknown'}，服务端未返回错误详情。`
    const report = TARO_APP_DEBUG ? askErrorReport(input, events.filter(value => value.sequence <= event.sequence), `${data.error_code || 'unknown'}：${message}`, `run_id=${event.run_id}`) : `${askFailureTitle(data.error_code)}：${message}`
    return (
      <View className="ask-result ask-result--failed">
        <Text className="ask-result-title">{askFailureTitle(data.error_code)}</Text>
        <Text>{message}</Text>
        {TARO_APP_DEBUG && <Text className="ask-error-report">{report}</Text>}
        <Button className="ask-copy-button" onClick={() => void copyText(report)} aria-label="复制错误报告">复制错误报告</Button>
      </View>
    )
  }
  return null
}
