export type AskSessionStatus = 'active' | 'closed'

export type AskTurnStatus = 'received' | 'attached' | 'superseded'

export type AskRunStatus = 'queued' | 'running' | 'waiting_input' | 'canceling' | 'completed' | 'failed' | 'canceled' | 'interrupted'

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
  count: number
  pets: AskFamilyPetItem[]
}

export interface AskQuestionResult {
  question: string
}

export interface AskAnswerSubject {
  subject_key: string
  kind: 'pet' | 'unresolved'
  pet_id?: string
  description?: string
  source_turn_ids?: string[]
}

export interface AskAnswerGroup {
  group_key: string
  task_keys: string[]
  answer_kind: 'casual' | 'fact' | 'health'
  subjects: AskAnswerSubject[]
  scope: 'full' | 'limited' | 'declined' | 'unavailable'
  segments: AskAnswerSegment[]
  risks: AskAnswerRisk[] | null
}

export interface AskEvidenceRef {
  source_type: string
  source_id: string
  version?: string
  position?: string
}

export interface AskAnswerSegment {
  segment_key: string
  group_key: string
  subject_keys: string[]
  field: string
  text: string
  basis_kind: 'user_statement' | 'image_observation' | 'business_fact' | 'general_knowledge' | 'speculation'
  evidence_refs?: AskEvidenceRef[] | null
}

export interface AskAnswerRisk {
  group_key: string
  subject_key: string
  level: AskRiskLevel
  evidence?: AskEvidenceRef[] | null
  uncertainty?: string
}

export interface AskTaskCoverage {
  task_key: string
  answer_group_keys: string[]
  question_keys: string[]
  operation_ids: string[]
  incomplete_reason?: string
}

export interface AskAssistantResult {
  answer: string
  groups: AskAnswerGroup[]
  coverage?: AskTaskCoverage[]
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
  error_code: string
  message: string
}

export interface AskProgressResult {
  stage: 'intent_routing' | 'intent_completed' | 'input_reviewing' | 'context_ready' | 'risk_checking' | 'response_generating'
  message: string
}

export interface AskDeltaResult {
  message_id: string
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
    status: AskTurnStatus
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
  asset_refs?: string[]
}

export interface ReplyAskRunRequest {
  input: string
  expected_version: number
  asset_refs?: string[]
}

export type AskOperationStatus = 'pending' | 'confirmed' | 'executing' | 'succeeded' | 'failed' | 'unknown' | 'abandoned' | 'withdrawn' | 'expired'

export interface AskOperation {
  id: string
  session_id: string
  run_id: string
  status: AskOperationStatus
  preview: string
  target: string
  result: string
  version: number
  confirmed_at: string | null
  expires_at: string | null
  created_at: string
  updated_at: string
}
