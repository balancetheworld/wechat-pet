import { expect, it } from 'vitest'
import { askFailureTitle } from './ask-failure'

it('returns a specific title for known ask failures', () => {
  expect(askFailureTitle('provider_timeout')).toBe('AI 服务响应超时')
  expect(askFailureTitle('provider_auth_failed')).toBe('AI 模型访问失败')
  expect(askFailureTitle('provider_quota_exhausted')).toBe('AI 服务额度不可用')
  expect(askFailureTitle('invalid_analysis_output')).toBe('健康建议校验未通过')
  expect(askFailureTitle('executor_failed')).toBe('问问执行器运行失败')
  expect(askFailureTitle('worker_attempts_exhausted')).toBe('AI 服务多次重试失败')
})

it('returns an explicit fallback title for unknown ask failures', () => {
  expect(askFailureTitle('unknown')).toBe('问问服务异常（unknown）')
})
