/**
 * 资产（文件）服务 —— 对接文件三后端 POST /api/v1/assets/upload
 *
 * 后端契约（server/internal/httpapi/asset/handler.go）：
 * - multipart 表单：file（文件）+ type（avatar|pet_avatar|pet_cover|
 *   birthday_photo|birthday_video|growth_image）
 * - 响应 data：{ asset_id }（asset_id 本身含扩展名，如 ab12….jpg）
 * - 本地存储模式下文件经 router.Static("/uploads", dir) 对外提供访问，
 *   资产 URL = 服务来源 + /uploads/{asset_id}
 */

import Taro from '@tarojs/taro'
import { API_BASE_URL, API_ENABLED, TOKEN_KEY } from './config'
import { ApiError } from './request'
import type { ApiResponse, ApiUploadResult } from './types'

/** 服务来源（API_BASE_URL 形如 http://127.0.0.1:8080/api/v1 → http://127.0.0.1:8080） */
export const SERVER_ORIGIN = API_BASE_URL.replace(/\/api\/v1\/?$/, '')

/** 支持的上传业务类型（与后端 supportedType 一致） */
export type AssetUploadType =
  | 'avatar'
  | 'pet_avatar'
  | 'pet_cover'
  | 'birthday_photo'
  | 'birthday_video'
  | 'growth_image'

/** 由 asset_id 拼出可展示的访问地址（本地存储模式） */
export function buildAssetUrl(assetId?: string | null): string | undefined {
  if (!assetId) return undefined
  return `${SERVER_ORIGIN}/uploads/${assetId}`
}

/**
 * 上传本地临时文件，返回 asset_id
 * 失败时抛 ApiError（调用方自行决定是否降级）
 */
export async function uploadAsset(filePath: string, type: AssetUploadType): Promise<string> {
  if (!API_ENABLED) {
    throw new ApiError(0, '后端未启用，跳过上传')
  }

  let token: string | null = null
  try {
    token = Taro.getStorageSync(TOKEN_KEY) || null
  } catch {
    token = null
  }

  const res = await Taro.uploadFile({
    url: `${API_BASE_URL}/assets/upload`,
    filePath,
    name: 'file',
    formData: { type },
    header: token ? { Authorization: `Bearer ${token}` } : {},
  })

  let body: ApiResponse<ApiUploadResult>
  try {
    body = JSON.parse(res.data) as ApiResponse<ApiUploadResult>
  } catch {
    throw new ApiError(res.statusCode, '上传响应解析失败')
  }

  if (res.statusCode < 200 || res.statusCode >= 300 || body.code !== 0 || !body.data) {
    throw new ApiError(body.code ?? res.statusCode, body.msg || '上传失败')
  }
  return body.data.asset_id
}
