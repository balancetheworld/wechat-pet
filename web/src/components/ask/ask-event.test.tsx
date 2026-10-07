import type { ReactNode } from 'react'
import type { AskEvent } from '../../types/ask'
import { renderToStaticMarkup } from 'react-dom/server'
import { expect, it, vi } from 'vitest'
import AskEventView from './ask-event'

vi.mock('@tarojs/components', () => ({
  Button: 'button',
  RichText: ({ nodes }: { nodes: string }) => <div data-nodes={nodes} />,
  ScrollView: ({ children, className }: { children?: ReactNode, className?: string }) => <div className={className}>{children}</div>,
  Text: 'span',
  View: 'div',
}))
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

it('shows the pet name instead of the internal id and skips placeholder subject text', () => {
  const event: AskEvent = {
    run_id: 'run-subject',
    sequence: 3,
    type: 'assistant.completed',
    created_at: '2026-10-07T10:00:00Z',
    data: {
      answer: '节哀。',
      intent: 'health',
      groups: [
        {
          group_key: 'g1',
          task_keys: ['t1'],
          answer_kind: 'health',
          scope: 'full',
          subjects: [{ subject_key: 's1', kind: 'pet', pet_id: '8b8c60660767898642f6e5c589783496' }],
          segments: [{ segment_key: 'seg1', group_key: 'g1', subject_keys: ['s1'], field: 'observation', text: '节哀。', basis_kind: 'general_knowledge' }],
          risks: [{ group_key: 'g1', subject_key: 's1', level: 'unknown', evidence: null, uncertainty: null }],
        },
        {
          group_key: 'g2',
          task_keys: ['t1'],
          answer_kind: 'health',
          scope: 'full',
          subjects: [{ subject_key: 's2', kind: 'unresolved' }],
          segments: [{ segment_key: 'seg2', group_key: 'g2', subject_keys: ['s2'], field: 'observation', text: '我在。', basis_kind: 'general_knowledge' }],
          risks: [{ group_key: 'g2', subject_key: 's2', level: 'unknown', evidence: null, uncertainty: null }],
        },
      ],
      coverage: [{ task_key: 't1', answer_group_keys: ['g1', 'g2'], question_keys: [], operation_ids: [] }],
    },
  }

  const markup = renderToStaticMarkup(<AskEventView event={event} petNames={{ '8b8c60660767898642f6e5c589783496': '旺仔' }} />)
  expect(markup).toContain('宠物 旺仔')
  expect(markup).not.toContain('8b8c60660767898642f6e5c589783496')
  expect(markup).not.toContain('>turn<')
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
  expect(markup).toContain('&lt;ol style=&quot;margin:0 0 12px;padding-left:26px;list-style-type:decimal;&quot;&gt;')
  expect(markup).toContain('&lt;strong style=&quot;font-weight:600;&quot;&gt;健康观察&lt;/strong&gt;')
  expect(markup).toContain('&lt;strong style=&quot;font-weight:600;&quot;&gt;记录查询&lt;/strong&gt;')
  expect(markup).not.toContain('&lt;script&gt;')
})

it('hides internal evidence source types from the user answer', () => {
  const event: AskEvent = {
    run_id: 'run-evidence',
    sequence: 7,
    type: 'assistant.completed',
    created_at: '2026-09-23T10:40:50Z',
    data: {
      answer: '啾啾是大狗狗。',
      intent: 'pet_fact',
      groups: [{
        group_key: 'g1',
        task_keys: ['t1'],
        answer_kind: 'casual',
        scope: 'full',
        subjects: [{ subject_key: 's1', kind: 'pet', pet_id: 'pet-1', description: '啾啾' }],
        segments: [{
          segment_key: 'seg1',
          group_key: 'g1',
          subject_keys: ['s1'],
          field: 'reply',
          text: '啾啾是大狗狗。',
          basis_kind: 'business_fact',
          evidence_refs: [{ source_type: 'pet_profile', source_id: 'pet-1' }, { source_type: 'turn', source_id: 'turn-1' }],
        }],
        risks: null,
      }],
      coverage: [],
    },
  }

  const markup = renderToStaticMarkup(<AskEventView event={event} />)
  expect(markup).not.toContain('pet_profile')
  expect(markup).not.toContain('turn')
})

it('renders the completed answer immediately without replaying a preview', () => {
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

  const markup = renderToStaticMarkup(<AskEventView event={completed} events={events} />)
  expect(markup).toContain('你好！')
  expect(markup).toContain('复制回答')
})

it('renders a streamed delta as received', () => {
  const delta: AskEvent = { run_id: 'run-4', sequence: 1, type: 'assistant.delta', created_at: '2026-09-23T10:40:49Z', data: { message_id: 'message-1', delta: '目前需要观察食欲和精神。' } }

  const markup = renderToStaticMarkup(<AskEventView event={delta} />)
  expect(markup).toContain('目前需要观察食欲和精神。')
})

it('renders the thinking preview with its own label and no copy button', () => {
  const thinking: AskEvent = { run_id: 'run-5', sequence: 1, type: 'assistant.thinking', created_at: '2026-09-23T10:40:49Z', data: { message_id: 'message-1', delta: '先看精神状态和饮水' } }

  const markup = renderToStaticMarkup(<AskEventView event={thinking} />)
  expect(markup).toContain('思考过程')
  expect(markup).toContain('先看精神状态和饮水')
  expect(markup).toContain('ask-thinking-scroll')
  expect(markup).not.toContain('复制回答')
})
