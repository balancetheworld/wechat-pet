import type { AskEventStreamError } from '../services/ask-stream'
import { expect, it } from 'vitest'
import { ApiError } from '../types/api'
import { shouldReconnectAfterSnapshotError, shouldReconnectAfterStreamError } from './ask-session-reconnect'

it('快照恢复请求返回 401 时停止重连', () => {
  const error = new ApiError('登录已失效', { kind: 'auth', statusCode: 401 })

  expect(shouldReconnectAfterSnapshotError(error)).toBe(false)
})

it('事件流请求返回 401 时停止重连', () => {
  const error = { retryable: true, statusCode: 401 } as AskEventStreamError

  expect(shouldReconnectAfterStreamError(error)).toBe(false)
})
