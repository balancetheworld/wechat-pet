package ask

// Intent 是前端兼容的意图字段（降级映射用，文档 8.4）。v2 决策循环不再做
// 意图分类，最终回答的 intent 字段由回答组类型推导（见 run_decision.go intentForGroups）。
type Intent string

const (
	IntentCasualChat Intent = "casual_chat"
	IntentPetHealth  Intent = "pet_health"
	IntentPetFact    Intent = "pet_fact"
)
