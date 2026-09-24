import type { AskEvent, AskSnapshot } from '../types/ask'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../types/api'
import { useAskSession } from './use-ask-session'

const mocks = vi.hoisted(() => ({
  actions: [] as Array<{ type: string, phase?: string, message?: string }>,
  effects: [] as Array<() => void | (() => void)>,
  getAskSnapshot: vi.fn(),
  createAskSession: vi.fn(),
  createAskSessionForPet: vi.fn(),
  mutations: [] as Array<{ mutationFn: (args: { input: string, key: string }) => Promise<unknown> }>,
  getPets: vi.fn(() => Promise.resolve([])),
  openAskEventStream: vi.fn(),
  askState: {
    draft: '',
    activeSessionId: 'session-1' as string | null,
    activeRunId: 'run-1' as string | null,
    activeRunVersion: 1,
    lastEventSequence: 0,
  },
}))

vi.mock('react', () => ({
  useCallback: <T>(callback: T) => callback,
  useEffect: (effect: () => void | (() => void)) => {
    mocks.effects.push(effect)
    effect()
  },
  useReducer: <S, A>(reducer: (state: S, action: A) => S, initialState: S, initializer: (state: S) => S) => {
    let state = initializer(initialState)
    return [state, (action: A) => {
      state = reducer(state, action)
      mocks.actions.push(action as { type: string, phase?: string, message?: string })
    }]
  },
  useRef: <T>(value: T) => ({ current: value }),
  useState: <T>(value: T) => [value, vi.fn()],
}))

vi.mock('@tanstack/react-query', () => ({
  useMutation: (options: { mutationFn: (args: { input: string, key: string }) => Promise<unknown> }) => {
    mocks.mutations.push(options)
    return { mutateAsync: vi.fn(), reset: vi.fn() }
  },
  useQueryClient: () => ({ setQueryData: vi.fn() }),
}))

vi.mock('@tarojs/taro', () => ({
  default: { useDidShow: vi.fn(), useDidHide: vi.fn() },
}))

vi.mock('../services/ask', () => ({
  continueAskSession: vi.fn(),
  createAskSession: mocks.createAskSession,
  createAskSessionForPet: mocks.createAskSessionForPet,
  getAskSnapshot: mocks.getAskSnapshot,
  replyAskRun: vi.fn(),
}))

vi.mock('../services/ask-stream', () => ({
  openAskEventStream: mocks.openAskEventStream,
}))

vi.mock('../services/pet', () => ({ getPets: mocks.getPets }))

vi.mock('../services/request', async () => {
  const { ApiError } = await vi.importActual<typeof import('../types/api')>('../types/api')
  return { ApiError }
})

vi.mock('../stores/ask-store', () => {
  const useAskStore = Object.assign(
    (selector: (state: typeof mocks.askState & { setDraft: () => void, reset: () => void }) => unknown) => selector({ ...mocks.askState, setDraft: vi.fn(), reset: vi.fn() }),
    {
      getState: () => ({
        ...mocks.askState,
        setActiveExecution: vi.fn(),
        setLastEventSequence: vi.fn((sequence: number) => {
          mocks.askState.lastEventSequence = sequence
        }),
        setDraft: vi.fn(),
        reset: vi.fn(),
      }),
    },
  )
  return { useAskStore }
})

vi.mock('../stores/auth-store', () => ({
  useAuthStore: (selector: (state: { token: string }) => unknown) => selector({ token: 'token' }),
}))

vi.mock('../stores/pet-store', () => ({
  usePetStore: { getState: () => ({ pets: [], selectionMode: 'all', currentPetId: null, setPets: vi.fn() }) },
}))

interface StreamCallbacks {
  onEvents: (events: AskEvent[]) => void
  onError: (error: Error & { retryable: boolean, statusCode?: number }) => void
  onClose: () => void
}

function runningSnapshot(): AskSnapshot {
  const createdAt = '2026-09-19T00:00:00Z'
  return {
    session: { id: 'session-1', pet_id: 'pet-1', status: 'active', risk_level: 'green', turn_count: 1, created_at: createdAt, updated_at: createdAt, completed_at: null },
    pets: [],
    turns: [{
      turn: { id: 'turn-1', session_id: 'session-1', turn_index: 1, status: 'attached', input: '问题', selected_run_id: 'run-1', created_at: createdAt },
      run: { id: 'run-1', session_id: 'session-1', turn_id: 'turn-1', run_index: 1, row_version: 1, clarification_count: 0, attempt_count: 0, status: 'running', risk_level: 'green', error_code: '', created_at: createdAt, started_at: createdAt, completed_at: null, next_attempt_at: null },
      events: [],
      messages: [],
      runs: [],
    }],
    event_cursors: [],
  }
}

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

describe('useAskSession 401 重连控制', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('TARO_APP_DEBUG', true)
    mocks.actions.length = 0
    mocks.effects.length = 0
    mocks.mutations.length = 0
    mocks.createAskSession.mockReset()
    mocks.createAskSessionForPet.mockReset()
    mocks.askState.activeSessionId = 'session-1'
    mocks.askState.activeRunId = 'run-1'
    mocks.askState.lastEventSequence = 0
    mocks.getAskSnapshot.mockReset()
    mocks.openAskEventStream.mockReset()
    mocks.openAskEventStream.mockReturnValue({ close: vi.fn() })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('恢复快照返回 401 后进入失败态且不再连接', async () => {
    mocks.getAskSnapshot
      .mockResolvedValueOnce(runningSnapshot())
      .mockRejectedValueOnce(new ApiError('登录已失效', { kind: 'auth', statusCode: 401 }))

    useAskSession()
    await flushPromises()
    expect(mocks.openAskEventStream).toHaveBeenCalledTimes(1)

    const callbacks = mocks.openAskEventStream.mock.calls[0][0] as StreamCallbacks
    callbacks.onEvents([{ run_id: 'run-1', sequence: 1, type: 'run.completed', data: {}, created_at: '2026-09-19T00:00:01Z' }])
    await flushPromises()
    vi.advanceTimersByTime(5000)

    expect(mocks.actions).toContainEqual({ type: 'request.failed', phase: 'failed', message: '登录已失效；类型：auth；HTTP：401' })
    expect(mocks.openAskEventStream).toHaveBeenCalledTimes(1)
  })

  it('事件流返回 401 后进入失败态且不安排重连', async () => {
    mocks.getAskSnapshot.mockResolvedValueOnce(runningSnapshot())

    useAskSession()
    await flushPromises()
    const callbacks = mocks.openAskEventStream.mock.calls[0][0] as StreamCallbacks
    const error = Object.assign(new Error('登录已失效'), { retryable: true, statusCode: 401 })
    callbacks.onError(error)
    vi.advanceTimersByTime(5000)

    expect(mocks.actions).toContainEqual({ type: 'request.failed', phase: 'failed', message: '登录已失效' })
    expect(mocks.openAskEventStream).toHaveBeenCalledTimes(1)
  })

  it('新会话不使用宠物专属接口', async () => {
    mocks.getAskSnapshot.mockResolvedValueOnce(runningSnapshot())
    useAskSession()
    await mocks.mutations[0].mutationFn({ input: '你好', key: 'key-1' })

    expect(mocks.createAskSession).toHaveBeenCalledWith({ input: '你好', asset_refs: undefined }, 'key-1')
    expect(mocks.createAskSessionForPet).not.toHaveBeenCalled()
  })
})
