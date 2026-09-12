import type { AskEvent, AskExecution, AskSession, AskSnapshot, CreateAskSessionRequest, ReplyAskRunRequest } from '../types/ask'
import { request } from './request'

export function createAskSession(data: CreateAskSessionRequest, idempotencyKey: string) {
  return request<AskExecution>({
    path: '/api/v1/ask/sessions',
    method: 'POST',
    data,
    header: { 'Idempotency-Key': idempotencyKey },
  })
}

export function createAskSessionForPet(petID: string, data: CreateAskSessionRequest, idempotencyKey: string) {
  return request<AskExecution>({
    path: `/api/v1/pets/${encodeURIComponent(petID)}/ask/sessions`,
    method: 'POST',
    data,
    header: { 'Idempotency-Key': idempotencyKey },
  })
}

export function processAskRun(sessionID: string, runID: string) {
  return request<AskExecution>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/runs/${encodeURIComponent(runID)}/process`,
    method: 'POST',
  })
}

export function replyAskRun(sessionID: string, runID: string, data: ReplyAskRunRequest, idempotencyKey: string) {
  return request<AskExecution>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/runs/${encodeURIComponent(runID)}/reply`,
    method: 'POST',
    data,
    header: { 'Idempotency-Key': idempotencyKey },
  })
}

export function getAskSession(sessionID: string) {
  return request<AskSession>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}`,
  })
}

export function getAskSnapshot(sessionID: string) {
  return request<AskSnapshot>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/snapshot`,
  })
}

export function getAskEvents(sessionID: string, runID: string, after = 0) {
  return request<AskEvent[]>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/runs/${encodeURIComponent(runID)}/events?after=${after}`,
  })
}
