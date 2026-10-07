package knowledge

// 本文件固定宠物年龄分段表（依据 2026-10-06 整理稿）。
// 宠物成长速度在不同阶段、不同体型间差异很大，不能按「年龄乘以 7」线性换算。
//
// 猫咪（月龄）：
//
//	幼猫期 young  0-11
//	青年期 adult  12-83
//	熟龄期 mature 84-119
//	老年期 senior 120 及以上
//
// 狗狗（月龄，幼年期结束按成年体重分档）：
//
//	幼犬期 young  0 至 puppyEnd-1
//	青年期 adult  puppyEnd-35
//	壮年期 mature 36-83
//	老年期 senior 84 及以上

// stageBoundary 是一个生命周期阶段的月龄起点。
type stageBoundary struct {
	stage     LifeStage
	minMonths int
}

// 猫的分段边界。7 岁（84 个月）开始进入熟龄期，是猫的重要关注节点。
var catStageBoundaries = []stageBoundary{
	{stage: LifeStageYoung, minMonths: 0},
	{stage: LifeStageAdult, minMonths: 12},
	{stage: LifeStageMature, minMonths: 84},
	{stage: LifeStageSenior, minMonths: 120},
}

// 狗的分段边界：7 岁（84 个月）进入老年期；青年期起点由体型决定，
// 见 puppyStageEndMonths。表格中的成年体重分档来自整理稿。
var dogStageBoundaries = []stageBoundary{
	{stage: LifeStageYoung, minMonths: 0},
	{stage: LifeStageAdult, minMonths: 12},
	{stage: LifeStageMature, minMonths: 36},
	{stage: LifeStageSenior, minMonths: 84},
}

// LifeStageForAge 依据物种、月龄与成年体重给出生命周期阶段。
// 物种或年龄未确认时返回空串，调用方不得据此过滤或替代。
// weightKg 为 0 表示体重未知，此时犬的幼年期按中型犬默认值处理。
func LifeStageForAge(species string, ageMonths int, weightKg float64) string {
	if ageMonths < 0 {
		return ""
	}
	switch species {
	case string(SpeciesCat):
		return stageForBoundaries(catStageBoundaries, ageMonths)
	case string(SpeciesDog):
		boundaries := make([]stageBoundary, len(dogStageBoundaries))
		copy(boundaries, dogStageBoundaries)
		boundaries[1].minMonths = puppyStageEndMonths(weightKg)
		return stageForBoundaries(boundaries, ageMonths)
	default:
		return ""
	}
}

// stageForBoundaries 返回月龄命中的最后一段；边界必须按 minMonths 升序排列。
func stageForBoundaries(boundaries []stageBoundary, ageMonths int) string {
	stage := boundaries[0].stage
	for _, boundary := range boundaries {
		if ageMonths >= boundary.minMonths {
			stage = boundary.stage
			continue
		}
		break
	}
	return string(stage)
}

// puppyStageEndMonths 返回幼年期结束的月龄：体型越大，幼年期越长。
// 成年体重未知时按中型犬的 12 个月处理。
func puppyStageEndMonths(weightKg float64) int {
	switch {
	case weightKg <= 0:
		return 12
	case weightKg <= 5:
		return 9
	case weightKg <= 20:
		return 12
	case weightKg <= 40:
		return 18
	default:
		return 21
	}
}
