import type { AskAnalysisResult, AskEvent, AskFactResult, AskQuestionResult, AskRiskResult } from '../../types/ask'
import { Text, View } from '@tarojs/components'
import './ask-event.scss'

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

export default function AskEventView({ event }: { event: AskEvent }) {
  if (event.type === 'fact.completed') {
    return <FactResult data={event.data as AskFactResult} />
  }
  if (event.type === 'assistant.question') {
    const data = event.data as AskQuestionResult
    return (
      <View className="ask-message ask-message--assistant">
        <Text>{data.question}</Text>
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
      </View>
    )
  }
  if (event.type === 'run.completed') {
    return <AnalysisResult data={event.data as AskAnalysisResult} />
  }
  if (event.type === 'run.failed') {
    return (
      <View className="ask-result ask-result--failed">
        <Text className="ask-result-title">这次没有完成</Text>
        <Text>当前暂时无法完成分析，请稍后重试。</Text>
      </View>
    )
  }
  return null
}
