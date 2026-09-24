import type { AskEvent } from '../types/ask'
import Taro from '@tarojs/taro'
import { useAuthStore } from '../stores/auth-store'
import { NDJSONDecoder, read } from './ndjson'
import { ApiError, requestURL } from './request'

interface AskEventStreamOptions {
  sessionID: string
  runID: string
  after: number
  onEvents: (events: AskEvent[]) => void
  onError: (error: AskEventStreamError) => void
  onClose: () => void
}

export interface AskEventStream {
  close: () => void
}

export class AskEventStreamError extends Error {
  readonly retryable: boolean
  readonly statusCode?: number

  constructor(message: string, retryable: boolean, statusCode?: number) {
    super(message)
    this.name = 'AskEventStreamError'
    this.retryable = retryable
    this.statusCode = statusCode
  }
}

function streamPath(sessionID: string, runID: string, after: number) {
  return `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/runs/${encodeURIComponent(runID)}/events/stream?after=${after}`
}

function streamError(error: unknown) {
  if (error instanceof AskEventStreamError) {
    return error
  }
  if (error instanceof ApiError || error instanceof SyntaxError) {
    return new AskEventStreamError(error.message, false, error instanceof ApiError ? error.statusCode : undefined)
  }
  return new AskEventStreamError(error instanceof Error ? error.message : '事件流连接失败', true)
}

function isAskEvent(value: unknown): value is AskEvent {
  if (!value || typeof value !== 'object') {
    return false
  }
  const event = value as Partial<AskEvent>
  return typeof event.run_id === 'string'
    && Number.isInteger(event.sequence)
    && Number(event.sequence) > 0
    && typeof event.type === 'string'
    && typeof event.created_at === 'string'
    && Boolean(event.data)
    && typeof event.data === 'object'
}

function emitEvents(options: AskEventStreamOptions, values: unknown[]) {
  if (!values.every(isAskEvent)) {
    throw new AskEventStreamError('事件流协议格式错误', false)
  }
  options.onEvents(values)
}

function openWebStream(options: AskEventStreamOptions, url: string, token: string): AskEventStream {
  const controller = new AbortController()
  void fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    signal: controller.signal,
  }).then(async (response) => {
    if (!response.ok) {
      throw new ApiError(`请求失败，HTTP 状态码：${response.status}`, { kind: 'http', statusCode: response.status })
    }
    if (!response.body) {
      throw new AskEventStreamError('当前环境不支持流式响应', false)
    }
    await read(response.body.getReader(), new NDJSONDecoder<unknown>(), values => emitEvents(options, values))
    options.onClose()
  }).catch((error: unknown) => {
    if (!controller.signal.aborted) {
      options.onError(streamError(error))
    }
  })
  return { close: () => controller.abort() }
}

function openMiniProgramStream(options: AskEventStreamOptions, url: string, token: string): AskEventStream {
  const decoder = new NDJSONDecoder<unknown>()
  let closed = false
  const task = Taro.request<ArrayBuffer>({
    url,
    method: 'GET',
    header: token ? { Authorization: `Bearer ${token}` } : {},
    enableChunked: true,
    responseType: 'arraybuffer',
    timeout: 30_000,
  })
  task.onChunkReceived(({ data }) => {
    try {
      const events = decoder.push(data)
      if (events.length) {
        emitEvents(options, events)
      }
    }
    catch (error) {
      closed = true
      task.abort()
      options.onError(streamError(error))
    }
  })
  void task.then((response) => {
    if (closed) {
      return
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new ApiError(`请求失败，HTTP 状态码：${response.statusCode}`, { kind: 'http', statusCode: response.statusCode })
    }
    const events = decoder.finish()
    if (events.length) {
      emitEvents(options, events)
    }
    options.onClose()
  }).catch((error: unknown) => {
    if (!closed) {
      options.onError(streamError(error))
    }
  })
  return {
    close() {
      closed = true
      task.abort()
    },
  }
}

export function openAskEventStream(options: AskEventStreamOptions): AskEventStream {
  const token = useAuthStore.getState().token
  const url = requestURL(streamPath(options.sessionID, options.runID, options.after))
  if (Taro.getEnv() === Taro.ENV_TYPE.WEB) {
    return openWebStream(options, url, token)
  }
  return openMiniProgramStream(options, url, token)
}
