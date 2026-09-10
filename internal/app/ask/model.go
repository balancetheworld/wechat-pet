package ask

import "time"

import calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"

type SessionStatus string

const (
	SessionActive    SessionStatus = "active"
	SessionCompleted SessionStatus = "completed"
	SessionEscalated SessionStatus = "escalated"
	SessionCanceled  SessionStatus = "canceled"
)

type RunStatus string

const (
	RunQueued       RunStatus = "queued"
	RunRunning      RunStatus = "running"
	RunWaitingInput RunStatus = "waiting_input"
	RunCompleted    RunStatus = "completed"
	RunEscalated    RunStatus = "escalated"
	RunFailed       RunStatus = "failed"
	RunCanceled     RunStatus = "canceled"
	RunInterrupted  RunStatus = "interrupted"
)

type RiskLevel string

const (
	RiskUnknown RiskLevel = "unknown"
	RiskGreen   RiskLevel = "green"
	RiskYellow  RiskLevel = "yellow"
	RiskRed     RiskLevel = "red"
)

type Session struct {
	ID               string
	FamilyID         string
	PetID            string
	CreatedBy        string
	Status           SessionStatus
	RiskLevel        RiskLevel
	TurnCount        int
	PromptVersion    string
	RuleVersion      string
	KnowledgeVersion string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time
	Pets             []SessionPet
}

type SessionPet struct {
	PetID     string
	PetName   string
	Mention   string
	SortOrder int
}

type FactType string

const (
	FactUnknown    FactType = ""
	FactBath       FactType = "bath"
	FactVaccine    FactType = "vaccine"
	FactDeworming  FactType = "deworming"
	FactCheckup    FactType = "checkup"
	FactVisit      FactType = "visit"
	FactMedication FactType = "medication"
)

type Turn struct {
	ID            string
	SessionID     string
	TurnIndex     int
	Status        RunStatus
	Input         string
	SelectedRunID string
	CreatedAt     time.Time
}

type PetContext struct {
	ID                 string
	Name               string
	Breed              string
	Gender             string
	Sterilized         bool
	Birthday           string
	HealthStatus       string
	Allergies          string
	LongTermMedication string
}

type ContextTurn struct {
	TurnIndex int
	Input     string
	Status    RunStatus
	CreatedAt time.Time
}

type ContextMessage struct {
	Role      string
	Content   string
	CreatedAt time.Time
}

type ContextSource struct {
	Name      string
	Version   string
	ItemCount int
	Truncated bool
}

type ContextEvent struct {
	Tag        string
	Source     string
	Summary    string
	OccurredAt time.Time
}

type ContextSnapshot struct {
	Version       string
	CapturedAt    time.Time
	CharCount     int
	Pet           PetContext
	RecentTurns   []ContextTurn
	Messages      []ContextMessage
	RecentRecords []calendarapp.ContextRecord
	Sources       []ContextSource
	Events        []ContextEvent
}

type Run struct {
	ID                 string
	SessionID          string
	TurnID             string
	RunIndex           int
	RowVersion         int
	ClarificationCount int
	Status             RunStatus
	RiskLevel          RiskLevel
	RuleVersion        string
	PromptVersion      string
	CreatedAt          time.Time
	StartedAt          *time.Time
	CompletedAt        *time.Time
	ErrorCode          string
	LeaseOwner         string
	LeaseExpiresAt     *time.Time
	AttemptCount       int
	NextAttemptAt      *time.Time
}

type SnapshotTurn struct {
	Turn     Turn
	Run      Run
	Events   []Event
	Messages []Message
	Runs     []SnapshotRun
}

type SnapshotRun struct {
	Run      Run
	Events   []Event
	Messages []Message
}

type Snapshot struct {
	Session Session
	Turns   []SnapshotTurn
}

type Event struct {
	ID        string
	SessionID string
	TurnID    string
	RunID     string
	Sequence  int
	Type      string
	Data      string
	CreatedAt time.Time
}

type Message struct {
	ID        string
	SessionID string
	TurnID    string
	RunID     string
	Role      string
	Content   string
	CreatedAt time.Time
}

type IdempotencyRecord struct {
	ID             string
	FamilyID       string
	UserID         string
	Operation      string
	IdempotencyKey string
	RequestHash    string
	ResponseData   string
	SessionID      string
	TurnID         string
	RunID          string
	CreatedAt      time.Time
}
