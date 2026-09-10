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
	ID                 string     `json:"id"`
	SessionID          string     `json:"session_id"`
	TurnID             string     `json:"turn_id"`
	RunIndex           int        `json:"run_index"`
	RowVersion         int        `json:"row_version"`
	ClarificationCount int        `json:"clarification_count"`
	AttemptCount       int        `json:"attempt_count"`
	Status             RunStatus  `json:"status"`
	RiskLevel          RiskLevel  `json:"risk_level"`
	ErrorCode          string     `json:"error_code"`
	CreatedAt          time.Time  `json:"created_at"`
	StartedAt          *time.Time `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	NextAttemptAt      *time.Time `json:"next_attempt_at"`
}

type TurnDTO struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	TurnIndex     int       `json:"turn_index"`
	Status        RunStatus `json:"status"`
	Input         string    `json:"input"`
	SelectedRunID string    `json:"selected_run_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type EventDTO struct {
	RunID     string          `json:"run_id"`
	Sequence  int             `json:"sequence"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

type MessageDTO struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type ExecutionDTO struct {
	Session SessionDTO `json:"session"`
	Run     RunDTO     `json:"run"`
	Events  []EventDTO `json:"events"`
}

type SnapshotTurnDTO struct {
	Turn     TurnDTO          `json:"turn"`
	Run      RunDTO           `json:"run"`
	Events   []EventDTO       `json:"events"`
	Messages []MessageDTO     `json:"messages"`
	Runs     []SnapshotRunDTO `json:"runs"`
}

type SnapshotRunDTO struct {
	Run      RunDTO       `json:"run"`
	Events   []EventDTO   `json:"events"`
	Messages []MessageDTO `json:"messages"`
}

type EventCursorDTO struct {
	RunID    string `json:"run_id"`
	Sequence int    `json:"sequence"`
}

type SnapshotDTO struct {
	Session      SessionDTO        `json:"session"`
	Pets         []SessionPetDTO   `json:"pets"`
	Turns        []SnapshotTurnDTO `json:"turns"`
	EventCursors []EventCursorDTO  `json:"event_cursors"`
}

func NewSessionDTO(value Session) SessionDTO {
	pets := make([]SessionPetDTO, 0, len(value.Pets))
	for _, pet := range value.Pets {
		pets = append(pets, SessionPetDTO{PetID: pet.PetID, PetName: pet.PetName, Mention: pet.Mention, SortOrder: pet.SortOrder})
	}
	return SessionDTO{ID: value.ID, PetID: value.PetID, Pets: pets, Status: value.Status, RiskLevel: value.RiskLevel, TurnCount: value.TurnCount, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt, CompletedAt: value.CompletedAt}
}

func NewRunDTO(value Run) RunDTO {
	return RunDTO{ID: value.ID, SessionID: value.SessionID, TurnID: value.TurnID, RunIndex: value.RunIndex, RowVersion: value.RowVersion, ClarificationCount: value.ClarificationCount, AttemptCount: value.AttemptCount, Status: value.Status, RiskLevel: value.RiskLevel, ErrorCode: value.ErrorCode, CreatedAt: value.CreatedAt, StartedAt: value.StartedAt, CompletedAt: value.CompletedAt, NextAttemptAt: value.NextAttemptAt}
}

func NewTurnDTO(value Turn) TurnDTO {
	return TurnDTO{ID: value.ID, SessionID: value.SessionID, TurnIndex: value.TurnIndex, Status: value.Status, Input: value.Input, SelectedRunID: value.SelectedRunID, CreatedAt: value.CreatedAt}
}

func NewEventDTO(value Event) EventDTO {
	data := json.RawMessage(value.Data)
	if !json.Valid(data) {
		data = json.RawMessage(`{}`)
	}
	return EventDTO{RunID: value.RunID, Sequence: value.Sequence, Type: value.Type, Data: data, CreatedAt: value.CreatedAt}
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

func NewSnapshotDTO(value Snapshot) SnapshotDTO {
	session := NewSessionDTO(value.Session)
	turns := make([]SnapshotTurnDTO, 0, len(value.Turns))
	cursors := make([]EventCursorDTO, 0, len(value.Turns))
	for _, value := range value.Turns {
		messages := make([]MessageDTO, 0, len(value.Messages))
		for _, message := range value.Messages {
			messages = append(messages, MessageDTO{Role: message.Role, Content: message.Content, CreatedAt: message.CreatedAt})
		}
		runs := make([]SnapshotRunDTO, 0, len(value.Runs))
		for _, run := range value.Runs {
			runMessages := make([]MessageDTO, 0, len(run.Messages))
			for _, message := range run.Messages {
				runMessages = append(runMessages, MessageDTO{Role: message.Role, Content: message.Content, CreatedAt: message.CreatedAt})
			}
			runs = append(runs, SnapshotRunDTO{Run: NewRunDTO(run.Run), Events: NewEventDTOs(run.Events), Messages: runMessages})
		}
		turns = append(turns, SnapshotTurnDTO{Turn: NewTurnDTO(value.Turn), Run: NewRunDTO(value.Run), Events: NewEventDTOs(value.Events), Messages: messages, Runs: runs})
		sequence := 0
		if len(value.Events) > 0 {
			sequence = value.Events[len(value.Events)-1].Sequence
		}
		cursors = append(cursors, EventCursorDTO{RunID: value.Run.ID, Sequence: sequence})
		for _, run := range value.Runs {
			if run.Run.ID == value.Run.ID {
				continue
			}
			sequence = 0
			if len(run.Events) > 0 {
				sequence = run.Events[len(run.Events)-1].Sequence
			}
			cursors = append(cursors, EventCursorDTO{RunID: run.Run.ID, Sequence: sequence})
		}
	}
	return SnapshotDTO{Session: session, Pets: session.Pets, Turns: turns, EventCursors: cursors}
}
