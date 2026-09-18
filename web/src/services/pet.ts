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

/* 新建档案资源条目 (如 birthday-records): 后端为通用 POST /pets/:id/:resource */
export function createPetResource(petID: string, resource: string, data: Record<string, unknown>) {
  return request<Record<string, unknown>>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/${resource}`,
    method: 'POST',
    data,
  })
}
