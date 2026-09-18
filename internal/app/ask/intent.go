package ask

import "strings"

// Intent 是前端兼容的意图字段（降级映射用，文档 8.4）。v2 决策循环不再做
// 意图分类，最终回答的 intent 字段由回答组类型推导（见 run_decision.go intentForGroups）。
type Intent string

const (
	IntentCasualChat Intent = "casual_chat"
	IntentPetHealth  Intent = "pet_health"
	IntentPetFact    Intent = "pet_fact"
)

// DetectFamilyQuery 判断输入是否为「查询家庭宠物列表」类问题。仅用于
// CreateSessionFromInput 在无宠物可解析时区分「家庭暂无宠物」与「需补充宠物名称」。
func DetectFamilyQuery(input string) bool {
	value := strings.ToLower(strings.TrimSpace(input))
	value = strings.Trim(value, "，。！？!?、~～. ")
	patterns := []string{"我家有哪些宠物", "家里有哪些宠物", "有哪些宠物", "宠物列表", "我的宠物有哪些", "有几只宠物", "几只宠物", "家里有多少只宠物", "我有几只宠物"}
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}
