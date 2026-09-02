import type { Identity, MeResponseDTO, User } from '../types/auth'
import type { UpdateProfileRequest } from '../types/request'
import { useAuthStore } from '../stores/auth-store'
import { request, uploadFile } from './request'

export interface UserProfileResponse {
  user: {
    id: string
    nickname: string
    avatar: string
    profile_completed: boolean
  }
  family: MeResponseDTO['family']
  identity: MeResponseDTO['identity']
}

function applyProfile(value: UserProfileResponse) {
  const user: User = {
    id: value.user.id,
    nickname: value.user.nickname,
    avatarUrl: value.user.avatar || undefined,
  }
  useAuthStore.getState().setUserProfile(user, value.family, value.identity as Identity)
  return value
}

export async function getMe() {
  const response = await request<UserProfileResponse>({
    path: '/api/v1/users/me',
  })
  return applyProfile(response)
}

export async function updateProfile(data: UpdateProfileRequest) {
  const response = await request<UserProfileResponse>({
    path: '/api/v1/users/me',
    method: 'PATCH',
    data,
  })
  return applyProfile(response)
}

export function uploadAvatar(filePath: string) {
  return uploadFile<{ asset_id: string }>({
    path: '/api/v1/assets/upload',
    filePath,
    name: 'file',
    formData: {
      type: 'avatar',
    },
  })
}
