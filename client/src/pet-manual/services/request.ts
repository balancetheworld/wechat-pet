/**
 * Taro HTTP 请求封装
 *
 * 对应接口文档：
 * - 1.1 请求协议：JSON Content-Type，Authorization header
 * - 1.2 统一响应结构：{ code, msg, data, request_id }
 * - 2.2 小程序使用方式：40101/40102 时静默重新登录并重放一次
 * - 1.5 通用错误响应：前端按 code 显示引导
 */

import Taro from '@tarojs/taro'
import { API_BASE_URL, API_ENABLED, TOKEN_KEY, REQUEST_TIMEOUT } from './config'
import type { ApiResponse } from './types'

/** 获取 token */
export function getToken(): string | null {
  try {
    return Taro.getStorageSync(TOKEN_KEY) || null
  } catch {
    return null
  }
}

/** 存储 token */
export function setToken(token: string): void {
  try {
    Taro.setStorageSync(TOKEN_KEY, token)
  } catch {
    /* storage 不可用时静默 */
  }
}

/** 清除 token */
export function clearToken(): void {
  try {
    Taro.removeStorageSync(TOKEN_KEY)
  } catch {
    /* */
  }
}

/** 业务错误（code !== 0） */
export class ApiError extends Error {
  code: number
  requestId?: string

  constructor(code: number, msg: string, requestId?: string) {
    super(msg)
    this.name = 'ApiError'
    this.code = code
    this.requestId = requestId
  }

  /** 业务错误消息（等同于 message） */
  get msg(): string {
    return this.message
  }
}

/** 认证类错误，需重新登录 */
export function isAuthError(code: number): boolean {
  return code === 40101 || code === 40102
}

/** 家庭权限错误（guest 无家庭 / 无权限） */
export function isFamilyError(code: number): boolean {
  return code === 40301 || code === 40302 || code === 40303 || code === 40304
}

/** 是否需要静默重试（40101/40102） */
let _silentReloginInProgress = false

/**
 * 静默重新登录（微信 code → /auth/login）。
 * 一期前端先用 mock token；后端就绪后取消注释对接真实登录。
 */
async function silentRelogin(): Promise<string | null> {
  if (_silentReloginInProgress) return null
  _silentReloginInProgress = true
  try {
    // TODO: 后端就绪后启用真实微信登录
    // const { code: wxCode } = await Taro.login()
    // const res = await rawRequest<ApiLoginResult>('POST', '/auth/login', { code: wxCode })
    // if (res.code === 0 && res.data?.token) {
    //   setToken(res.data.token)
    //   return res.data.token
    // }
    return null
  } catch {
    return null
  } finally {
    _silentReloginInProgress = false
  }
}

/**
 * 底层请求（不处理静默重试，供 silentRelogin 调用避免循环）
 */
async function rawRequest<T>(
  method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE',
  path: string,
  data?: Record<string, unknown>,
  header?: Record<string, string>
): Promise<ApiResponse<T>> {
  const url = `${API_BASE_URL}${path}`
  const token = getToken()

  const finalHeader: Record<string, string> = {
    'Content-Type': 'application/json',
    ...header,
  }
  if (token) {
    finalHeader['Authorization'] = `Bearer ${token}`
  }

  try {
    const res = await Taro.request({
      url,
      method,
      data: data || {},
      header: finalHeader,
      timeout: REQUEST_TIMEOUT,
    })

    // HTTP 层面失败
    if (res.statusCode < 200 || res.statusCode >= 300) {
      // 尝试读取业务响应体
      const body = res.data as ApiResponse<T> | undefined
      if (body && typeof body.code === 'number') {
        throw new ApiError(body.code, body.msg || '请求失败', body.request_id)
      }
      throw new ApiError(res.statusCode, `网络错误 ${res.statusCode}`)
    }

    const body = res.data as ApiResponse<T>
    return body
  } catch (err) {
    // Taro.request 网络异常
    if (err instanceof ApiError) throw err
    throw new ApiError(500, (err as Error)?.message || '网络请求失败')
  }
}

/**
 * 统一请求入口
 *
 * 流程：
 * 1. 发送请求
 * 2. 若 code === 0 → 返回 data
 * 3. 若 40101/40102 → 静默重新登录 → 重放一次
 * 4. 重放仍失败 → 清 token，抛 ApiError
 * 5. 其他 code → 抛 ApiError（由调用方处理）
 *
 * @param method HTTP 方法
 * @param path   接口路径（不含 /api/v1 前缀），如 '/pets'
 * @param data   请求体（GET 时作为 query params）
 * @param opts   额外 header / Idempotency-Key
 */
export async function request<T = unknown>(
  method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE',
  path: string,
  data?: Record<string, unknown>,
  opts?: {
    header?: Record<string, string>
    idempotencyKey?: string
  }
): Promise<T> {
  const header: Record<string, string> = { ...(opts?.header || {}) }
  if (opts?.idempotencyKey) {
    header['Idempotency-Key'] = opts.idempotencyKey
  }

  // 后端未启用：直接走本地回退，不发起网络请求
  // （小程序端未配置 request 合法域名时，发起请求会被微信拦截并报错）
  if (!API_ENABLED) {
    throw new ApiError(0, '后端未启用，使用本地数据')
  }

  // GET 请求把 data 作为 query params
  let finalPath = path
  let finalData: Record<string, unknown> | undefined = data
  if (method === 'GET' && data) {
    const params = new URLSearchParams()
    for (const [k, v] of Object.entries(data)) {
      if (v !== undefined && v !== null) params.append(k, String(v))
    }
    const qs = params.toString()
    if (qs) finalPath += (path.includes('?') ? '&' : '?') + qs
    finalData = undefined
  }

  let resp = await rawRequest<T>(method, finalPath, finalData, header)

  // 静默重试
  if (isAuthError(resp.code)) {
    const newToken = await silentRelogin()
    if (newToken) {
      // 重放一次
      const retryHeader = { ...header, Authorization: `Bearer ${newToken}` }
      resp = await rawRequest<T>(method, finalPath, finalData, retryHeader)
    }
  }

  // 最终仍为认证错误 → 清 token
  if (isAuthError(resp.code)) {
    clearToken()
  }

  if (resp.code !== 0) {
    throw new ApiError(resp.code, resp.msg || '请求失败', resp.request_id)
  }

  return resp.data as T
}
