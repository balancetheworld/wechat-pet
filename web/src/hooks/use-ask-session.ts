import type { AskEvent, AskExecution, AskSnapshot } from '../types/ask'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import Taro from '@tarojs/taro'
import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { continueAskSession, createAskSession, getAskSnapshot, replyAskRun, retryAskRun, stopAskRun } from '../services/ask'
import { openAskEventStream } from '../services/ask-stream'
import { ApiError } from '../services/request'
import { useAskStore } from '../stores/ask-store'
import { askReducer, hasSequenceGap, initialAskRuntimeState } from './ask-reducer'
import { shouldReconnectAfterSnapshotError, shouldReconnectAfterStreamError } from './ask-session-reconnect'

export const askQueryKeys = {
  session: (sessionID: string) => ['ask', 'session', sessionID] as const,
  execution: (sessionID: string, runID: string) => ['ask', 'execution', sessionID, runID] as const,
  eventLog: (sessionID: string, runID: string) => ['ask', 'events', sessionID, runID] as const,
  snapshot: (sessionID: string) => ['ask', 'snapshot', sessionID] as const,
}

function latestSequence(events: AskEvent[]) {
  return events.reduce((value, event) => Math.max(value, event.sequence), 0)
}

function mergeEvents(current: AskEvent[] | undefined, incoming: AskEvent[]) {
  const values = new Map<string, AskEvent>()
  for (const event of current ?? []) {
    values.set(`${event.run_id}:${event.sequence}`, event)
  }
  for (const event of incoming) {
    values.set(`${event.run_id}:${event.sequence}`, event)
  }
  return Array.from(values.values()).sort((left, right) => left.sequence - right.sequence)
}

function isTerminalEvent(event: AskEvent) {
  return event.type === 'assistant.completed' || event.type === 'assistant.question' || event.type === 'fact.completed' || event.type === 'family.pets.completed' || event.type === 'run.completed' || event.type === 'risk.escalated' || event.type === 'run.failed'
}

function isTerminalRunStatus(status: AskExecution['run']['status']) {
  return status === 'waiting_input' || status === 'completed' || status === 'failed' || status === 'canceled' || status === 'interrupted'
}

function isFollowUpRunStatus(status: AskExecution['run']['status']) {
  return status === 'completed' || status === 'failed' || status === 'canceled' || status === 'interrupted'
}

function errorPhase(error: unknown) {
  if (!(error instanceof ApiError)) {
    return 'network_error' as const
  }
  if (error.statusCode === 400 || error.code === 40001) {
    return 'input_error' as const
  }
  if (error.kind === 'network' || error.kind === 'timeout') {
    return 'network_error' as const
  }
  return 'failed' as const
}

function errorMessage(error: unknown) {
  const message = error instanceof Error ? error.message : '问问请求失败'
  if (!TARO_APP_DEBUG || !(error instanceof ApiError)) {
    return message
  }
  return [message, `类型：${error.kind}`, error.statusCode ? `HTTP：${error.statusCode}` : '', error.code ? `业务代码：${error.code}` : '', error.requestId ? `请求 ID：${error.requestId}` : ''].filter(Boolean).join('；')
}

function localRunID() {
  return `local-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function idempotencyKey() {
  return `ask-${Date.now()}-${Math.random().toString(16).slice(2)}-${Math.random().toString(16).slice(2)}`
}

function isRetryableRequest(error: unknown) {
  return !(error instanceof ApiError) || error.kind === 'network' || error.kind === 'timeout'
}

interface PendingRequest {
  kind: 'create' | 'continue' | 'reply'
  input: string
  assetRefs?: string[]
  idempotencyKey: string
  sessionID?: string
  runID?: string
  expectedVersion?: number
}

export function useAskSession() {
  const queryClient = useQueryClient()
  const [runtime, dispatch] = useReducer(askReducer, initialAskRuntimeState, state => useAskStore.getState().activeSessionId ? { ...state, phase: 'reconnecting' as const } : state)
  const [pageVisible, setPageVisible] = useState(true)
  const [connectionVersion, setConnectionVersion] = useState(0)
  const pendingRequest = useRef<PendingRequest | null>(null)
  const draft = useAskStore(state => state.draft)
  const activeSessionId = useAskStore(state => state.activeSessionId)
  const activeRunId = useAskStore(state => state.activeRunId)
  const activeRunVersion = useAskStore(state => state.activeRunVersion)
  const setDraft = useAskStore(state => state.setDraft)
  const resetStore = useAskStore(state => state.reset)

  Taro.useDidShow(() => {
    setPageVisible(true)
  })

  Taro.useDidHide(() => {
    setPageVisible(false)
  })

  const syncEvents = useCallback((sessionID: string, runID: string, events: AskEvent[]) => {
    queryClient.setQueryData<AskEvent[]>(askQueryKeys.eventLog(sessionID, runID), current => mergeEvents(current, events))
    dispatch({ type: 'events.received', events })
    const sequence = latestSequence(events)
    if (sequence > useAskStore.getState().lastEventSequence) {
      useAskStore.getState().setLastEventSequence(sequence)
    }
  }, [queryClient])

  function restoreSnapshot(value: AskExecution) {
    queryClient.setQueryData(askQueryKeys.session(value.session.id), value.session)
    queryClient.setQueryData(askQueryKeys.execution(value.session.id, value.run.id), value)
    queryClient.setQueryData<AskEvent[]>(askQueryKeys.eventLog(value.session.id, value.run.id), current => mergeEvents(current, value.events))
    useAskStore.getState().setActiveExecution(value.session.id, value.run.id, value.run.row_version, latestSequence(value.events))
    dispatch({ type: 'snapshot.restored', execution: value })
  }

  const restoreFullSnapshot = useCallback((value: AskSnapshot) => {
    queryClient.setQueryData(askQueryKeys.snapshot(value.session.id), value)
    queryClient.setQueryData(askQueryKeys.session(value.session.id), value.session)
    for (const turn of value.turns) {
      queryClient.setQueryData(askQueryKeys.execution(value.session.id, turn.run.id), {
        session: value.session,
        run: turn.run,
        events: turn.events,
      })
      queryClient.setQueryData<AskEvent[]>(askQueryKeys.eventLog(value.session.id, turn.run.id), turn.events)
    }
    const selected = value.turns[value.turns.length - 1]
    const sequence = selected ? latestSequence(selected.events) : 0
    if (selected) {
      useAskStore.getState().setActiveExecution(value.session.id, selected.run.id, selected.run.row_version, sequence)
    }
    dispatch({ type: 'snapshot.loaded', snapshot: value })
  }, [queryClient])

  const createMutation = useMutation({
    mutationFn: ({ input, assetRefs, key }: { input: string, assetRefs?: string[], key: string }) => createAskSession({ input, asset_refs: assetRefs }, key),
    onSuccess: restoreSnapshot,
    onError(error) {
      dispatch({ type: 'request.failed', phase: error instanceof ApiError && error.statusCode === 409 ? 'ambiguous' : errorPhase(error), message: errorMessage(error) })
    },
  })

  const replyMutation = useMutation({
    mutationFn: ({ sessionID, runID, input, assetRefs, expectedVersion, key }: { sessionID: string, runID: string, input: string, assetRefs?: string[], expectedVersion: number, key: string }) => replyAskRun(sessionID, runID, { input, asset_refs: assetRefs, expected_version: expectedVersion }, key),
    onSuccess: restoreSnapshot,
    onError(error) {
      dispatch({ type: 'request.failed', phase: errorPhase(error), message: errorMessage(error) })
    },
  })

  const continueMutation = useMutation({
    mutationFn: ({ sessionID, input, assetRefs, key }: { sessionID: string, input: string, assetRefs?: string[], key: string }) => continueAskSession(sessionID, { input, asset_refs: assetRefs }, key),
    onSuccess: restoreSnapshot,
    onError(error) {
      dispatch({ type: 'request.failed', phase: errorPhase(error), message: errorMessage(error) })
    },
  })

  useEffect(() => {
    if (!pageVisible || !activeSessionId || !activeRunId) {
      return
    }
    const sessionID = activeSessionId
    const runID = activeRunId
    let disposed = false
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let stream: ReturnType<typeof openAskEventStream> | null = null
    let recovering = false
    function reconnect() {
      if (disposed || reconnectTimer) {
        return
      }
      dispatch({ type: 'stream.reconnecting' })
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null
        connect()
      }, 1000)
    }
    function connect() {
      if (disposed) {
        return
      }
      stream = openAskEventStream({
        sessionID,
        runID,
        after: useAskStore.getState().lastEventSequence,
        onEvents(events) {
          if (disposed) {
            return
          }
          if (hasSequenceGap(events, useAskStore.getState().lastEventSequence)) {
            void recover()
            return
          }
          syncEvents(sessionID, runID, events)
          if (events.some(isTerminalEvent)) {
            void recover()
          }
        },
        onError(error) {
          if (disposed || recovering) {
            return
          }
          if (!shouldReconnectAfterStreamError(error)) {
            disposed = true
            dispatch({ type: 'request.failed', phase: 'failed', message: error.message })
            return
          }
          reconnect()
        },
        onClose() {
          if (disposed || recovering) {
            return
          }
          reconnect()
        },
      })
    }
    async function recover() {
      if (disposed || recovering) {
        return
      }
      recovering = true
      stream?.close()
      dispatch({ type: 'stream.reconnecting' })
      try {
        const snapshot = await getAskSnapshot(sessionID)
        if (disposed) {
          return
        }
        restoreFullSnapshot(snapshot)
        recovering = false
        const selected = snapshot.turns[snapshot.turns.length - 1]
        if (!selected || !isTerminalRunStatus(selected.run.status)) {
          connect()
        }
      }
      catch (error) {
        recovering = false
        if (!disposed) {
          dispatch({ type: 'request.failed', phase: errorPhase(error), message: errorMessage(error) })
          if (shouldReconnectAfterSnapshotError(error)) {
            reconnect()
          }
        }
      }
    }
    async function initialize() {
      dispatch({ type: 'snapshot.loading' })
      try {
        const snapshot = await getAskSnapshot(sessionID)
        if (disposed) {
          return
        }
        restoreFullSnapshot(snapshot)
        const selected = snapshot.turns[snapshot.turns.length - 1]
        if (!selected || !isTerminalRunStatus(selected.run.status)) {
          connect()
        }
      }
      catch (error) {
        if (!disposed) {
          if (!shouldReconnectAfterSnapshotError(error)) {
            disposed = true
            dispatch({ type: 'request.failed', phase: 'failed', message: errorMessage(error) })
          }
          else {
            connect()
          }
        }
      }
    }
    void initialize()
    return () => {
      disposed = true
      stream?.close()
      if (reconnectTimer) {
        clearTimeout(reconnectTimer)
      }
    }
  }, [activeRunId, activeRunVersion, activeSessionId, connectionVersion, pageVisible, restoreFullSnapshot, syncEvents])

  async function submit(input = draft, assetRefs: string[] = []) {
    const value = input.trim()
    if (activeSessionId && (!runtime.session || !runtime.run || runtime.session.id !== activeSessionId)) {
      return
    }
    if (activeSessionId && runtime.run && isFollowUpRunStatus(runtime.run.status)) {
      const current = pendingRequest.current
      const pending = current?.kind === 'continue' && current.input === value && current.sessionID === activeSessionId ? current : { kind: 'continue' as const, input: value, assetRefs, idempotencyKey: idempotencyKey(), sessionID: activeSessionId }
      const reused = pending === current
      pendingRequest.current = pending
      if (!reused) {
        dispatch({ type: 'local.followed_up', input: value, clientRunID: localRunID() })
      }
      try {
        const continued = await continueMutation.mutateAsync({ sessionID: activeSessionId, input: value, assetRefs: pending.assetRefs, key: pending.idempotencyKey })
        pendingRequest.current = null
        useAskStore.getState().setDraft('')
        return continued
      }
      catch (error) {
        if (!isRetryableRequest(error)) {
          pendingRequest.current = null
        }
        throw error
      }
    }
    const current = pendingRequest.current
    const pending = current?.kind === 'create' && current.input === value ? current : { kind: 'create' as const, input: value, assetRefs, idempotencyKey: idempotencyKey() }
    const reused = pending === current
    pendingRequest.current = pending
    if (!reused) {
      dispatch({ type: 'local.submitted', input: value, clientRunID: localRunID() })
    }
    try {
      const created = await createMutation.mutateAsync({ input: value, assetRefs: pending.assetRefs, key: pending.idempotencyKey })
      pendingRequest.current = null
      useAskStore.getState().setDraft('')
      return created
    }
    catch (error) {
      if (!isRetryableRequest(error)) {
        pendingRequest.current = null
      }
      throw error
    }
  }

  async function reply(input = draft, assetRefs: string[] = []) {
    if (!activeSessionId || !activeRunId || !runtime.run) {
      return
    }
    const value = input.trim()
    const expectedVersion = runtime.run.row_version
    const current = pendingRequest.current
    const pending = current?.kind === 'reply' && current.input === value && current.sessionID === activeSessionId && current.runID === activeRunId && current.expectedVersion === expectedVersion ? current : { kind: 'reply' as const, input: value, assetRefs, idempotencyKey: idempotencyKey(), sessionID: activeSessionId, runID: activeRunId, expectedVersion }
    const reused = pending === current
    pendingRequest.current = pending
    if (!reused) {
      dispatch({ type: 'local.replied', input: value, clientRunID: localRunID(), runID: activeRunId })
    }
    try {
      const queued = await replyMutation.mutateAsync({ sessionID: activeSessionId, runID: activeRunId, input: value, assetRefs: pending.assetRefs, expectedVersion, key: pending.idempotencyKey })
      pendingRequest.current = null
      useAskStore.getState().setDraft('')
      return queued
    }
    catch (error) {
      if (!isRetryableRequest(error)) {
        pendingRequest.current = null
      }
      throw error
    }
  }

  function retryConnection() {
    if (!activeSessionId || !activeRunId) {
      return
    }
    dispatch({ type: 'stream.reconnecting' })
    setConnectionVersion(value => value + 1)
  }

  async function stop() {
    if (!activeSessionId || !activeRunId || !runtime.run) {
      return
    }
    try {
      restoreSnapshot(await stopAskRun(activeSessionId, activeRunId, runtime.run.row_version))
    }
    catch (error) {
      /* 409 表示运行已进入终态(完成/失败/已停止), 拉一次快照对齐即可, 不当作失败 */
      if (!(error instanceof ApiError) || error.statusCode !== 409) {
        throw error
      }
      restoreFullSnapshot(await getAskSnapshot(activeSessionId))
    }
  }

  async function retry() {
    if (!activeSessionId || !activeRunId || !runtime.run) {
      return
    }
    restoreSnapshot(await retryAskRun(activeSessionId, activeRunId, runtime.run.row_version))
  }

  function reset() {
    pendingRequest.current = null
    createMutation.reset()
    replyMutation.reset()
    continueMutation.reset()
    dispatch({ type: 'reset' })
    resetStore()
  }

  return {
    draft,
    phase: runtime.phase,
    error: runtime.error,
    session: runtime.session,
    run: runtime.run,
    conversation: runtime.turns,
    setDraft,
    submit,
    reply,
    retryConnection,
    stop,
    retry,
    reset,
  }
}
