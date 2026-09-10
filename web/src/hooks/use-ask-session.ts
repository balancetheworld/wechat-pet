import type { AskEvent, AskExecution, AskSnapshot } from '../types/ask'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useReducer, useRef } from 'react'
import { createAskSession, getAskSnapshot, processAskRun, replyAskRun } from '../services/ask'
import { openAskEventStream } from '../services/ask-stream'
import { ApiError } from '../services/request'
import { useAskStore } from '../stores/ask-store'
import { askReducer, hasSequenceGap, initialAskRuntimeState } from './ask-reducer'

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
  return event.type === 'assistant.question' || event.type === 'fact.completed' || event.type === 'run.completed' || event.type === 'risk.escalated' || event.type === 'run.failed'
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
  return error instanceof Error ? error.message : '问问请求失败'
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
  kind: 'create' | 'reply'
  input: string
  idempotencyKey: string
  sessionID?: string
  runID?: string
  expectedVersion?: number
}

export function useAskSession() {
  const queryClient = useQueryClient()
  const [runtime, dispatch] = useReducer(askReducer, initialAskRuntimeState)
  const pendingRequest = useRef<PendingRequest | null>(null)
  const draft = useAskStore(state => state.draft)
  const activeSessionId = useAskStore(state => state.activeSessionId)
  const activeRunId = useAskStore(state => state.activeRunId)
  const activeRunVersion = useAskStore(state => state.activeRunVersion)
  const setDraft = useAskStore(state => state.setDraft)
  const resetStore = useAskStore(state => state.reset)

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
    mutationFn: ({ input, key }: { input: string, key: string }) => createAskSession({ input }, key),
    onSuccess: restoreSnapshot,
    onError(error) {
      dispatch({ type: 'request.failed', phase: error instanceof ApiError && error.statusCode === 409 ? 'ambiguous' : errorPhase(error), message: errorMessage(error) })
    },
  })

  const processMutation = useMutation({
    mutationFn: ({ sessionID, runID }: { sessionID: string, runID: string }) => processAskRun(sessionID, runID),
    onSuccess: restoreSnapshot,
    onError(error) {
      dispatch({ type: 'request.failed', phase: errorPhase(error), message: errorMessage(error) })
    },
  })

  const replyMutation = useMutation({
    mutationFn: ({ sessionID, runID, input, expectedVersion, key }: { sessionID: string, runID: string, input: string, expectedVersion: number, key: string }) => replyAskRun(sessionID, runID, { input, expected_version: expectedVersion }, key),
    onSuccess: restoreSnapshot,
    onError(error) {
      dispatch({ type: 'request.failed', phase: errorPhase(error), message: errorMessage(error) })
    },
  })

  useEffect(() => {
    if (!activeSessionId || !activeRunId) {
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
            disposed = true
            stream?.close()
          }
        },
        onError(error) {
          if (disposed) {
            return
          }
          if (!error.retryable) {
            disposed = true
            dispatch({ type: 'request.failed', phase: 'failed', message: error.message })
            return
          }
          reconnect()
        },
        onClose() {
          if (disposed) {
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
        connect()
      }
      catch (error) {
        recovering = false
        if (!disposed) {
          dispatch({ type: 'request.failed', phase: errorPhase(error), message: errorMessage(error) })
          reconnect()
        }
      }
    }
    async function initialize() {
      try {
        const snapshot = await getAskSnapshot(sessionID)
        if (disposed) {
          return
        }
        restoreFullSnapshot(snapshot)
        connect()
      }
      catch {
        if (!disposed) {
          connect()
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
  }, [activeRunId, activeRunVersion, activeSessionId, restoreFullSnapshot, syncEvents])

  async function submit(input = draft) {
    const value = input.trim()
    const current = pendingRequest.current
    const pending = current?.kind === 'create' && current.input === value ? current : { kind: 'create' as const, input: value, idempotencyKey: idempotencyKey() }
    const reused = pending === current
    pendingRequest.current = pending
    if (!reused) {
      dispatch({ type: 'local.submitted', input: value, clientRunID: localRunID() })
    }
    try {
      const created = await createMutation.mutateAsync({ input: value, key: pending.idempotencyKey })
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

  async function reply(input = draft) {
    if (!activeSessionId || !activeRunId || !runtime.run) {
      return
    }
    const value = input.trim()
    const expectedVersion = runtime.run.row_version
    const current = pendingRequest.current
    const pending = current?.kind === 'reply' && current.input === value && current.sessionID === activeSessionId && current.runID === activeRunId && current.expectedVersion === expectedVersion ? current : { kind: 'reply' as const, input: value, idempotencyKey: idempotencyKey(), sessionID: activeSessionId, runID: activeRunId, expectedVersion }
    const reused = pending === current
    pendingRequest.current = pending
    if (!reused) {
      dispatch({ type: 'local.replied', input: value, clientRunID: localRunID(), runID: activeRunId })
    }
    try {
      const queued = await replyMutation.mutateAsync({ sessionID: activeSessionId, runID: activeRunId, input: value, expectedVersion, key: pending.idempotencyKey })
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

  async function retryProcess() {
    if (!activeSessionId || !activeRunId) {
      return
    }
    return processMutation.mutateAsync({ sessionID: activeSessionId, runID: activeRunId })
  }

  function reset() {
    pendingRequest.current = null
    createMutation.reset()
    processMutation.reset()
    replyMutation.reset()
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
    retryProcess,
    reset,
  }
}
