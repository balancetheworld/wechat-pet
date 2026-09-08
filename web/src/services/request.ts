import type { ApiResponse } from '../types/api'
import type { RequestOptions, UploadOptions } from '../types/request'
import Taro from '@tarojs/taro'
import { routes } from '../constants/routes'
import { useAuthStore } from '../stores/auth-store'
import { ApiError } from '../types/api'

export { ApiError } from '../types/api'
export type { UploadOptions } from '../types/request'

const apiBaseURL = TARO_APP_API_BASE_URL
const requestTimeout = 10_000
let refreshingSession: Promise<void> | null = null

if (!apiBaseURL) {
  throw new Error('缺少 TARO_APP_API_BASE_URL 配置')
}

function requestURL(path: string) {
  return `${apiBaseURL}${path}`
}

export function assetURL(assetID: string) {
  const value = assetID.trim()
  return value ? authorizedAssetURL(requestURL(`/api/v1/uploads/${encodeURIComponent(value)}`)) : ''
}

export function authorizedAssetURL(value: string) {
  if (!value || !value.startsWith(`${apiBaseURL}/api/v1/uploads/`)) {
    return value
  }
  const token = useAuthStore.getState().token
  return token ? `${value}${value.includes('?') ? '&' : '?'}access_token=${encodeURIComponent(token)}` : value
}

function errorFromNetwork(error: unknown) {
  const message = error instanceof Error ? error.message : String(error || '网络请求失败')
  const kind = /timeout|超时/i.test(message) ? 'timeout' : 'network'
  return new ApiError(message, { kind })
}

function isAuthFailure(response: ApiResponse<unknown> | undefined, statusCode: number) {
  return statusCode === 401 || response?.code === 40101 || response?.code === 40102
}

async function handleAuthFailure() {
  await useAuthStore.getState().clearSession()
  await Taro.showToast({
    title: '登录已失效',
    icon: 'none',
  })
  await Taro.reLaunch({ url: routes.pages.home })
}

async function refreshSession() {
  if (!refreshingSession) {
    refreshingSession = (async () => {
      const { silentLogin } = await import('./auth')
      await silentLogin()
    })()
  }

  try {
    await refreshingSession
  }
  finally {
    refreshingSession = null
  }
}

async function requestWithRetry<T>(options: RequestOptions, retried: boolean): Promise<T> {
  const token = useAuthStore.getState().token
  let response: Taro.request.SuccessCallbackResult<ApiResponse<T>>

  try {
    response = await Taro.request<ApiResponse<T>>({
      url: requestURL(options.path),
      method: options.method ?? 'GET',
      data: options.data,
      timeout: requestTimeout,
      header: {
        ...options.header,
        ...(token && !options.skipAuthHeader ? { Authorization: `Bearer ${token}` } : {}),
      },
    })
  }
  catch (error) {
    throw errorFromNetwork(error)
  }

  if (isAuthFailure(response.data, response.statusCode)) {
    if (!retried && !options.skipAuthRefresh) {
      try {
        await refreshSession()
        return requestWithRetry(options, true)
      }
      catch {
        await handleAuthFailure()
      }
    }

    throw new ApiError('登录已失效', {
      kind: 'auth',
      statusCode: response.statusCode,
      code: response.data?.code,
      requestId: response.data?.request_id,
    })
  }

  if (response.statusCode < 200 || response.statusCode >= 300) {
    throw new ApiError(`请求失败，HTTP 状态码：${response.statusCode}`, {
      kind: 'http',
      statusCode: response.statusCode,
      requestId: response.data?.request_id,
    })
  }

  if (response.data.code !== 0) {
    throw new ApiError(response.data.msg || '请求失败', {
      kind: 'business',
      statusCode: response.statusCode,
      code: response.data.code,
      requestId: response.data.request_id,
    })
  }

  return response.data.data
}

export function request<T>(options: RequestOptions) {
  return requestWithRetry<T>(options, false)
}

function parseUploadResponse<T>(data: string, statusCode: number) {
  let response: ApiResponse<T>

  try {
    response = JSON.parse(data) as ApiResponse<T>
  }
  catch {
    throw new ApiError('上传响应格式错误', { kind: 'network', statusCode })
  }

  if (response.code !== 0) {
    throw new ApiError(response.msg || '上传失败', {
      kind: 'business',
      statusCode,
      code: response.code,
      requestId: response.request_id,
    })
  }

  return response.data
}

async function uploadFileWithRetry<T>(options: UploadOptions, retried: boolean): Promise<T> {
  const token = useAuthStore.getState().token
  let response: Taro.uploadFile.SuccessCallbackResult

  try {
    response = await Taro.uploadFile({
      url: requestURL(options.path),
      filePath: options.filePath,
      name: options.name,
      formData: options.formData,
      timeout: requestTimeout,
      header: token ? { Authorization: `Bearer ${token}` } : {},
    })
  }
  catch (error) {
    throw errorFromNetwork(error)
  }

  let parsed: ApiResponse<T> | undefined
  try {
    parsed = JSON.parse(response.data) as ApiResponse<T>
  }
  catch {
    throw new ApiError('上传响应格式错误', { kind: 'network', statusCode: response.statusCode })
  }

  if (isAuthFailure(parsed, response.statusCode) && !retried) {
    try {
      await refreshSession()
      return uploadFileWithRetry(options, true)
    }
    catch {
      await handleAuthFailure()
    }
  }

  if (isAuthFailure(parsed, response.statusCode)) {
    throw new ApiError('登录已失效', {
      kind: 'auth',
      statusCode: response.statusCode,
      code: parsed.code,
      requestId: parsed.request_id,
    })
  }

  if (response.statusCode < 200 || response.statusCode >= 300) {
    throw new ApiError(`上传失败，HTTP 状态码：${response.statusCode}`, {
      kind: 'http',
      statusCode: response.statusCode,
      requestId: parsed.request_id,
    })
  }

  return parseUploadResponse<T>(response.data, response.statusCode)
}

export function uploadFile<T>(options: UploadOptions) {
  return uploadFileWithRetry<T>(options, false)
}
