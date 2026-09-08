package ask

import (
	"encoding/json"
	"time"
)

type SessionDTO struct {
	ID          string          `json:"id"`
	PetID       string          `json:"pet_id"`
	Pets        []SessionPetDTO `json:"pets,omitempty"`
	Status      SessionStatus   `json:"status"`
	RiskLevel   RiskLevel       `json:"risk_level"`
	TurnCount   int             `json:"turn_count"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at"`
}

type SessionPetDTO struct {
	PetID     string `json:"pet_id"`
	PetName   string `json:"pet_name"`
	Mention   string `json:"mention"`
	SortOrder int    `json:"sort_order"`
}

type RunDTO struct {
	ID          string     `json:"id"`
	SessionID   string     `json:"session_id"`
	TurnID      string     `json:"turn_id"`
	Status      RunStatus  `json:"status"`
	RiskLevel   RiskLevel  `json:"risk_level"`
	ErrorCode   string     `json:"error_code"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type EventDTO struct {
	Sequence  int             `json:"sequence"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

type ExecutionDTO struct {
	Session SessionDTO `json:"session"`
	Run     RunDTO     `json:"run"`
	Events  []EventDTO `json:"events"`
}

func NewSessionDTO(value Session) SessionDTO {
	pets := make([]SessionPetDTO, 0, len(value.Pets))
	for _, pet := range value.Pets {
		pets = append(pets, SessionPetDTO{PetID: pet.PetID, PetName: pet.PetName, Mention: pet.Mention, SortOrder: pet.SortOrder})
	}
	return SessionDTO{ID: value.ID, PetID: value.PetID, Pets: pets, Status: value.Status, RiskLevel: value.RiskLevel, TurnCount: value.TurnCount, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, CompletedAt: value.CompletedAt}
}

func NewRunDTO(value Run) RunDTO {
	return RunDTO{ID: value.ID, SessionID: value.SessionID, TurnID: value.TurnID, Status: value.Status, RiskLevel: value.RiskLevel, ErrorCode: value.ErrorCode, CreatedAt: value.CreatedAt, StartedAt: value.StartedAt, CompletedAt: value.CompletedAt}
}

func NewEventDTO(value Event) EventDTO {
	data := json.RawMessage(value.Data)
	if !json.Valid(data) {
		data = json.RawMessage(`{}`)
	}
	return EventDTO{Sequence: value.Sequence, Type: value.Type, Data: data, CreatedAt: value.CreatedAt}
}

func NewEventDTOs(values []Event) []EventDTO {
	result := make([]EventDTO, 0, len(values))
	for _, value := range values {
		result = append(result, NewEventDTO(value))
	}
	return result
}

func NewExecutionDTO(value ExecutionResult) ExecutionDTO {
	return ExecutionDTO{Session: NewSessionDTO(value.Session), Run: NewRunDTO(value.Run), Events: NewEventDTOs(value.Events)}
}
