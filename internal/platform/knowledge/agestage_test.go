package knowledge

import "testing"

func TestLifeStageForAgeCat(t *testing.T) {
	tests := []struct {
		ageMonths int
		want      string
	}{
		{0, "young"},
		{11, "young"},
		{12, "adult"},
		{83, "adult"},
		{84, "mature"},
		{119, "mature"},
		{120, "senior"},
		{240, "senior"},
	}
	for _, test := range tests {
		if got := LifeStageForAge(string(SpeciesCat), test.ageMonths, 0); got != test.want {
			t.Fatalf("cat %d months = %q, want %q", test.ageMonths, got, test.want)
		}
	}
}

func TestLifeStageForAgeDogByWeight(t *testing.T) {
	tests := []struct {
		name      string
		weightKg  float64
		ageMonths int
		want      string
	}{
		{name: "small puppy", weightKg: 4, ageMonths: 8, want: "young"},
		{name: "small adult", weightKg: 4, ageMonths: 9, want: "adult"},
		{name: "medium puppy", weightKg: 12, ageMonths: 11, want: "young"},
		{name: "medium adult", weightKg: 12, ageMonths: 12, want: "adult"},
		{name: "large puppy", weightKg: 30, ageMonths: 17, want: "young"},
		{name: "large adult", weightKg: 30, ageMonths: 18, want: "adult"},
		{name: "giant puppy", weightKg: 45, ageMonths: 20, want: "young"},
		{name: "giant adult", weightKg: 45, ageMonths: 21, want: "adult"},
		{name: "unknown weight uses medium", weightKg: 0, ageMonths: 11, want: "young"},
		{name: "unknown weight adult", weightKg: 0, ageMonths: 12, want: "adult"},
		{name: "mature", weightKg: 12, ageMonths: 36, want: "mature"},
		{name: "mature upper", weightKg: 12, ageMonths: 83, want: "mature"},
		{name: "senior", weightKg: 12, ageMonths: 84, want: "senior"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := LifeStageForAge(string(SpeciesDog), test.ageMonths, test.weightKg); got != test.want {
				t.Fatalf("dog %.1fkg %d months = %q, want %q", test.weightKg, test.ageMonths, got, test.want)
			}
		})
	}
}

func TestLifeStageBoundariesAreNotMutated(t *testing.T) {
	// 犬的分段边界会按体重调整，必须每次复制，不能改写共享表。
	before := dogStageBoundaries[1].minMonths
	LifeStageForAge(string(SpeciesDog), 6, 45)
	if dogStageBoundaries[1].minMonths != before {
		t.Fatalf("dog stage boundaries mutated: %d, want %d", dogStageBoundaries[1].minMonths, before)
	}
}

func TestLifeStageForAgeUnconfirmedInputs(t *testing.T) {
	if got := LifeStageForAge("", 60, 0); got != "" {
		t.Fatalf("unknown species = %q, want empty", got)
	}
	if got := LifeStageForAge(string(SpeciesCat), -1, 0); got != "" {
		t.Fatalf("unknown age = %q, want empty", got)
	}
}
