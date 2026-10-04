import type { AskOperation } from '../../types/ask'
import { Button, Text, View } from '@tarojs/components'
import { useState } from 'react'
import './ask-operation.scss'

const visibleStatuses = new Set<AskOperation['status']>(['pending', 'confirmed', 'executing', 'succeeded', 'failed'])
const calendarRecordCreateTarget = 'calendar.record.create'

/* 档案同步目标: 只支持成长足迹页; 生日纪念页只记录生日内容, 不接收日历记录同步 */
const syncOptions = [
  { key: 'growth', label: '同步到成长足迹页' },
]

function syncOptionLabel(key: string) {
  return syncOptions.find(option => option.key === key)?.label || key
}

function statusTitle(operation: AskOperation) {
  switch (operation.status) {
    case 'pending':
      return '待确认写入'
    case 'confirmed':
      return '已确认，等待写入'
    case 'executing':
      return '正在写入'
    case 'succeeded':
      return '已写入'
    default:
      return '写入失败'
  }
}

/* 执行结果里带 synced / sync_failed 时说明是日历记录, 据此回显档案同步结果 */
function calendarSyncOutcome(operation: AskOperation) {
  if (!operation.result) {
    return null
  }
  try {
    const parsed = JSON.parse(operation.result) as Record<string, unknown>
    if (!Array.isArray(parsed.synced) && !Array.isArray(parsed.sync_failed)) {
      return null
    }
    return {
      synced: Array.isArray(parsed.synced) ? parsed.synced.filter((item): item is string => typeof item === 'string') : [],
      failed: Array.isArray(parsed.sync_failed) ? parsed.sync_failed.filter((item): item is string => typeof item === 'string') : [],
    }
  }
  catch {
    return null
  }
}

function successText(operation: AskOperation) {
  const outcome = calendarSyncOutcome(operation)
  if (!outcome) {
    return '记录已保存到宠物日历'
  }
  const synced = outcome.synced.map(syncOptionLabel)
  const failed = outcome.failed.map(syncOptionLabel)
  if (synced.length > 0 && failed.length === 0) {
    return `记录已保存到宠物日历，并${synced.join('、')}`
  }
  if (synced.length > 0) {
    return `记录已保存到宠物日历，${synced.join('、')}成功，${failed.join('、')}失败`
  }
  if (failed.length > 0) {
    return `记录已保存到宠物日历，但${failed.join('、')}失败`
  }
  return '记录已保存到宠物日历'
}

export default function AskOperationCard({ operation, busy = false, onConfirm, onAbandon }: {
  operation: AskOperation
  busy?: boolean
  onConfirm?: (operation: AskOperation, syncTargets?: string[]) => void
  onAbandon?: (operation: AskOperation) => void
}) {
  /* 初始勾选服务端给出的建议目标, 用户可在确认前自行增删 */
  const [syncTargets, setSyncTargets] = useState<string[]>(() => operation.sync_targets || [])
  if (!visibleStatuses.has(operation.status)) {
    return null
  }
  const chooseSync = operation.status === 'pending' && operation.target === calendarRecordCreateTarget
  return (
    <View className={`ask-operation ask-operation--${operation.status}`}>
      <Text className="ask-operation-title">{statusTitle(operation)}</Text>
      <Text className="ask-operation-preview">{operation.preview}</Text>
      {operation.status === 'succeeded' && <Text className="ask-operation-result">{successText(operation)}</Text>}
      {operation.status === 'failed' && <Text className="ask-operation-result">写入未完成，请重试或联系客服</Text>}
      {chooseSync && (
        <View className="ask-operation-sync">
          <Text className="ask-operation-sync-title">同步到档案（可选）</Text>
          <View className="ask-operation-sync-options">
            {syncOptions.map(option => (
              <Button
                key={option.key}
                className={`ask-operation-sync-option${syncTargets.includes(option.key) ? ' selected' : ''}`}
                disabled={busy}
                onClick={() => setSyncTargets(value => value.includes(option.key) ? value.filter(item => item !== option.key) : [...value, option.key])}
              >
                {option.label}
              </Button>
            ))}
          </View>
        </View>
      )}
      {(operation.status === 'pending' || operation.status === 'confirmed') && (
        <View className="ask-operation-actions">
          <Button className="ask-operation-button" disabled={busy} onClick={() => onConfirm?.(operation, chooseSync ? syncTargets : undefined)}>
            {operation.status === 'pending' ? '确认并写入' : '执行写入'}
          </Button>
          {(operation.status === 'pending' || operation.status === 'confirmed') && (
            <Button className="ask-operation-button ask-operation-button--ghost" disabled={busy} onClick={() => onAbandon?.(operation)}>
              不用了
            </Button>
          )}
        </View>
      )}
    </View>
  )
}
