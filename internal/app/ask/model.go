package ask

import "time"

import calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"

// SessionStatus：会话生命周期。v2 中 Session 属于用户/家庭，可持续多轮问答，
// 只有 active -> closed 一种转移，closed 为终态。
type SessionStatus string

const (
	SessionActive SessionStatus = "active"
	SessionClosed SessionStatus = "closed"
)

// TurnStatus：Turn 的输入处理状态（独立于 Run 状态）。
type TurnStatus string

const (
	TurnReceived   TurnStatus = "received"
	TurnAttached   TurnStatus = "attached"
	TurnSuperseded TurnStatus = "superseded"
)

// RunStatus：一次任务执行的状态。风险等级独立记录，不再用 escalated 表达。
type RunStatus string

const (
	RunQueued       RunStatus = "queued"
	RunRunning      RunStatus = "running"
	RunWaitingInput RunStatus = "waiting_input"
	RunCanceling    RunStatus = "canceling"
	RunCompleted    RunStatus = "completed"
	RunFailed       RunStatus = "failed"
	RunCanceled     RunStatus = "canceled"
	RunInterrupted  RunStatus = "interrupted"
)

// AttemptStatus：至多一次模型请求的结果状态。
type AttemptStatus string

const (
	AttemptQueued    AttemptStatus = "queued"
	AttemptRunning   AttemptStatus = "running"
	AttemptSucceeded AttemptStatus = "succeeded"
	AttemptFailed    AttemptStatus = "failed"
	AttemptCanceled  AttemptStatus = "canceled"
	AttemptUnknown   AttemptStatus = "unknown"
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
	InputSequence    int
	EventSequence    int
	LaunchInstance   string
	Generation       int
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

type Turn struct {
	ID            string
	SessionID     string
	TurnIndex     int
	InputSequence int
	Status        TurnStatus
	Input         string
	AssetRefs     []string
	SelectedRunID string
	SupersededBy  *string
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
	Status    string
	ItemCount int
	Truncated bool
}

type ContextEvent struct {
	Tag        string
	Source     string
	SourceID   string
	Version    string
	Summary    string
	OccurredAt time.Time
}

type ContextSnapshot struct {
	Version       string
	CapturedAt    time.Time
	CharCount     int
	Pet           PetContext
	Pets          []PetContext
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
	OriginTurnID       string
	RunIndex           int
	RowVersion         int
	ClarificationCount int
	Status             RunStatus
	RiskLevel          RiskLevel
	InputRevision      int
	ExecutionEpoch     int
	TerminationReason  string
	Checkpoint         string
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

// Attempt：至多发出一次模型请求，独立持久化。
type Attempt struct {
	ID            string
	SessionID     string
	RunID         string
	Sequence      int
	Status        AttemptStatus
	Purpose       string
	InputSnapshot string
	RequestID     string
	ErrorCode     string
	Usage         string
	StartedAt     time.Time
	CompletedAt   *time.Time
}

// TaskItem 的领域模型已迁移至 task_item.go（文档 2.3、2.3.1、2.3.2），
// 旧字段在此不再保留。

// OperationStatus：确认写入操作的独立状态。
type OperationStatus string

const (
	OperationPending   OperationStatus = "pending"
	OperationConfirmed OperationStatus = "confirmed"
	OperationExecuting OperationStatus = "executing"
	OperationSucceeded OperationStatus = "succeeded"
	OperationFailed    OperationStatus = "failed"
	OperationUnknown   OperationStatus = "unknown"
	OperationAbandoned OperationStatus = "abandoned"
	OperationWithdrawn OperationStatus = "withdrawn"
	OperationExpired   OperationStatus = "expired"
)

// Operation：独立于聊天 Run 的确认写入操作。
type Operation struct {
	ID          string
	SessionID   string
	RunID       string
	CreatedBy   string
	Status      OperationStatus
	Preview     string
	Target      string
	Payload     string
	Result      string
	Version     int
	ConfirmedAt *time.Time
	ExpiresAt   *time.Time
	VerifyUntil *time.Time
	VerifyCount int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MemoryKind：三类长期记忆。
type MemoryKind string

const (
	MemoryProfile MemoryKind = "profile"
	MemoryEvent   MemoryKind = "event"
	MemoryFact    MemoryKind = "fact"
)

// Memory：长期记忆，带来源版本。
type Memory struct {
	ID            string
	FamilyID      string
	PetID         string
	Kind          MemoryKind
	Content       string
	SourceType    string
	SourceID      string
	SourceVersion string
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SourceVersion：来源集合版本，用于检索失效判断。
type SourceVersion struct {
	SourceType  string
	SourceID    string
	Version     string
	VersionedAt time.Time
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
