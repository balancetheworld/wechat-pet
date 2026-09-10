import type { AskEvent, AskExecution, AskSnapshot } from '../types/ask'
import { expect, it } from 'vitest'
import { askReducer, hasSequenceGap, initialAskRuntimeState } from './ask-reducer'

function event(sequence: number, type: string): AskEvent {
  return {
    run_id: 'run-1',
    sequence,
    type,
    data: {},
    created_at: '2026-09-09T10:00:00Z',
  }
}

function execution(events: AskEvent[]): AskExecution {
  return {
    session: {
      id: 'session-1',
      pet_id: 'pet-1',
      pets: [{ pet_id: 'pet-1', pet_name: '旺仔', mention: '旺仔', sort_order: 0 }],
      status: 'active',
      risk_level: 'unknown',
      turn_count: 1,
      created_at: '2026-09-09T10:00:00Z',
      updated_at: '2026-09-09T10:00:00Z',
      completed_at: null,
    },
    run: {
      id: 'run-1',
      session_id: 'session-1',
      turn_id: 'turn-1',
      run_index: 0,
      row_version: 1,
      clarification_count: 0,
      attempt_count: 0,
      status: 'queued',
      risk_level: 'unknown',
      error_code: '',
      created_at: '2026-09-09T10:00:00Z',
      started_at: null,
      completed_at: null,
      next_attempt_at: null,
    },
    events,
  }
}

function snapshot(events: AskEvent[]): AskSnapshot {
  const value = execution(events)
  return {
    session: value.session,
    pets: value.session.pets ?? [],
    turns: [{
      turn: {
        id: 'turn-1',
        session_id: 'session-1',
        turn_index: 0,
        status: value.run.status,
        input: '旺仔怎么了',
        selected_run_id: value.run.id,
        created_at: '2026-09-09T10:00:00Z',
      },
      run: value.run,
      events,
      messages: [{ role: 'user', content: '旺仔怎么了', created_at: '2026-09-09T10:00:00Z' }],
      runs: [{ run: value.run, events, messages: [{ role: 'user', content: '旺仔怎么了', created_at: '2026-09-09T10:00:00Z' }] }],
    }],
    event_cursors: [{ run_id: value.run.id, sequence: events.at(-1)?.sequence ?? 0 }],
  }
}

it('local submit enters creating before server response', () => {
  const state = askReducer(initialAskRuntimeState, { type: 'local.submitted', input: '旺仔怎么了', clientRunID: 'local-1' })
  expect(state.phase).toBe('creating')
  expect(state.turns[0].input).toBe('旺仔怎么了')
  expect(state.turns[0].optimistic).toBe(true)
})

it('snapshot replaces optimistic run and ignores duplicate events', () => {
  const local = askReducer(initialAskRuntimeState, { type: 'local.submitted', input: '旺仔怎么了', clientRunID: 'local-1' })
  const restored = askReducer(local, { type: 'snapshot.restored', execution: execution([event(1, 'run.queued'), event(2, 'run.started')]) })
  const duplicated = askReducer(restored, { type: 'events.received', events: [event(2, 'run.started')] })
  expect(duplicated.phase).toBe('thinking')
  expect(duplicated.turns[0].runID).toBe('run-1')
  expect(duplicated.turns[0].optimistic).toBe(false)
  expect(duplicated.turns[0].events.length).toBe(2)
  expect(duplicated.cursors['run-1']).toBe(2)
})

it('terminal events select the expected runtime phase', () => {
  const cases = [
    ['assistant.question', 'waiting_input'],
    ['fact.completed', 'completed'],
    ['run.completed', 'completed'],
    ['risk.escalated', 'escalated'],
    ['run.failed', 'failed'],
  ] as const
  for (const [type, phase] of cases) {
    const state = askReducer(initialAskRuntimeState, { type: 'events.received', events: [event(3, type)] })
    expect(state.phase).toBe(phase)
  }
})

it('unknown events are retained without changing the phase', () => {
  const state = askReducer(initialAskRuntimeState, { type: 'events.received', events: [event(1, 'context.loaded')] })
  expect(state.phase).toBe('idle')
  expect(state.turns[0].events[0].type).toBe('context.loaded')
})

it('late events are retained without regressing a terminal phase', () => {
  const completed = askReducer(initialAskRuntimeState, { type: 'events.received', events: [event(3, 'run.completed')] })
  const restored = askReducer(completed, { type: 'snapshot.restored', execution: execution([event(1, 'run.queued'), event(2, 'run.started'), event(3, 'run.completed')]) })
  expect(restored.phase).toBe('completed')
  expect(restored.turns[0].events.map(value => value.sequence)).toEqual([1, 2, 3])
  expect(restored.cursors['run-1']).toBe(3)
})

it('same run reply keeps a separate user message and appends new events after it', () => {
  const waitingExecution = execution([event(1, 'run.queued'), event(2, 'run.started'), event(3, 'assistant.question')])
  waitingExecution.run.status = 'waiting_input'
  waitingExecution.run.row_version = 3
  const waiting = askReducer(initialAskRuntimeState, { type: 'snapshot.restored', execution: waitingExecution })
  const replying = askReducer(waiting, { type: 'local.replied', input: '现在呼吸困难', clientRunID: 'message-2', runID: 'run-1' })
  const queuedExecution = execution([event(4, 'run.queued')])
  queuedExecution.run.row_version = 4
  queuedExecution.run.clarification_count = 1
  const queued = askReducer(replying, { type: 'snapshot.restored', execution: queuedExecution })
  expect(queued.turns).toHaveLength(2)
  expect(queued.turns[0].events.map(value => value.sequence)).toEqual([1, 2, 3])
  expect(queued.turns[1].id).toBe('message-2')
  expect(queued.turns[1].runID).toBe('run-1')
  expect(queued.turns[1].input).toBe('现在呼吸困难')
  expect(queued.turns[1].events.map(value => value.sequence)).toEqual([4])
})

it('full snapshot restores server turns and keeps only unmatched optimistic messages', () => {
  const local = askReducer(initialAskRuntimeState, { type: 'local.submitted', input: '旺仔怎么了', clientRunID: 'local-1' })
  const restored = askReducer(local, { type: 'snapshot.loaded', snapshot: snapshot([event(1, 'run.queued')]) })
  expect(restored.turns).toHaveLength(1)
  expect(restored.turns[0].optimistic).toBe(false)
  const replied = askReducer(restored, { type: 'local.replied', input: '现在呼吸困难', clientRunID: 'message-2', runID: 'run-1' })
  const recovered = askReducer(replied, { type: 'snapshot.loaded', snapshot: snapshot([event(1, 'run.queued'), event(2, 'run.started')]) })
  expect(recovered.turns).toHaveLength(2)
  expect(recovered.turns[1].input).toBe('现在呼吸困难')
  expect(recovered.cursors['run-1']).toBe(2)
})

it('detects event sequence gaps before reducer consumption', () => {
  expect(hasSequenceGap([event(4, 'run.started')], 3)).toBe(false)
  expect(hasSequenceGap([event(5, 'run.started')], 3)).toBe(true)
  expect(hasSequenceGap([event(2, 'run.started'), event(4, 'run.completed')], 1)).toBe(true)
})

it('restores persisted same run replies as separate messages', () => {
  const value = snapshot([event(1, 'run.queued'), event(2, 'assistant.question')])
  value.turns[0].events[1].created_at = '2026-09-09T10:01:00Z'
  value.turns[0].messages.push({ role: 'question', content: '什么时候开始的？', created_at: '2026-09-09T10:01:00Z' })
  value.turns[0].messages.push({ role: 'user', content: '今天早上', created_at: '2026-09-09T10:02:00Z' })
  value.turns[0].events.push({ ...event(3, 'run.queued'), created_at: '2026-09-09T10:02:00Z' })
  value.event_cursors[0].sequence = 3
  const restored = askReducer(initialAskRuntimeState, { type: 'snapshot.loaded', snapshot: value })
  expect(restored.turns).toHaveLength(2)
  expect(restored.turns[0].input).toBe('旺仔怎么了')
  expect(restored.turns[0].events.map(item => item.sequence)).toEqual([1, 2])
  expect(restored.turns[1].input).toBe('今天早上')
  expect(restored.turns[1].events.map(item => item.sequence)).toEqual([3])
})
