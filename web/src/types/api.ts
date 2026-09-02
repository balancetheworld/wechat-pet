export interface ApiResponse<T> {
  code: number
  msg: string
  data: T
  request_id: string
}

export type ApiErrorKind = 'network' | 'timeout' | 'http' | 'business' | 'auth'

export class ApiError extends Error {
  readonly kind: ApiErrorKind
  readonly statusCode?: number
  readonly code?: number
  readonly requestId?: string

  constructor(message: string, options: {
    kind: ApiErrorKind
    statusCode?: number
    code?: number
    requestId?: string
  }) {
    super(message)
    this.name = 'ApiError'
    this.kind = options.kind
    this.statusCode = options.statusCode
    this.code = options.code
    this.requestId = options.requestId
  }
}
