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
  attempt_count: number
  status: AskRunStatus
  risk_level: AskRiskLevel
  error_code: string
  created_at: string
  started_at: string | null
  completed_at: string | null
  next_attempt_at: string | null
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

export interface AskFamilyPetItem {
  pet_id: string
  pet_name: string
}

export interface AskFamilyPetsResult {
  pets: AskFamilyPetItem[]
}

export interface AskQuestionResult {
  question: string
}

export interface AskAssistantResult {
  answer: string
  intent: 'casual_chat' | 'pet_fact' | 'unsupported'
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

export interface AskProgressResult {
  stage: 'intent_routing' | 'input_reviewing' | 'context_ready' | 'risk_checking' | 'response_generating'
  message: string
}

export interface AskDeltaResult {
  delta: string
}

export type AskEventData = AskFactResult | AskFamilyPetsResult | AskQuestionResult | AskAssistantResult | AskRiskResult | AskAnalysisResult | AskFailedResult | AskProgressResult | AskDeltaResult | Record<string, unknown>

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

export interface AskSnapshotTurn {
  turn: {
    id: string
    session_id: string
    turn_index: number
    status: AskRunStatus
    input: string
    selected_run_id: string
    created_at: string
  }
  run: AskRun
  events: AskEvent[]
  messages: AskMessage[]
  runs: AskSnapshotRun[]
}

export interface AskSnapshotRun {
  run: AskRun
  events: AskEvent[]
  messages: AskMessage[]
}

export interface AskMessage {
  role: 'user' | 'assistant' | 'question'
  content: string
  created_at: string
}

export interface AskEventCursor {
  run_id: string
  sequence: number
}

export interface AskSnapshot {
  session: AskSession
  pets: AskSessionPet[]
  turns: AskSnapshotTurn[]
  event_cursors: AskEventCursor[]
}

export interface CreateAskSessionRequest {
  input: string
}

export interface ReplyAskRunRequest {
  input: string
  expected_version: number
}
