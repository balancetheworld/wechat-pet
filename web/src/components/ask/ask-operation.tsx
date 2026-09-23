import type { AskOperation } from '../../types/ask'
import { Button, Text, View } from '@tarojs/components'
import './ask-operation.scss'

const visibleStatuses = new Set<AskOperation['status']>(['pending', 'confirmed', 'executing', 'succeeded', 'failed'])

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

export default function AskOperationCard({ operation, busy = false, onConfirm, onAbandon }: {
  operation: AskOperation
  busy?: boolean
  onConfirm?: (operation: AskOperation) => void
  onAbandon?: (operation: AskOperation) => void
}) {
  if (!visibleStatuses.has(operation.status)) {
    return null
  }
  return (
    <View className={`ask-operation ask-operation--${operation.status}`}>
      <Text className="ask-operation-title">{statusTitle(operation)}</Text>
      <Text className="ask-operation-preview">{operation.preview}</Text>
      {operation.status === 'succeeded' && <Text className="ask-operation-result">记录已保存到宠物日历</Text>}
      {operation.status === 'failed' && <Text className="ask-operation-result">写入未完成，请重试或联系客服</Text>}
      {(operation.status === 'pending' || operation.status === 'confirmed') && (
        <View className="ask-operation-actions">
          <Button className="ask-operation-button" disabled={busy} onClick={() => onConfirm?.(operation)}>
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
