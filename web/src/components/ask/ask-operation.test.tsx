import type { AskOperation } from '../../types/ask'
import { renderToStaticMarkup } from 'react-dom/server'
import { expect, it, vi } from 'vitest'
import AskOperationCard from './ask-operation'

vi.mock('@tarojs/components', () => ({ Button: 'button', Text: 'span', View: 'div' }))

function operation(status: AskOperation['status']): AskOperation {
  return {
    id: 'op-1',
    session_id: 'session-1',
    run_id: 'run-1',
    status,
    preview: '新增日常记录：2026-09-23 20:00 洗澡',
    target: 'calendar.record.create',
    result: '',
    sync_targets: [],
    version: 1,
    confirmed_at: null,
    expires_at: null,
    created_at: '2026-09-23T12:00:00Z',
    updated_at: '2026-09-23T12:00:00Z',
  }
}

it('待确认的写入预览展示摘要与确认按钮', () => {
  const markup = renderToStaticMarkup(<AskOperationCard operation={operation('pending')} />)
  expect(markup).toContain('待确认写入')
  expect(markup).toContain('2026-09-23 20:00 洗澡')
  expect(markup).toContain('确认并写入')
  expect(markup).toContain('不用了')
})

it('写入成功后展示结果且不再提供确认按钮', () => {
  const markup = renderToStaticMarkup(<AskOperationCard operation={operation('succeeded')} />)
  expect(markup).toContain('已写入')
  expect(markup).not.toContain('确认并写入')
})

it('日历写入把档案同步作为独立选项, 默认勾选服务端建议', () => {
  const markup = renderToStaticMarkup(<AskOperationCard operation={{ ...operation('pending'), sync_targets: ['growth'] }} />)
  expect(markup).toContain('同步到档案（可选）')
  expect(markup).toContain('同步到成长足迹页')
  expect(markup).not.toContain('生日纪念页')
  expect(markup).toContain('ask-operation-sync-option selected')
})

it('非日历写入不展示档案同步选项', () => {
  const markup = renderToStaticMarkup(<AskOperationCard operation={{ ...operation('pending'), target: 'pet.profile.update' }} />)
  expect(markup).not.toContain('同步到档案')
})

it('写入成功后回显档案同步结果', () => {
  const result = JSON.stringify({ record_id: 'record-1', verified: true, synced: ['growth'], sync_failed: [] })
  const markup = renderToStaticMarkup(<AskOperationCard operation={{ ...operation('succeeded'), result }} />)
  expect(markup).toContain('并同步到成长足迹页')
})

it('已放弃的预览不再展示', () => {
  expect(renderToStaticMarkup(<AskOperationCard operation={operation('abandoned')} />)).toBe('')
})
