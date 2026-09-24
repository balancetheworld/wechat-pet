package ask

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// 本文件固定工具批次启动前的整批校验、依赖与并行判定
// （对应架构设计 v2 文档 7.1、7.4）。整批校验是启动前的统一门槛，
// 不是承诺整批执行具有数据库事务的原子性。

// CallValidationError：单个调用的可配对校验错误。
type CallValidationError struct {
	CallIndex  int
	ToolCallID string
	Reason     string
}

// BatchValidationError：整批校验失败，携带每项可配对的错误说明。
// 其余项说明整批未启动，不能暗示它们各自调用了后端。
type BatchValidationError struct {
	CallErrors []CallValidationError
}

func (e *BatchValidationError) Error() string {
	if len(e.CallErrors) == 0 {
		return "ask: batch validation failed"
	}
	parts := make([]string, 0, len(e.CallErrors))
	for _, call := range e.CallErrors {
		parts = append(parts, fmt.Sprintf("call %d (%s): %s", call.CallIndex, call.ToolCallID, call.Reason))
	}
	return "ask: batch validation failed: " + strings.Join(parts, "; ")
}

// findToolInCatalog 在目录中按 name + version 查找工具。
func findToolInCatalog(catalog *Catalog, name, version string) (Tool, bool) {
	if catalog == nil {
		return Tool{}, false
	}
	for _, tool := range catalog.tools {
		if tool.Name == name && tool.Version == version {
			return tool, true
		}
	}
	return Tool{}, false
}

// ValidateBatch 校验整批调用：任何一项存在非法工具、参数、对象权限或不可用的
// 前置依据，都拒绝启动整批。返回 nil 表示整批通过统一门槛。
func ValidateBatch(batch ToolBatch, catalog *Catalog, filter Filter) error {
	var callErrors []CallValidationError
	seenCallIDs := make(map[string]struct{}, len(batch.Calls))
	seenIndexes := make(map[int]struct{}, len(batch.Calls))

	for _, call := range batch.Calls {
		if call.ToolCallID == "" {
			callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: "", Reason: "empty tool_call_id"})
			continue
		}
		if _, ok := seenCallIDs[call.ToolCallID]; ok {
			callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "duplicate tool_call_id"})
		}
		seenCallIDs[call.ToolCallID] = struct{}{}
		if _, ok := seenIndexes[call.CallIndex]; ok {
			callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "duplicate call_index"})
		}
		seenIndexes[call.CallIndex] = struct{}{}

		tool, ok := findToolInCatalog(catalog, call.ToolName, call.ToolVersion)
		if !ok {
			callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "tool not found or version mismatch"})
			continue
		}
		if !filter.allowTool(tool) {
			callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "tool disabled or outside allowed scope"})
		}
		if len(call.Arguments) == 0 || !json.Valid(call.Arguments) {
			callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "invalid arguments"})
			continue
		}
		if len(tool.Parameters) > 0 {
			if err := validateToolArguments(call.Arguments, tool.Parameters); err != nil {
				callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: err.Error()})
			}
		}
	}
	for _, call := range batch.Calls {
		seenDependencies := make(map[string]struct{}, len(call.DependsOn))
		for _, dependencyID := range call.DependsOn {
			switch {
			case dependencyID == call.ToolCallID:
				callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "call cannot depend on itself"})
			case dependencyID == "":
				callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "empty dependency"})
			case hasCallID(seenDependencies, dependencyID):
				callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "duplicate dependency " + dependencyID})
			default:
				seenDependencies[dependencyID] = struct{}{}
				if _, ok := seenCallIDs[dependencyID]; !ok {
					callErrors = append(callErrors, CallValidationError{CallIndex: call.CallIndex, ToolCallID: call.ToolCallID, Reason: "dependency not found: " + dependencyID})
				}
			}
		}
	}
	if len(callErrors) == 0 && hasDependencyCycle(batch.Calls) {
		callErrors = append(callErrors, CallValidationError{Reason: "dependency cycle"})
	}

	if len(callErrors) > 0 {
		return &BatchValidationError{CallErrors: callErrors}
	}
	return nil
}

func hasCallID(values map[string]struct{}, id string) bool {
	_, ok := values[id]
	return ok
}

func hasDependencyCycle(calls []ToolCall) bool {
	dependencies := make(map[string][]string, len(calls))
	for _, call := range calls {
		dependencies[call.ToolCallID] = call.DependsOn
	}
	visiting := make(map[string]bool, len(calls))
	visited := make(map[string]bool, len(calls))
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependencyID := range dependencies[id] {
			if visit(dependencyID) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range dependencies {
		if visit(id) {
			return true
		}
	}
	return false
}

type parameterSchema struct {
	Type                 string                     `json:"type"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Required             []string                   `json:"required"`
	AdditionalProperties *bool                      `json:"additionalProperties"`
	Items                json.RawMessage            `json:"items"`
	Enum                 []json.RawMessage          `json:"enum"`
	Minimum              *float64                   `json:"minimum"`
	Maximum              *float64                   `json:"maximum"`
}

func validateToolArguments(arguments, schemaJSON json.RawMessage) error {
	if len(schemaJSON) == 0 {
		return fmt.Errorf("tool parameters schema is missing")
	}
	if err := validateSchemaValue(arguments, schemaJSON, "arguments"); err != nil {
		return err
	}
	return nil
}

func validateSchemaValue(valueJSON, schemaJSON json.RawMessage, path string) error {
	var schema parameterSchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return fmt.Errorf("%s has invalid schema", path)
	}
	var value any
	if err := json.Unmarshal(valueJSON, &value); err != nil {
		return fmt.Errorf("%s is invalid JSON", path)
	}
	if len(schema.Enum) > 0 && !matchesEnum(value, schema.Enum) {
		return fmt.Errorf("%s has a value outside enum", path)
	}
	switch schema.Type {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		for _, name := range schema.Required {
			if _, exists := object[name]; !exists {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for name, raw := range object {
			property, exists := schema.Properties[name]
			if !exists {
				if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
				continue
			}
			value, err := json.Marshal(raw)
			if err != nil {
				return fmt.Errorf("%s.%s is invalid", path, name)
			}
			if err := validateSchemaValue(value, property, path+"."+name); err != nil {
				return err
			}
		}
	case "array":
		values, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		for index, item := range values {
			raw, err := json.Marshal(item)
			if err != nil {
				return fmt.Errorf("%s[%d] is invalid", path, index)
			}
			if err := validateSchemaValue(raw, schema.Items, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || math.Trunc(number) != number {
			return fmt.Errorf("%s must be an integer", path)
		}
		if schema.Minimum != nil && number < *schema.Minimum {
			return fmt.Errorf("%s is below minimum", path)
		}
		if schema.Maximum != nil && number > *schema.Maximum {
			return fmt.Errorf("%s is above maximum", path)
		}
	case "number":
		number, ok := value.(float64)
		if !ok {
			return fmt.Errorf("%s must be a number", path)
		}
		if schema.Minimum != nil && number < *schema.Minimum {
			return fmt.Errorf("%s is below minimum", path)
		}
		if schema.Maximum != nil && number > *schema.Maximum {
			return fmt.Errorf("%s is above maximum", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	default:
		return fmt.Errorf("%s has unsupported schema type %q", path, schema.Type)
	}
	return nil
}

func matchesEnum(value any, values []json.RawMessage) bool {
	for _, raw := range values {
		var candidate any
		if json.Unmarshal(raw, &candidate) == nil && fmt.Sprint(candidate) == fmt.Sprint(value) {
			return true
		}
	}
	return false
}

// referencesCall 判断调用是否依赖目标调用（以真实数据表达依赖）。
func referencesCall(call ToolCall, targetID string) bool {
	for _, id := range call.DependsOn {
		if id == targetID {
			return true
		}
	}
	return false
}

// CanRunInParallel 判断两个调用能否并行：两者都必须是目录声明的只读操作，
// 且不存在「一个的结果作为另一个参数」的依赖。非只读（如准备写入）一律不并行。
func CanRunInParallel(a, b ToolCall, catalog *Catalog) bool {
	toolA, okA := findToolInCatalog(catalog, a.ToolName, a.ToolVersion)
	toolB, okB := findToolInCatalog(catalog, b.ToolName, b.ToolVersion)
	if !okA || !okB {
		return false
	}
	if !IsReadOnly(toolA.ActionType) || !IsReadOnly(toolB.ActionType) {
		return false
	}
	if referencesCall(a, b.ToolCallID) || referencesCall(b, a.ToolCallID) {
		return false
	}
	return true
}

// BatchDisposition：批次内单个调用的处置。
type BatchDisposition struct {
	CallIndex   int
	ToolCallID  string
	Disposition DispositionKind
	Reason      string
}

// DispositionKind：调用处置分类（7.4 步骤 2）。
type DispositionKind string

const (
	DispositionExecute DispositionKind = "execute" // 将执行
	DispositionReuse   DispositionKind = "reuse"   // 复用候选结果
	DispositionBlocked DispositionKind = "blocked" // Loop Guard 阻断
)
