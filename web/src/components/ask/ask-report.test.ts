import type { AskEvent } from '../../types/ask'
import { expect, it } from 'vitest'
import { askErrorReport } from './ask-report'

it('includes input, ordered process details and final error', () => {
  const events: AskEvent[] = [{ run_id: 'run-1', sequence: 1, type: 'run.progress', data: { stage: 'context_ready', message: '已读取记录' }, created_at: '2026-09-23T01:00:00Z' }]
  const report = askErrorReport('旺仔怎么了', events, 'provider_timeout：请求超时', 'run_id=run-1')
  expect(report).toContain('输入：旺仔怎么了')
  expect(report).toContain('run.progress: {"stage":"context_ready","message":"已读取记录"}')
  expect(report).toContain('最终错误：provider_timeout：请求超时')
})

it('does not invent server progress when request fails before events arrive', () => {
  const report = askErrorReport('测试', [], '网络连接失败')
  expect(report).toContain('未收到过程事件')
  expect(report).toContain('最终错误：网络连接失败')
})
