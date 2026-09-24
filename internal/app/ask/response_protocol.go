package ask

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// 本文件固定 v2 核心问答的「动作先行响应协议 record_array_v1」（文档 8.4）。
// 模型以有序 JSON 数组承载整次响应：首个元素为 header，中间记录与动作匹配，最后为 end。
// 数组尚未闭合时可解析其中已闭合的元素；字符串内换行、转义字符或网络包边界均不是记录边界。
// 服务端拒绝重复对象键、重复记录键、未知字段或记录类型、数组外杂文、多个 header、
// end 之后的内容以及与已冻结动作冲突的记录。

// RecordArrayV1 是当前支持的响应协议版本。
const RecordArrayV1 = "record_array_v1"

// ResponseAction 是 header 冻结的三类业务动作（文档 2.2、8.4）。
type ResponseAction string

const (
	ActionCallTools    ResponseAction = "call_tools"
	ActionRequestInput ResponseAction = "request_input"
	ActionFinalAnswer  ResponseAction = "final_answer"
)

// Valid 报告动作是否为契约内合法值。
func (a ResponseAction) Valid() bool {
	switch a {
	case ActionCallTools, ActionRequestInput, ActionFinalAnswer:
		return true
	default:
		return false
	}
}

// RecordType 是响应数组内每条记录的类型（文档 8.4 表格）。
type RecordType string

const (
	RecordHeader   RecordType = "header"
	RecordGroup    RecordType = "group"
	RecordSegment  RecordType = "segment"
	RecordRisk     RecordType = "risk"
	RecordQuestion RecordType = "question"
	RecordCall     RecordType = "call"
	RecordCoverage RecordType = "coverage"
	RecordEnd      RecordType = "end"
)

// ProtocolRecord 是响应数组内一条已闭合且已按类型解析的记录。
// 仅 Type 对应的指针字段非空，其余为 nil。
type ProtocolRecord struct {
	Type     RecordType
	Header   *HeaderRecord
	Group    *GroupRecord
	Segment  *SegmentRecord
	Risk     *RiskRecord
	Question *QuestionRecord
	Call     *CallRecord
	Coverage *CoverageRecord
}

// HeaderRecord：header 记录，冻结本次响应的动作与任务项更新。
// TaskUpdates 为空集合表示无更新（文档 8.4）。
type HeaderRecord struct {
	Type          RecordType     `json:"type"`
	SchemaVersion string         `json:"schema_version"`
	Action        ResponseAction `json:"action"`
	TaskUpdates   []TaskUpdate   `json:"task_updates"`
}

// TaskUpdate：header 中带输入依据的新任务临时键（文档 2.3.1、8.4）。
// TaskKey 仅在本次响应内唯一，由 Runtime 校验后分配稳定 task_item_id。
type TaskUpdate struct {
	TaskKey       string   `json:"task_key"`
	Goal          string   `json:"goal"`
	SourceTurnIDs []string `json:"source_turn_ids,omitempty"` // 输入依据（本次 Run 已消费输入）
	SubjectKeys   []string `json:"subject_keys,omitempty"`    // 已解析对象键或留待明确
}

// GroupRecord：group 记录，建立内容所属任务和对象（文档 8.5、8.4）。
type GroupRecord struct {
	Type       RecordType      `json:"type"`
	GroupKey   string          `json:"group_key"`
	TaskKeys   []string        `json:"task_keys"`
	AnswerKind AnswerKind      `json:"answer_kind"`
	Subjects   []AnswerSubject `json:"subjects"`
	Scope      AnswerScope     `json:"scope"`
}

// SegmentRecord：segment 记录，一条可发布正文段（文档 8.4、8.6）。
type SegmentRecord struct {
	Type         RecordType    `json:"type"`
	SegmentKey   string        `json:"segment_key"`
	GroupKey     string        `json:"group_key"`
	SubjectKeys  []string      `json:"subject_keys"`
	Field        string        `json:"field"`
	Text         string        `json:"text"`
	BasisKind    BasisKind     `json:"basis_kind"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs,omitempty"`
}

// RiskRecord：risk 记录，每组每对象至多一条模型风险建议（文档 8.4）。
type RiskRecord struct {
	Type        RecordType    `json:"type"`
	GroupKey    string        `json:"group_key"`
	SubjectKey  string        `json:"subject_key"`
	Level       RiskLevel     `json:"level"`
	Evidence    []EvidenceRef `json:"evidence,omitempty"`
	Uncertainty string        `json:"uncertainty,omitempty"`
}

// QuestionRecord：question 记录，仅 request_input 使用（文档 2.5、8.4）。
type QuestionRecord struct {
	Type          RecordType     `json:"type"`
	QuestionKey   string         `json:"question_key"`
	TaskKeys      []string       `json:"task_keys"`
	SubjectKeys   []string       `json:"subject_keys,omitempty"`
	Text          string         `json:"text"`
	MissingFields []MissingField `json:"missing_fields"`
	Purpose       string         `json:"purpose,omitempty"`
}

// MissingField：追问中一项待补信息（文档 2.5）。
type MissingField struct {
	TaskKey    string `json:"task_key"`
	SubjectKey string `json:"subject_key,omitempty"`
	Field      string `json:"field"`
	Necessity  string `json:"necessity"`
	KnownValue string `json:"known_value,omitempty"`
}

// CallRecord：call 记录，仅 call_tools 使用（文档 8.4、7 节）。
type CallRecord struct {
	Type           RecordType      `json:"type"`
	CallKey        string          `json:"call_key"`
	TaskKeys       []string        `json:"task_keys"`
	ToolName       string          `json:"tool_name"`
	CatalogVersion string          `json:"catalog_version"`
	Arguments      json.RawMessage `json:"arguments"`
	DependsOn      []string        `json:"depends_on,omitempty"`
}

// CoverageRecord：coverage 记录，模型提出的覆盖关系（文档 8.4）。
type CoverageRecord struct {
	Type  RecordType     `json:"type"`
	Tasks []TaskCoverage `json:"tasks"`
}

// TaskCoverage：单个有效任务项的覆盖关系（文档 8.4、2.3.2）。
type TaskCoverage struct {
	TaskKey          string   `json:"task_key"`
	AnswerGroupKeys  []string `json:"answer_group_keys,omitempty"`
	QuestionKeys     []string `json:"question_keys,omitempty"`
	CallKeys         []string `json:"call_keys,omitempty"`
	OperationIDs     []string `json:"operation_ids,omitempty"`
	IncompleteReason string   `json:"incomplete_reason,omitempty"`
}

// RecordArrayParser 增量解析 record_array_v1 有序 JSON 数组。
// 它是纯状态机，只从已识别字段提取已闭合的完整记录，不直接展示原始 JSON，
// 不用字符串截取猜测嵌套结构（文档 8.4、8.6）。
type RecordArrayParser struct {
	buf           []byte
	started       bool
	closed        bool
	needSeparator bool
}

func NewRecordArrayParser() *RecordArrayParser {
	return &RecordArrayParser{}
}

// Feed 送入一段流式分片，返回自上次以来已闭合的完整记录。
// 半截记录继续缓存，直到其闭合；遇到协议错误时返回已解析记录与错误。
func (p *RecordArrayParser) Feed(chunk string) ([]ProtocolRecord, error) {
	p.buf = append(p.buf, chunk...)
	records := make([]ProtocolRecord, 0)
	for {
		i := skipJSONSpace(p.buf, 0)
		if i >= len(p.buf) {
			p.buf = p.buf[:0]
			break
		}
		if p.closed {
			return records, fmt.Errorf("record_array: unexpected content after ']'")
		}
		if !p.started {
			if p.buf[i] != '[' {
				return records, fmt.Errorf("record_array: expected '[' but got %q", p.buf[i])
			}
			p.started = true
			p.buf = p.buf[i+1:]
			continue
		}
		if p.needSeparator {
			switch p.buf[i] {
			case ',':
				p.needSeparator = false
				p.buf = p.buf[i+1:]
			case ']':
				p.closed = true
				p.buf = p.buf[i+1:]
			default:
				return records, fmt.Errorf("record_array: expected ',' or ']', got %q", p.buf[i])
			}
			continue
		}
		if p.buf[i] == ']' {
			p.closed = true
			p.buf = p.buf[i+1:]
			continue
		}
		if p.buf[i] != '{' {
			return records, fmt.Errorf("record_array: expected record object, got %q", p.buf[i])
		}
		end := findJSONValueEnd(p.buf, i)
		if end < 0 {
			p.buf = p.buf[i:]
			break
		}
		rec, err := decodeRecord(p.buf[i:end])
		if err != nil {
			return records, err
		}
		records = append(records, rec)
		p.buf = p.buf[end:]
		p.needSeparator = true
	}
	return records, nil
}

// Closed 报告数组是否已闭合（收到 ']'）。
func (p *RecordArrayParser) Closed() bool { return p.closed }

// normalizeCallArguments 归一化 call 记录的 arguments。
// 协议正文（strict 结构化输出）使用 JSON 字符串承载工具参数，需转义内部引号；
// 非 strict 回退时模型可能直接给出对象，两种形式都接受并统一为 JSON 对象字节。
func normalizeCallArguments(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] != '"' {
		return json.RawMessage(trimmed), nil
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, fmt.Errorf("arguments string is invalid: %w", err)
	}
	return json.RawMessage(bytes.TrimSpace([]byte(value))), nil
}

// ValidateResponse 校验一次完整响应：先冻结 header 动作，再按动作校验记录集合
// （文档 8.4）。返回冻结的动作与首个结构化错误。
func ValidateResponse(records []ProtocolRecord) (ResponseAction, error) {
	if len(records) == 0 {
		return "", fmt.Errorf("record_array: missing header")
	}
	if records[0].Type != RecordHeader || records[0].Header == nil {
		return "", fmt.Errorf("record_array: first record must be header")
	}
	header := records[0].Header
	if header.SchemaVersion != RecordArrayV1 {
		return "", fmt.Errorf("record_array: unsupported schema_version %q", header.SchemaVersion)
	}
	if !header.Action.Valid() {
		return "", fmt.Errorf("record_array: invalid action %q", header.Action)
	}
	if err := validateTaskUpdates(header.TaskUpdates); err != nil {
		return header.Action, err
	}

	seenHeader := 0
	seenCoverage := 0
	seenEnd := 0
	groups := make(map[string]*GroupRecord)
	recordKeys := make(map[string]string) // key -> record type，用于重复记录键检测
	riskSubjects := make(map[string]bool) // group_key + "|" + subject_key

	allowed := allowedRecordKinds(header.Action)
	for index, rec := range records {
		if rec.Type != RecordHeader && rec.Type != RecordCoverage && rec.Type != RecordEnd {
			if !allowed[rec.Type] {
				return header.Action, fmt.Errorf("record_array: record %q not allowed for action %q", rec.Type, header.Action)
			}
		}
		switch rec.Type {
		case RecordHeader:
			seenHeader++
			if seenHeader > 1 {
				return header.Action, fmt.Errorf("record_array: duplicate header")
			}
		case RecordCoverage:
			seenCoverage++
			if seenCoverage > 1 {
				return header.Action, fmt.Errorf("record_array: duplicate coverage")
			}
			if index != len(records)-2 {
				return header.Action, fmt.Errorf("record_array: coverage must immediately precede end")
			}
			if err := validateCoverage(rec.Coverage, header.TaskUpdates); err != nil {
				return header.Action, err
			}
		case RecordEnd:
			seenEnd++
			if seenEnd > 1 {
				return header.Action, fmt.Errorf("record_array: duplicate end")
			}
			if index != len(records)-1 {
				return header.Action, fmt.Errorf("record_array: end must be the last record")
			}
		case RecordGroup:
			if err := registerRecordKey(recordKeys, "group", rec.Group.GroupKey); err != nil {
				return header.Action, err
			}
			if err := validateGroup(rec.Group); err != nil {
				return header.Action, err
			}
			groups[rec.Group.GroupKey] = rec.Group
		case RecordSegment:
			if err := registerRecordKey(recordKeys, "segment", rec.Segment.SegmentKey); err != nil {
				return header.Action, err
			}
			if err := validateSegment(rec.Segment, groups, recordKeys); err != nil {
				return header.Action, err
			}
		case RecordRisk:
			if err := validateRisk(rec.Risk, groups, riskSubjects); err != nil {
				return header.Action, err
			}
		case RecordQuestion:
			if err := registerRecordKey(recordKeys, "question", rec.Question.QuestionKey); err != nil {
				return header.Action, err
			}
			if err := validateQuestion(rec.Question); err != nil {
				return header.Action, err
			}
		case RecordCall:
			if err := registerRecordKey(recordKeys, "call", rec.Call.CallKey); err != nil {
				return header.Action, err
			}
			if err := validateCall(rec.Call); err != nil {
				return header.Action, err
			}
		}
	}

	if seenEnd == 0 {
		return header.Action, fmt.Errorf("record_array: missing end")
	}
	if seenCoverage == 0 {
		// coverage 缺失不是协议错误：模型经常在 call_tools 时漏写，
		// 由 DecideStep 依据任务项、调用、追问与回答组推导出等价覆盖关系。
		return header.Action, nil
	}
	switch header.Action {
	case ActionCallTools:
		if !recordKeysHaveType(recordKeys, "call") {
			return header.Action, fmt.Errorf("record_array: call_tools requires at least one call")
		}
	case ActionRequestInput:
		if !recordKeysHaveType(recordKeys, "question") {
			return header.Action, fmt.Errorf("record_array: request_input requires at least one question")
		}
	case ActionFinalAnswer:
		if !recordKeysHaveType(recordKeys, "group") {
			return header.Action, fmt.Errorf("record_array: final_answer requires at least one group")
		}
	}
	return header.Action, nil
}

// allowedRecordKinds 报告动作允许的内容记录类型（header/coverage/end 单独处理）。
func allowedRecordKinds(action ResponseAction) map[RecordType]bool {
	switch action {
	case ActionCallTools:
		return map[RecordType]bool{RecordCall: true}
	case ActionRequestInput:
		return map[RecordType]bool{RecordQuestion: true}
	case ActionFinalAnswer:
		return map[RecordType]bool{RecordGroup: true, RecordSegment: true, RecordRisk: true}
	default:
		return map[RecordType]bool{}
	}
}

func registerRecordKey(keys map[string]string, kind, key string) error {
	if key == "" {
		return fmt.Errorf("record_array: %s record has empty key", kind)
	}
	if prev, ok := keys[key]; ok {
		return fmt.Errorf("record_array: duplicate %s key %q (already used by %s)", kind, key, prev)
	}
	keys[key] = kind
	return nil
}

func recordKeysHaveType(keys map[string]string, kind string) bool {
	for _, v := range keys {
		if v == kind {
			return true
		}
	}
	return false
}

func validateGroup(g *GroupRecord) error {
	if !g.AnswerKind.Valid() {
		return fmt.Errorf("record_array: invalid answer_kind %q", g.AnswerKind)
	}
	if !g.Scope.Valid() {
		return fmt.Errorf("record_array: invalid scope %q", g.Scope)
	}
	if len(g.TaskKeys) == 0 {
		return fmt.Errorf("record_array: group %q has empty task_keys", g.GroupKey)
	}
	if len(g.Subjects) == 0 {
		return fmt.Errorf("record_array: group %q has empty subjects", g.GroupKey)
	}
	subjectKeys := make(map[string]bool)
	for _, s := range g.Subjects {
		if s.SubjectKey == "" {
			return fmt.Errorf("record_array: group %q has subject with empty key", g.GroupKey)
		}
		if subjectKeys[s.SubjectKey] {
			return fmt.Errorf("record_array: group %q has duplicate subject_key %q", g.GroupKey, s.SubjectKey)
		}
		subjectKeys[s.SubjectKey] = true
		if s.Kind == SubjectPet && s.PetID == "" {
			return fmt.Errorf("record_array: group %q subject %q missing pet_id", g.GroupKey, s.SubjectKey)
		}
		if s.Kind == SubjectUnresolved && s.Description == "" {
			return fmt.Errorf("record_array: group %q subject %q missing description", g.GroupKey, s.SubjectKey)
		}
	}
	return nil
}

func validateSegment(s *SegmentRecord, groups map[string]*GroupRecord, keys map[string]string) error {
	g, ok := groups[s.GroupKey]
	if !ok {
		return fmt.Errorf("record_array: segment %q references unknown group %q", s.SegmentKey, s.GroupKey)
	}
	if s.Field == "" {
		return fmt.Errorf("record_array: segment %q has empty field", s.SegmentKey)
	}
	if s.Text == "" {
		return fmt.Errorf("record_array: segment %q has empty text", s.SegmentKey)
	}
	if !s.BasisKind.Valid() {
		return fmt.Errorf("record_array: segment %q has invalid basis_kind %q", s.SegmentKey, s.BasisKind)
	}
	if s.BasisKind == BasisBusinessFact && len(s.EvidenceRefs) == 0 {
		return fmt.Errorf("record_array: business_fact segment %q requires evidence_refs", s.SegmentKey)
	}
	if !allowedFieldForKind(g.AnswerKind, s.Field) {
		return fmt.Errorf("record_array: segment %q field %q not allowed for answer_kind %q", s.SegmentKey, s.Field, g.AnswerKind)
	}
	for _, sk := range s.SubjectKeys {
		if !groupHasSubject(g, sk) {
			return fmt.Errorf("record_array: segment %q references unknown subject %q", s.SegmentKey, sk)
		}
	}
	return nil
}

func validateTaskUpdates(updates []TaskUpdate) error {
	seen := make(map[string]struct{}, len(updates))
	for _, update := range updates {
		if update.TaskKey == "" {
			return fmt.Errorf("record_array: task update has empty task_key")
		}
		if update.Goal == "" {
			return fmt.Errorf("record_array: task update %q has empty goal", update.TaskKey)
		}
		if _, ok := seen[update.TaskKey]; ok {
			return fmt.Errorf("record_array: duplicate task_key %q in task_updates", update.TaskKey)
		}
		seen[update.TaskKey] = struct{}{}
	}
	return nil
}

func validateRisk(r *RiskRecord, groups map[string]*GroupRecord, seen map[string]bool) error {
	g, ok := groups[r.GroupKey]
	if !ok {
		return fmt.Errorf("record_array: risk references unknown group %q", r.GroupKey)
	}
	if !groupHasSubject(g, r.SubjectKey) {
		return fmt.Errorf("record_array: risk references unknown subject %q", r.SubjectKey)
	}
	if !r.Level.Valid() {
		return fmt.Errorf("record_array: risk has invalid level %q", r.Level)
	}
	key := r.GroupKey + "|" + r.SubjectKey
	if seen[key] {
		return fmt.Errorf("record_array: duplicate risk for subject %q in group %q", r.SubjectKey, r.GroupKey)
	}
	seen[key] = true
	return nil
}

func validateQuestion(q *QuestionRecord) error {
	if q.Text == "" {
		return fmt.Errorf("record_array: question %q has empty text", q.QuestionKey)
	}
	if len(q.TaskKeys) == 0 {
		return fmt.Errorf("record_array: question %q has empty task_keys", q.QuestionKey)
	}
	return nil
}

func validateCall(c *CallRecord) error {
	if c.ToolName == "" {
		return fmt.Errorf("record_array: call %q has empty tool_name", c.CallKey)
	}
	if len(c.Arguments) == 0 || !json.Valid(c.Arguments) {
		return fmt.Errorf("record_array: call %q has invalid arguments", c.CallKey)
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(c.Arguments, &args); err != nil {
		return fmt.Errorf("record_array: call %q arguments must be a JSON object", c.CallKey)
	}
	// 空对象 {} 是合法参数：无参数工具（例如 list_family_pets）只能给出空集合。
	return nil
}

func validateCoverage(c *CoverageRecord, updates []TaskUpdate) error {
	seen := make(map[string]bool)
	for _, t := range c.Tasks {
		if t.TaskKey == "" {
			return fmt.Errorf("record_array: coverage task has empty task_key")
		}
		if seen[t.TaskKey] {
			return fmt.Errorf("record_array: coverage has duplicate task_key %q", t.TaskKey)
		}
		seen[t.TaskKey] = true
	}
	return nil
}

func groupHasSubject(g *GroupRecord, subjectKey string) bool {
	for _, s := range g.Subjects {
		if s.SubjectKey == subjectKey {
			return true
		}
	}
	return false
}

// decodeRecord 把一条已闭合记录对象解析为带类型的 ProtocolRecord。
// 拒绝重复对象键、未知字段、未知记录类型与非对象输入（文档 8.4）。
func decodeRecord(raw []byte) (ProtocolRecord, error) {
	if err := rejectDuplicateKeys(raw); err != nil {
		return ProtocolRecord{}, err
	}
	var probe struct {
		Type RecordType `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ProtocolRecord{}, fmt.Errorf("record_array: invalid record JSON: %w", err)
	}
	rec := ProtocolRecord{Type: probe.Type}
	switch probe.Type {
	case RecordHeader:
		h := HeaderRecord{}
		if err := decodeStrict(raw, &h); err != nil {
			return rec, fmt.Errorf("record_array: invalid header: %w", err)
		}
		rec.Header = &h
	case RecordGroup:
		g := GroupRecord{}
		if err := decodeStrict(raw, &g); err != nil {
			return rec, fmt.Errorf("record_array: invalid group: %w", err)
		}
		rec.Group = &g
	case RecordSegment:
		s := SegmentRecord{}
		if err := decodeStrict(raw, &s); err != nil {
			return rec, fmt.Errorf("record_array: invalid segment: %w", err)
		}
		rec.Segment = &s
	case RecordRisk:
		r := RiskRecord{}
		if err := decodeStrict(raw, &r); err != nil {
			return rec, fmt.Errorf("record_array: invalid risk: %w", err)
		}
		rec.Risk = &r
	case RecordQuestion:
		q := QuestionRecord{}
		if err := decodeStrict(raw, &q); err != nil {
			return rec, fmt.Errorf("record_array: invalid question: %w", err)
		}
		rec.Question = &q
	case RecordCall:
		c := CallRecord{}
		if err := decodeStrict(raw, &c); err != nil {
			return rec, fmt.Errorf("record_array: invalid call: %w", err)
		}
		arguments, err := normalizeCallArguments(c.Arguments)
		if err != nil {
			return rec, fmt.Errorf("record_array: invalid call: %w", err)
		}
		c.Arguments = arguments
		rec.Call = &c
	case RecordCoverage:
		c := CoverageRecord{}
		if err := decodeStrict(raw, &c); err != nil {
			return rec, fmt.Errorf("record_array: invalid coverage: %w", err)
		}
		rec.Coverage = &c
	case RecordEnd:
		// end 是结束标记，无业务字段（文档 8.4）。
		var end struct {
			Type RecordType `json:"type"`
		}
		if err := decodeStrict(raw, &end); err != nil {
			return rec, fmt.Errorf("record_array: invalid end: %w", err)
		}
	default:
		return rec, fmt.Errorf("record_array: unknown record type %q", probe.Type)
	}
	return rec, nil
}

func decodeStrict(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// rejectDuplicateKeys 拒绝同一对象内的重复键（文档 8.4），并确认顶层为对象。
func rejectDuplicateKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("record_array: record is not a JSON object")
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("record_array: record must be a JSON object")
	}
	seen := make(map[string]bool)
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := keyToken.(string)
		if seen[key] {
			return fmt.Errorf("record_array: duplicate object key %q", key)
		}
		seen[key] = true
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
	}
	return nil
}

func skipJSONSpace(data []byte, i int) int {
	for i < len(data) {
		switch data[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

// findJSONValueEnd 返回从 i 开始的完整 JSON 值的结束下标（不含），不完整时返回 -1。
// 处理对象/数组嵌套与字符串转义，不把字符串内的换行或转义引号当作值边界。
func findJSONValueEnd(data []byte, i int) int {
	depth := 0
	inString := false
	escape := false
	for j := i; j < len(data); j++ {
		c := data[j]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return -1
}
