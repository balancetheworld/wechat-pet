import type { AskEvent, AskExecution, AskRun, AskSession } from '../types/ask'

export type AskRuntimePhase = 'idle' | 'creating' | 'thinking' | 'reconnecting' | 'waiting_input' | 'replying' | 'completed' | 'escalated' | 'failed' | 'input_error' | 'ambiguous' | 'network_error'

export interface AskRuntimeTurn {
  id: string
  runID: string
  input: string
  optimistic: boolean
  events: AskEvent[]
}

export interface AskRuntimeState {
  phase: AskRuntimePhase
  session: AskSession | null
  run: AskRun | null
  turns: AskRuntimeTurn[]
  cursors: Record<string, number>
  seenEvents: Record<string, boolean>
  error: string
}

export type AskRuntimeAction
  = { type: 'local.submitted', input: string, clientRunID: string }
    | { type: 'local.replied', input: string, clientRunID: string, runID: string }
    | { type: 'snapshot.restored', execution: AskExecution }
    | { type: 'events.received', events: AskEvent[] }
    | { type: 'stream.reconnecting' }
    | { type: 'request.failed', phase: 'failed' | 'input_error' | 'ambiguous' | 'network_error', message: string }
    | { type: 'reset' }

export const initialAskRuntimeState: AskRuntimeState = {
  phase: 'idle',
  session: null,
  run: null,
  turns: [],
  cursors: {},
  seenEvents: {},
  error: '',
}

function phaseForEvent(state: AskRuntimeState, event: AskEvent): AskRuntimePhase {
  switch (event.type) {
    case 'run.queued':
    case 'run.started':
      return 'thinking'
    case 'assistant.question':
      return 'waiting_input'
    case 'fact.completed':
    case 'run.completed':
      return 'completed'
    case 'risk.escalated':
      return 'escalated'
    case 'run.failed':
      return 'failed'
    default:
      return state.phase
  }
}

function phaseForRun(status: AskRun['status'], fallback: AskRuntimePhase): AskRuntimePhase {
  switch (status) {
    case 'queued':
    case 'running':
      return 'thinking'
    case 'waiting_input':
      return 'waiting_input'
    case 'completed':
      return 'completed'
    case 'escalated':
      return 'escalated'
    case 'failed':
    case 'canceled':
    case 'interrupted':
      return 'failed'
    default:
      return fallback
  }
}

function applyEvent(state: AskRuntimeState, event: AskEvent): AskRuntimeState {
  const key = `${event.run_id}:${event.sequence}`
  if (state.seenEvents[key]) {
    return state
  }
  const currentSequence = state.cursors[event.run_id] ?? 0
  let index = -1
  for (let value = state.turns.length - 1; value >= 0; value--) {
    if (state.turns[value].runID === event.run_id) {
      index = value
      break
    }
  }
  const turns = state.turns.slice()
  if (index >= 0) {
    turns[index] = {
      ...turns[index],
      events: [...turns[index].events, event].sort((left, right) => left.sequence - right.sequence),
    }
  }
  else {
    turns.push({ id: event.run_id, runID: event.run_id, input: '', optimistic: false, events: [event] })
  }
  return {
    ...state,
    phase: (!state.run || state.run.id === event.run_id) && event.sequence >= currentSequence ? phaseForEvent(state, event) : state.phase,
    turns,
    cursors: {
      ...state.cursors,
      [event.run_id]: Math.max(state.cursors[event.run_id] ?? 0, event.sequence),
    },
    seenEvents: {
      ...state.seenEvents,
      [key]: true,
    },
  }
}

function restoreSnapshot(state: AskRuntimeState, execution: AskExecution) {
  let turns = state.turns
  const optimisticIndex = turns.findIndex(turn => turn.optimistic)
  if (optimisticIndex >= 0) {
    turns = turns.slice()
    turns[optimisticIndex] = {
      ...turns[optimisticIndex],
      runID: execution.run.id,
      optimistic: false,
    }
  }
  else if (!turns.some(turn => turn.runID === execution.run.id)) {
    turns = [...turns, { id: execution.run.turn_id, runID: execution.run.id, input: '', optimistic: false, events: [] }]
  }
  let value: AskRuntimeState = {
    ...state,
    phase: phaseForRun(execution.run.status, state.phase),
    session: execution.session,
    run: execution.run,
    turns,
    error: '',
  }
  for (const event of execution.events) {
    value = applyEvent(value, event)
  }
  const turn = value.turns.find(turn => turn.runID === execution.run.id)
  const latestEvent = turn?.events.reduce<AskEvent | null>((latest, event) => !latest || event.sequence > latest.sequence ? event : latest, null)
  return latestEvent ? { ...value, phase: phaseForEvent(value, latestEvent) } : value
}

export function askReducer(state: AskRuntimeState, action: AskRuntimeAction): AskRuntimeState {
  switch (action.type) {
    case 'local.submitted':
      return {
        ...initialAskRuntimeState,
        phase: 'creating',
        turns: [{ id: action.clientRunID, runID: action.clientRunID, input: action.input, optimistic: true, events: [] }],
      }
    case 'snapshot.restored':
      return restoreSnapshot(state, action.execution)
    case 'local.replied':
      return {
        ...state,
        phase: 'replying',
        turns: [...state.turns, { id: action.clientRunID, runID: action.runID, input: action.input, optimistic: true, events: [] }],
        error: '',
      }
    case 'events.received': {
      let value = state
      for (const event of action.events.slice().sort((left, right) => left.sequence - right.sequence)) {
        value = applyEvent(value, event)
      }
      return value
    }
    case 'stream.reconnecting':
      if (state.phase === 'thinking' || state.phase === 'creating') {
        return { ...state, phase: 'reconnecting' }
      }
      return state
    case 'request.failed':
      return { ...state, phase: action.phase, error: action.message }
    case 'reset':
      return initialAskRuntimeState
    default:
      return state
  }
}
