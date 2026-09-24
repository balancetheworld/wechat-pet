import type { AskEventStreamError } from '../services/ask-stream'
import { ApiError } from '../types/api'

export function shouldReconnectAfterSnapshotError(error: unknown) {
  return !(error instanceof ApiError && (error.kind === 'auth' || error.statusCode === 401))
}

export function shouldReconnectAfterStreamError(error: AskEventStreamError) {
  return error.statusCode !== 401 && error.retryable
}
