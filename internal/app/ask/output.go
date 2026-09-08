package ask

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidAnalysisOutput = errors.New("invalid ask analysis output")

var analysisRequiredFields = []string{
	"current_assessment",
	"observations",
	"possible_causes",
	"home_actions",
	"escalation_conditions",
}

var unsafeAnalysisTerms = []string{"确诊", "诊断为", "处方", "用药剂量", "服用剂量", "毫克", "mg"}

func ValidateAnalysisOutput(decision RunDecision) error {
	if decision.Status != RunCompleted {
		return nil
	}
	if _, ok := decision.Data["fact_type"]; ok {
		if decision.RiskLevel != RiskGreen || decision.Data["items"] == nil {
			return fmt.Errorf("%w: invalid fact output", ErrInvalidAnalysisOutput)
		}
		return nil
	}
	if decision.RiskLevel != RiskGreen && decision.RiskLevel != RiskYellow {
		return fmt.Errorf("%w: invalid risk level", ErrInvalidAnalysisOutput)
	}
	if decision.Data == nil {
		return fmt.Errorf("%w: missing data", ErrInvalidAnalysisOutput)
	}
	for _, field := range analysisRequiredFields {
		value, ok := decision.Data[field]
		if !ok {
			return fmt.Errorf("%w: missing %s", ErrInvalidAnalysisOutput, field)
		}
		if field == "current_assessment" {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("%w: invalid %s", ErrInvalidAnalysisOutput, field)
			}
			if containsUnsafeAnalysisTerm(text) {
				return fmt.Errorf("%w: unsafe %s", ErrInvalidAnalysisOutput, field)
			}
			continue
		}
		items, ok := analysisStrings(value)
		if !ok || len(items) == 0 {
			return fmt.Errorf("%w: invalid %s", ErrInvalidAnalysisOutput, field)
		}
		for _, item := range items {
			if strings.TrimSpace(item) == "" || containsUnsafeAnalysisTerm(item) {
				return fmt.Errorf("%w: unsafe %s", ErrInvalidAnalysisOutput, field)
			}
		}
		if field == "possible_causes" && len(items) > 3 {
			return fmt.Errorf("%w: too many possible causes", ErrInvalidAnalysisOutput)
		}
	}
	return nil
}

func analysisStrings(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return values, true
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			item, ok := value.(string)
			if !ok {
				return nil, false
			}
			result = append(result, item)
		}
		return result, true
	default:
		return nil, false
	}
}

func containsUnsafeAnalysisTerm(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, term := range unsafeAnalysisTerms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}
