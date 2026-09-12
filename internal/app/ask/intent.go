package ask

import (
	"context"
	"errors"
	"strings"
)

type Intent string

const (
	IntentCasualChat  Intent = "casual_chat"
	IntentPetHealth   Intent = "pet_health"
	IntentPetFact     Intent = "pet_fact"
	IntentFamilyQuery Intent = "family_query"
	IntentAmbiguous   Intent = "ambiguous"
	IntentUnsupported Intent = "unsupported"
)

type IntentInput struct {
	Session  Session
	Turn     Turn
	Run      Run
	Messages []ContextMessage
}

type IntentDecision struct {
	Intent   Intent
	Reply    string
	Question string
}

type IntentRouter interface {
	Route(context.Context, IntentInput) (IntentDecision, error)
}

func ValidateIntentDecision(value IntentDecision) error {
	switch value.Intent {
	case IntentPetHealth, IntentPetFact, IntentFamilyQuery:
		return nil
	case IntentCasualChat, IntentUnsupported:
		if strings.TrimSpace(value.Reply) == "" {
			return errors.New("intent reply is required")
		}
		return nil
	case IntentAmbiguous:
		if strings.TrimSpace(value.Question) == "" {
			return errors.New("intent question is required")
		}
		return nil
	default:
		return errors.New("intent is invalid")
	}
}

func DetectFamilyQuery(input string) (IntentDecision, bool) {
	value := strings.ToLower(strings.TrimSpace(input))
	value = strings.Trim(value, "，。！？!?、~～. ")
	patterns := []string{"我家有哪些宠物", "家里有哪些宠物", "有哪些宠物", "宠物列表", "我的宠物有哪些"}
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return IntentDecision{Intent: IntentFamilyQuery}, true
		}
	}
	return IntentDecision{}, false
}

func DetectSimpleIntent(input string) (IntentDecision, bool) {
	value := strings.ToLower(strings.TrimSpace(input))
	value = strings.Trim(value, "，。！？!?、~～. ")
	switch value {
	case "你好", "您好", "嗨", "哈喽", "hello", "hi":
		return IntentDecision{Intent: IntentCasualChat, Reply: "你好，我可以陪你聊聊，也可以帮你查看宠物记录或整理健康问题。"}, true
	case "谢谢", "感谢", "谢谢你":
		return IntentDecision{Intent: IntentCasualChat, Reply: "不客气，有需要可以继续问我。"}, true
	case "你是谁":
		return IntentDecision{Intent: IntentCasualChat, Reply: "我是宠物家庭助手，可以帮你查看宠物记录并整理健康观察信息。"}, true
	case "你能做什么", "你会做什么":
		return IntentDecision{Intent: IntentCasualChat, Reply: "我可以帮你查看宠物记录、整理健康观察信息，也可以回答简单问题。"}, true
	default:
		return IntentDecision{}, false
	}
}
