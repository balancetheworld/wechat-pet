import type { CreatePetRequest, Pet, PetProfile, UpdatePetRequest } from '../types/pet'
import { request } from './request'

export function getPets() {
  return request<Pet[]>({
    path: '/api/v1/pets',
  })
}

export function createPet(data: CreatePetRequest) {
  return request<Pet>({
    path: '/api/v1/pets',
    method: 'POST',
    data,
  })
}

export function getPet(petID: string) {
  return request<Pet>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}`,
  })
}

export function updatePet(petID: string, data: UpdatePetRequest) {
  return request<Pet>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}`,
    method: 'PATCH',
    data,
  })
}

export function deletePet(petID: string) {
  return request<void>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}`,
    method: 'DELETE',
  })
}

export function getPetProfile(petID: string) {
  return request<PetProfile>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/profile`,
  })
}

/* 更新宠物主表字段 (后端允许 name/avatar_asset_id/cover_asset_id/breed/gender/sterilized/birthday/home_date) */
export function updatePetProfile(petID: string, data: Record<string, unknown>) {
  return request<PetProfile>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/profile`,
    method: 'PATCH',
    data,
  })
}

export function getPetResource<T>(petID: string, resource: string) {
  return request<T>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/${resource}`,
  })
}

/* 更新档案资源条目 (如 growth-events): 后端为通用 PATCH /pets/:id/:resource/:resource_id */
export function updatePetResource(petID: string, resource: string, resourceID: string, data: Record<string, unknown>) {
  return request<unknown>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/${resource}/${encodeURIComponent(resourceID)}`,
    method: 'PATCH',
    data,
  })
}

/* 删除档案资源条目 (如 weights): 后端为通用 DELETE /pets/:id/:resource/:resource_id (硬删除) */
export function deletePetResource(petID: string, resource: string, resourceID: string) {
  return request<unknown>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/${resource}/${encodeURIComponent(resourceID)}`,
    method: 'DELETE',
  })
}

/* 新建档案资源条目 (如 birthday-records): 后端为通用 POST /pets/:id/:resource */
export function createPetResource(petID: string, resource: string, data: Record<string, unknown>) {
  return request<Record<string, unknown>>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/${resource}`,
    method: 'POST',
    data,
  })
}

/* 健康资料四项: 后端 pet_health 表持久化 (GET/PUT /pets/:id/health) */
export function getPetHealth(petID: string) {
  return request<Record<string, string>>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/health`,
  })
}

export function putPetHealth(petID: string, data: Record<string, string>) {
  return request<Record<string, string>>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/health`,
    method: 'PUT',
    data,
  })
}
