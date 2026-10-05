import type { AskEvent, AskExecution, AskOperation, AskSession, AskSnapshot, CreateAskSessionRequest, ReplyAskRunRequest } from '../types/ask'
import { request, uploadFile } from './request'

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

export function continueAskSession(sessionID: string, data: CreateAskSessionRequest, idempotencyKey: string) {
  return request<AskExecution>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/turns`,
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

export function uploadAskImage(filePath: string) {
  return uploadFile<{ asset_id: string }>({ path: '/api/v1/assets/upload', filePath, name: 'file', formData: { type: 'ask_image' } })
}

export function stopAskRun(sessionID: string, runID: string, expectedVersion: number) {
  return request<AskExecution>({ path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/runs/${encodeURIComponent(runID)}/stop`, method: 'POST', data: { expected_version: expectedVersion } })
}

export function retryAskRun(sessionID: string, runID: string, expectedVersion: number) {
  return request<AskExecution>({ path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/runs/${encodeURIComponent(runID)}/retry`, method: 'POST', data: { expected_version: expectedVersion } })
}

export function getAskOperations(sessionID: string) {
  return request<AskOperation[]>({ path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/operations` })
}

/* syncTargets: 卡片上用户选择的档案同步目标; 不传表示不做单独选择(沿用服务端建议) */
export function confirmAskOperation(sessionID: string, operationID: string, expectedVersion: number, summary: string, syncTargets?: string[]) {
  return request<AskOperation>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/operations/${encodeURIComponent(operationID)}/confirm`,
    method: 'POST',
    data: { expected_version: expectedVersion, summary, sync_targets: syncTargets },
  })
}

export function abandonAskOperation(sessionID: string, operationID: string, expectedVersion: number) {
  return request<AskOperation>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/operations/${encodeURIComponent(operationID)}/abandon`,
    method: 'POST',
    data: { expected_version: expectedVersion },
  })
}

export function executeAskOperation(sessionID: string, operationID: string, expectedVersion: number) {
  return request<AskOperation>({
    path: `/api/v1/ask/sessions/${encodeURIComponent(sessionID)}/operations/${encodeURIComponent(operationID)}/execute`,
    method: 'POST',
    data: { expected_version: expectedVersion },
  })
}
