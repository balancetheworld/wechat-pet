export interface RequestOptions {
  path: string
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  data?: unknown
  header?: Record<string, string>
  skipAuthHeader?: boolean
  skipAuthRefresh?: boolean
}

export interface UploadOptions {
  path: string
  filePath: string
  name: string
  formData?: Record<string, string>
}

export interface UpdateProfileRequest {
  nickname: string
  avatar_asset_id: string
}
