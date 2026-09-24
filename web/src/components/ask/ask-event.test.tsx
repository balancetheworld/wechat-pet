import type { AskEvent } from '../../types/ask'
import { renderToStaticMarkup } from 'react-dom/server'
import { expect, it, vi } from 'vitest'
import AskEventView from './ask-event'

vi.mock('@tarojs/components', () => ({ Button: 'button', RichText: ({ nodes }: { nodes: string }) => <div data-nodes={nodes} />, Text: 'span', View: 'div' }))
vi.mock('@tarojs/taro', () => ({ default: {} }))

it('renders a completed casual answer with empty optional evidence and risks', () => {
  const event: AskEvent = {
    run_id: 'run-1',
    sequence: 7,
    type: 'assistant.completed',
    created_at: '2026-09-23T10:40:50Z',
    data: {
      answer: '你好！',
      intent: 'casual_chat',
      groups: [{
        group_key: 'g1',
        task_keys: ['t1'],
        answer_kind: 'casual',
        scope: 'full',
        subjects: [{ subject_key: 's1', kind: 'unresolved', description: '用户' }],
        segments: [{ segment_key: 'seg1', group_key: 'g1', subject_keys: ['s1'], field: 'reply', text: '你好！', basis_kind: 'general_knowledge' }],
        risks: null,
      }],
      coverage: [{ task_key: 't1', answer_group_keys: ['g1'], question_keys: [], operation_ids: [] }],
    },
  }

  const markup = renderToStaticMarkup(<AskEventView event={event} />)
  expect(markup).toContain('你好！')
  expect(markup).toContain('复制回答')
})

it('renders markdown in assistant segments without exposing raw HTML', () => {
  const event: AskEvent = {
    run_id: 'run-2',
    sequence: 7,
    type: 'assistant.completed',
    created_at: '2026-09-23T10:40:50Z',
    data: {
      answer: '1. **健康观察**\n2. **记录查询**',
      intent: 'casual_chat',
      groups: [{
        group_key: 'g1',
        task_keys: ['t1'],
        answer_kind: 'casual',
        scope: 'full',
        subjects: [],
        segments: [{ segment_key: 'seg1', group_key: 'g1', subject_keys: [], field: 'reply', text: '1. **健康观察**\n2. **记录查询**\n\n<script>alert(1)</script>', basis_kind: 'general_knowledge' }],
        risks: null,
      }],
      coverage: [],
    },
  }

  const markup = renderToStaticMarkup(<AskEventView event={event} />)
  expect(markup).toContain('&lt;ol&gt;')
  expect(markup).toContain('&lt;strong&gt;健康观察&lt;/strong&gt;')
  expect(markup).toContain('&lt;strong&gt;记录查询&lt;/strong&gt;')
  expect(markup).not.toContain('&lt;script&gt;')
})

it('keeps playing the streamed preview before switching to the completed answer', () => {
  const completed: AskEvent = {
    run_id: 'run-3',
    sequence: 8,
    type: 'assistant.completed',
    created_at: '2026-09-23T10:40:50Z',
    data: {
      answer: '你好！',
      intent: 'casual_chat',
      groups: [{
        group_key: 'g1',
        task_keys: ['t1'],
        answer_kind: 'casual',
        scope: 'full',
        subjects: [{ subject_key: 's1', kind: 'unresolved', description: '用户' }],
        segments: [{ segment_key: 'seg1', group_key: 'g1', subject_keys: ['s1'], field: 'reply', text: '你好！', basis_kind: 'general_knowledge' }],
        risks: null,
      }],
      coverage: [],
    },
  }
  const preview: AskEvent = { run_id: 'run-3', sequence: 7, type: 'assistant.delta', created_at: '2026-09-23T10:40:49Z', data: { message_id: 'message-1', delta: '你好！' } }
  const events = [preview, completed]

  const live = renderToStaticMarkup(<AskEventView event={completed} events={events} live />)
  expect(live).toContain('复制回答')
  expect(live).not.toContain('你好！')

  const restored = renderToStaticMarkup(<AskEventView event={completed} events={events} />)
  expect(restored).toContain('你好！')
})
