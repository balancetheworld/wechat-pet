package safety

import "strings"

// 本文件是项目唯一的红旗症状表（问题二）。
//
// 用途有两处，共用同一份词表，避免规则层与知识层各自维护一份而对不上：
//  1. Runtime 的确定性风险规则：命中 red 级红旗即升级为 red，直接给出就医提示，
//     不交给模型自由判断（internal/app/ask/rules.go）。
//  2. 知识库：查询命中红旗词时强制保留带升级条件的知识条目，并允许把知识正文里
//     的红旗段落单独抽为 escalation_conditions（internal/platform/knowledge）。
//
// 维护约定：
//   - 只收录「需要立即联系宠物医院」的信号，也就是 red 级；普通的观察项、护理建议
//     放在知识条目里，不要塞进这张表。判断不确定时先不收，宁可漏也不要误报。
//   - Phrases 是用户原话里可能出现的短语，越具体越好；过短的通用词会造成误报。
//   - Code 是稳定标识，发布后不要改名，改名会让历史 Run 的 trigger_code 失去对应。
//   - 修改后必须同步更新 docs/red-flags.md（有测试锁定两边一致）。

// Version 是红旗表版本。词表内容变化时递增，便于审计历史 Run 的判定依据。
const Version = "ask-red-flags-v1"

// Flag 是一条红旗症状。
type Flag struct {
	Code    string   // 稳定标识，出现在 Run 事件的 trigger_code
	Label   string   // 中文名，供人工审核与文档对照
	Phrases []string // 触发短语
	Message string   // 观察到的现象描述（不确诊）
	Action  string   // 建议动作
}

// flags 是红旗表本体。顺序即匹配优先级：越靠前的条目越先命中并作为 trigger_code，
// 因此已经上线、有历史数据依赖的条目保持在前。
var flags = []Flag{
	{
		Code:    "breathing_distress",
		Label:   "呼吸异常",
		Phrases: []string{"呼吸困难", "呼吸急促", "喘不上气", "喘不过气", "张口呼吸", "呼吸不上来"},
		Message: "观察到可能存在呼吸异常",
		Action:  "请立即联系附近的宠物医院；途中尽量保持安静，避免强行喂食或喂水。",
	},
	{
		Code:    "persistent_seizure",
		Label:   "持续或反复抽搐",
		Phrases: []string{"持续抽搐", "一直抽搐", "反复抽搐", "不停抽搐", "癫痫发作"},
		Message: "观察到可能存在持续或反复抽搐",
		Action:  "请立即联系附近的宠物医院，移开周围尖锐物品，不要把手伸进宠物口中。",
	},
	{
		Code:    "severe_bleeding",
		Label:   "严重出血",
		Phrases: []string{"大量出血", "止不住血", "一直流血", "喷血", "大出血"},
		Message: "观察到可能存在严重出血",
		Action:  "请立即联系附近的宠物医院；可用干净纱布轻压出血处，不要反复掀开查看。",
	},
	{
		Code:    "suspected_poisoning",
		Label:   "误食或中毒",
		Phrases: []string{"疑似中毒", "可能中毒", "误食毒", "吃了老鼠药", "吃了杀虫剂", "误食了杀虫剂", "误食药物", "误食清洁剂", "吃了巧克力", "吃了葡萄", "吃了洋葱", "吃了百合", "吃了木糖醇"},
		Message: "观察到可能存在误食或中毒风险",
		Action:  "请立即联系附近的宠物医院，不要自行催吐或喂药，并保留可能误食物品的信息。",
	},
	{
		Code:    "altered_consciousness",
		Label:   "意识异常",
		Phrases: []string{"意识不清", "失去意识", "无意识", "昏迷", "叫不醒", "没有反应", "没有任何反应", "意识异常"},
		Message: "观察到可能存在意识异常",
		Action:  "请立即联系附近的宠物医院，避免摇晃或强行喂食。",
	},
	{
		Code:    "unable_to_stand",
		Label:   "无法站立或突然瘫倒",
		Phrases: []string{"无法站立", "站不起来", "不能站立", "突然瘫倒", "四肢无力"},
		Message: "观察到可能存在突发运动障碍",
		Action:  "请立即联系附近的宠物医院，减少移动并防止跌落。",
	},
	{
		Code:    "vomiting_blood",
		Label:   "呕血或呕吐物带血",
		Phrases: []string{"呕血", "吐血", "呕吐物带血", "呕吐带血", "吐出血"},
		Message: "观察到呕吐物可能带血",
		Action:  "请立即联系附近的宠物医院，保留呕吐物照片或样本信息，不要自行喂药。",
	},
	{
		Code:    "bloody_stool",
		Label:   "便血或黑便",
		Phrases: []string{"便血", "血便", "大便带血", "拉血", "黑色柏油便", "黑便"},
		Message: "观察到可能存在消化道出血信号",
		Action:  "请立即联系附近的宠物医院，保留排泄物照片，不要自行使用止泻药。",
	},
	{
		Code:    "unable_to_drink",
		Label:   "无法饮水或无法进食",
		Phrases: []string{"无法喝水", "不能喝水", "喝不进水", "无法饮水", "无法进食", "吃不下东西", "拒食", "一口都不吃"},
		Message: "观察到可能无法正常饮水或进食",
		Action:  "请尽快联系附近的宠物医院；不要强行灌水灌食，避免呛咳误吸。",
	},
	{
		Code:    "severe_weakness",
		Label:   "精神极差或明显虚弱",
		Phrases: []string{"精神很差", "精神极差", "精神萎靡", "明显虚弱", "非常虚弱", "瘫软"},
		Message: "观察到可能存在明显精神或体能下降",
		Action:  "请尽快联系附近的宠物医院，途中保持温暖安静，避免剧烈搬动。",
	},
	{
		Code:    "repeated_vomiting",
		Label:   "持续或反复呕吐",
		Phrases: []string{"持续呕吐", "反复呕吐", "连续呕吐", "一直吐", "频繁呕吐", "不停呕吐", "吐了好多次", "吐了七八次"},
		Message: "观察到可能存在持续或反复呕吐",
		Action:  "请尽快联系附近的宠物医院；记录呕吐次数和内容，不要自行使用人用止吐药。",
	},
	{
		Code:    "repeated_diarrhea",
		Label:   "持续或剧烈腹泻",
		Phrases: []string{"持续腹泻", "反复腹泻", "连续腹泻", "一直拉稀", "频繁腹泻", "拉个不停", "剧烈腹泻"},
		Message: "观察到可能存在持续或剧烈腹泻",
		Action:  "请尽快联系附近的宠物医院；注意补液与保温，不要自行使用人用止泻药。",
	},
	{
		Code:    "urinary_obstruction",
		Label:   "排尿困难或尿不出",
		Phrases: []string{"尿不出来", "尿不出", "排尿困难", "尿闭", "一直蹲猫砂没尿", "蹲厕所尿不出"},
		Message: "观察到可能存在排尿受阻",
		Action:  "请立即联系附近的宠物医院；排尿受阻可能在数小时内危及生命，不要等待观察。",
	},
	{
		Code:    "severe_abdominal_pain",
		Label:   "剧烈腹痛或腹部明显膨大",
		Phrases: []string{"腹部剧痛", "肚子很痛", "碰肚子就叫", "腹部膨大", "肚子胀得很大", "腹胀明显"},
		Message: "观察到可能存在剧烈腹痛或腹部异常膨大",
		Action:  "请立即联系附近的宠物医院，不要喂食喂水，避免按压腹部。",
	},
	{
		Code:    "rapid_deterioration",
		Label:   "快速恶化或倒地",
		Phrases: []string{"快速恶化", "急速恶化", "突然倒地", "倒地不起", "越来越差", "一下子就不行了"},
		Message: "观察到可能短时间内明显恶化",
		Action:  "请立即联系附近的宠物医院，保持安静保暖，随时准备送医。",
	},
	{
		Code:    "dehydration",
		Label:   "明显脱水",
		Phrases: []string{"明显脱水", "严重脱水", "皮肤回弹很慢", "牙龈很干", "眼窝凹陷"},
		Message: "观察到可能存在明显脱水",
		Action:  "请尽快联系附近的宠物医院；不要一次性大量灌水，少量多次并提供就医信息。",
	},
}

// Flags 返回红旗表副本，供 Runtime、知识库与人工审核读取。
func Flags() []Flag {
	result := make([]Flag, len(flags))
	copy(result, flags)
	return result
}

// FlagByCode 按稳定标识查找红旗条目。
func FlagByCode(code string) (Flag, bool) {
	for _, flag := range flags {
		if flag.Code == code {
			return flag, true
		}
	}
	return Flag{}, false
}

// Match 返回文本命中的第一条红旗。按表内顺序匹配，忽略否定与假设表达，
// 避免「没有呼吸困难」「如果抽搐怎么办」被误判为急症。
func Match(text string) (Flag, bool) {
	normalized := normalize(text)
	if normalized == "" {
		return Flag{}, false
	}
	for _, flag := range flags {
		for _, phrase := range flag.Phrases {
			if ContainsActivePhrase(normalized, phrase) {
				return flag, true
			}
		}
	}
	return Flag{}, false
}

// Terms 返回文本命中的全部红旗短语（已排除被否定的部分），
// 供知识检索判断是否需要强制保留升级条件。
func Terms(text string) []string {
	normalized := normalize(text)
	if normalized == "" {
		return nil
	}
	matched := make([]string, 0)
	seen := make(map[string]struct{})
	for _, flag := range flags {
		for _, phrase := range flag.Phrases {
			if _, ok := seen[phrase]; ok {
				continue
			}
			if ContainsActivePhrase(normalized, phrase) {
				seen[phrase] = struct{}{}
				matched = append(matched, phrase)
			}
		}
	}
	return matched
}

// ContainsPhrase 报告文本是否包含任一红旗短语，不做否定与假设判断。
// 知识条目正文是审核过的参考文本，抽取升级条件时应按字面判断。
func ContainsPhrase(text string) bool {
	for _, flag := range flags {
		for _, phrase := range flag.Phrases {
			if strings.Contains(text, phrase) {
				return true
			}
		}
	}
	return false
}

func normalize(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.Join(strings.Fields(value), "")
}

// ContainsActivePhrase 报告归一化文本里是否存在未被否定的短语。
func ContainsActivePhrase(text, phrase string) bool {
	index := strings.Index(text, phrase)
	for index >= 0 {
		if !hasNegationOrHypothetical(text[:index]) {
			return true
		}
		next := index + len(phrase)
		remaining := text[next:]
		nextIndex := strings.Index(remaining, phrase)
		if nextIndex < 0 {
			return false
		}
		index = next + nextIndex
	}
	return false
}

// negationMarkers 是否定、假设与询问标记；紧邻短语之前出现时不判为急症。
var negationMarkers = []string{"没有", "无", "未见", "不是", "并非", "如果", "假如", "怎么判断", "如何判断", "担心", "怕"}

func hasNegationOrHypothetical(prefix string) bool {
	for _, marker := range negationMarkers {
		if strings.HasSuffix(prefix, marker) {
			return true
		}
	}
	return false
}
