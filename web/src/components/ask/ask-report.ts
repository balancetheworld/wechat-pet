import type { AskEvent } from '../../types/ask'

export function askErrorReport(input: string, events: AskEvent[], error: string, context = '') {
  const process = events.length
    ? events.map(event => `${event.sequence}. ${event.created_at} ${event.type}: ${JSON.stringify(event.data)}`).join('\n')
    : '未收到过程事件（请求可能在创建或连接阶段失败）'
  return [
    `输入：${input || '仅上传图片或输入未能恢复'}`,
    context ? `上下文：${context}` : '',
    `中间过程：\n${process}`,
    `最终错误：${error}`,
  ].filter(Boolean).join('\n\n')
}
