import type { CreateFamilyRequest, FamilyDetail, FamilyMember, JoinApplication, JoinFamilyRequest } from '../types/family'
import { request } from './request'

export function createFamily(data: CreateFamilyRequest) {
  return request<FamilyDetail>({
    path: '/api/v1/families',
    method: 'POST',
    data,
  })
}

export function getCurrentFamily() {
  return request<FamilyDetail>({
    path: '/api/v1/families/current',
  })
}

export function applyJoinFamily(data: JoinFamilyRequest) {
  return request<JoinApplication>({
    path: '/api/v1/families/join-applications',
    method: 'POST',
    data,
  })
}

export function getMyJoinApplication() {
  return request<JoinApplication | null>({
    path: '/api/v1/families/join-applications/me',
  })
}

export function getMembers() {
  return request<FamilyMember[]>({
    path: '/api/v1/families/members',
  })
}

export function getPendingApplications() {
  return request<JoinApplication[]>({
    path: '/api/v1/families/join-applications',
  })
}

export function approveApplication(memberID: string) {
  return request<void>({
    path: `/api/v1/families/join-applications/${encodeURIComponent(memberID)}/approve`,
    method: 'POST',
  })
}

export function rejectApplication(memberID: string) {
  return request<void>({
    path: `/api/v1/families/join-applications/${encodeURIComponent(memberID)}/reject`,
    method: 'POST',
  })
}

export function removeMember(memberID: string) {
  return request<void>({
    path: `/api/v1/families/members/${encodeURIComponent(memberID)}/remove`,
    method: 'POST',
  })
}
