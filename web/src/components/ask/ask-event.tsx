import type { AskAnalysisResult, AskAnswerGroup, AskAnswerSubject, AskAssistantResult, AskDeltaResult, AskEvent, AskFactResult, AskFailedResult, AskFamilyPetsResult, AskProgressResult, AskQuestionResult, AskRiskResult, AskTaskCoverage } from '../../types/ask'
import { Button, RichText, ScrollView, Text, View } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { micromark } from 'micromark'
import { askFailureTitle } from './ask-failure'
import { askErrorReport } from './ask-report'
import './ask-event.scss'

const emptyEvents: AskEvent[] = []

// 小程序 rich-text 不支持 class，外部 CSS 作用不到内部节点，因此把排版写成内联样式。
// 字号与行高沿用外层 .ask-markdown（rpx），这里只控制间距、列表符号与字重。
const markdownStyles: Record<string, string> = {
  p: 'margin:0 0 12px;',
  ul: 'margin:0 0 12px;padding-left:26px;list-style-type:disc;',
  ol: 'margin:0 0 12px;padding-left:26px;list-style-type:decimal;',
  li: 'margin:0 0 6px;',
  strong: 'font-weight:600;',
}

function renderMarkdown(text: string) {
  return micromark(text).replace(/<(p|ul|ol|li|strong)(\s[^>]*)?>/g, (matched, tag: string, attributes?: string) => {
    const style = markdownStyles[tag]
    if (!style) {
      return matched
    }
    return attributes ? `<${tag}${attributes} style="${style}">` : `<${tag} style="${style}">`
  })
}

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

function DeltaResult({ delta }: { delta: string }) {
  return (
    <View className="ask-copy-result">
      <RichText className="ask-markdown ask-delta" nodes={renderMarkdown(delta)} />
      <Button className="ask-copy-button" onClick={() => void copyText(delta)} aria-label="复制回答">复制</Button>
    </View>
  )
}

function ThinkingResult({ delta }: { delta: string }) {
  return (
    <View className="ask-thinking">
      <Text className="ask-thinking-label">思考过程</Text>
      <ScrollView className="ask-thinking-scroll" scrollY showScrollbar={false} scrollTop={delta.length * 100}>
        <Text className="ask-thinking-text">{delta}</Text>
      </ScrollView>
    </View>
  )
}

// subjectLabel 生成对象标签：已确认对象显示宠物名字而不是内部 ID，
// 未明确对象只在有描述时显示，占位内容（例如 "turn"）不展示给用户。
function subjectLabel(subject: AskAnswerSubject, petNames?: Record<string, string>) {
  if (subject.kind === 'pet') {
    const name = subject.pet_id ? petNames?.[subject.pet_id] : ''
    return name ? `宠物 ${name}` : '宠物'
  }
  return (subject.description ?? '').trim()
}

function AssistantGroups({ groups, coverage, answer, petNames }: { groups: AskAnswerGroup[], coverage: AskTaskCoverage[], answer: string, petNames?: Record<string, string> }) {
  if (!groups.length) {
    return <DeltaResult delta={answer} />
  }
  return (
    <View className="ask-answer-groups">
      {groups.map(group => (
        <View className="ask-answer-group" key={group.group_key}>
          <View className="ask-answer-subjects">
            {group.subjects.map((subject) => {
              const label = subjectLabel(subject, petNames)
              return label ? <Text key={subject.subject_key}>{label}</Text> : null
            })}
          </View>
          {(group.segments ?? []).map(segment => (
            <View className="ask-answer-segment" key={segment.segment_key}>
              <RichText className="ask-markdown" nodes={renderMarkdown(segment.text)} />
            </View>
          ))}
          {(group.risks ?? []).map(risk => (
            <View className="ask-answer-risk" key={`${risk.group_key}-${risk.subject_key}`}>
              <Text>{`风险：${risk.level}`}</Text>
              {!!risk.uncertainty && <Text>{risk.uncertainty}</Text>}
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

function AssistantResult({ data, petNames }: { data: AskAssistantResult, petNames?: Record<string, string> }) {
  return <AssistantGroups groups={data.groups ?? []} coverage={data.coverage ?? []} answer={data.answer} petNames={petNames} />
}

async function copyText(value: string) {
  try {
    await Taro.setClipboardData({ data: value })
  }
  catch {
    await Taro.showToast({ title: '复制失败，请重试', icon: 'none' })
  }
}

export default function AskEventView({ event, input = '', events = emptyEvents, petNames }: { event: AskEvent, input?: string, events?: AskEvent[], petNames?: Record<string, string> }) {
  if (event.type === 'run.progress') {
    return <ProgressResult data={event.data as AskProgressResult} />
  }
  if (event.type === 'assistant.delta') {
    return <DeltaResult delta={(event.data as AskDeltaResult).delta} />
  }
  if (event.type === 'assistant.thinking') {
    return <ThinkingResult delta={(event.data as AskDeltaResult).delta} />
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
        <RichText className="ask-markdown" nodes={renderMarkdown(data.question)} />
        <Button className="ask-copy-button" onClick={() => void copyText(data.question)} aria-label="复制回答">复制</Button>
      </View>
    )
  }
  if (event.type === 'assistant.completed') {
    const data = event.data as AskAssistantResult
    return (
      <View className="ask-message ask-message--assistant">
        <AssistantResult data={data} petNames={petNames} />
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
