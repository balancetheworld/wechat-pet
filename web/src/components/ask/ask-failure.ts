const failureTitles: Record<string, string> = {
  agent_output_unparsable: 'AI 返回内容格式异常',
  executor_failed: '问问执行器运行失败',
  invalid_analysis_output: '健康建议校验未通过',
  invalid_event_data: '回答保存失败',
  invalid_executor_status: 'AI 返回状态异常',
  provider_auth_failed: 'AI 模型访问失败',
  provider_canceled: 'AI 请求已中断',
  provider_failed: 'AI 服务调用失败',
  provider_output_invalid: 'AI 返回内容格式异常',
  provider_output_truncated: 'AI 本次回答被截断',
  provider_quota_exhausted: 'AI 服务额度不可用',
  provider_rate_limited: 'AI 服务请求过于频繁',
  provider_request_invalid: 'AI 服务请求配置异常',
  provider_timeout: 'AI 服务响应超时',
  provider_unavailable: 'AI 服务暂时不可用',
  worker_attempts_exhausted: 'AI 服务多次重试失败',
}

export function askFailureTitle(errorCode: string) {
  return failureTitles[errorCode] || `问问服务异常（${errorCode || 'unknown'}）`
}
