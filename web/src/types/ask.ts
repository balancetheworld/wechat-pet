export type AskSessionStatus = 'active' | 'completed' | 'escalated' | 'canceled'

export type AskRunStatus = 'queued' | 'running' | 'waiting_input' | 'completed' | 'escalated' | 'failed' | 'canceled' | 'interrupted'

export type AskRiskLevel = 'unknown' | 'green' | 'yellow' | 'red'

export type AskFactType = 'bath' | 'vaccine' | 'deworming' | 'checkup' | 'visit' | 'medication'

export interface AskSessionPet {
  pet_id: string
  pet_name: string
  mention: string
  sort_order: number
}

export interface AskSession {
  id: string
  pet_id: string
  pets?: AskSessionPet[]
  status: AskSessionStatus
  risk_level: AskRiskLevel
  turn_count: number
  created_at: string
  updated_at: string
  completed_at: string | null
}

export interface AskRun {
  id: string
  session_id: string
  turn_id: string
  run_index: number
  row_version: number
  clarification_count: number
  status: AskRunStatus
  risk_level: AskRiskLevel
  error_code: string
  created_at: string
  started_at: string | null
  completed_at: string | null
}

export interface AskFactItem {
  pet_id: string
  pet_name: string
  found: boolean
  occurred_at: string
  content: string
}

export interface AskFactResult {
  fact_type: AskFactType
  items: AskFactItem[]
}

export interface AskQuestionResult {
  question: string
}

export interface AskRiskResult {
  trigger_code: string
  message: string
  action: string
}

export interface AskAnalysisResult {
  current_assessment: string
  observations: string[]
  possible_causes: string[]
  home_actions: string[]
  escalation_conditions: string[]
}

export interface AskFailedResult {
  message: string
}

export type AskEventData = AskFactResult | AskQuestionResult | AskRiskResult | AskAnalysisResult | AskFailedResult | Record<string, unknown>

export interface AskEvent {
  run_id: string
  sequence: number
  type: string
  data: AskEventData
  created_at: string
}

export interface AskExecution {
  session: AskSession
  run: AskRun
  events: AskEvent[]
}

export interface CreateAskSessionRequest {
  input: string
}

export interface ReplyAskRunRequest {
  input: string
  expected_version: number
}
