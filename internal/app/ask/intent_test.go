package ask

import "testing"

func TestDetectFamilyQuery(t *testing.T) {
	values := []string{"你知道我家有哪些宠物吗", "家里有哪些宠物？", "宠物列表", "我的宠物有哪些"}
	for _, value := range values {
		decision, ok := DetectFamilyQuery(value)
		if !ok || decision.Intent != IntentFamilyQuery {
			t.Fatalf("DetectFamilyQuery(%q) = %+v, %t", value, decision, ok)
		}
	}
	if _, ok := DetectFamilyQuery("旺仔最近没精神"); ok {
		t.Fatal("health question was detected as family query")
	}
}
