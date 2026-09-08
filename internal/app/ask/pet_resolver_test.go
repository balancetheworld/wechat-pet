package ask

import (
	"testing"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
)

func TestResolvePetsSupportsMultipleNames(t *testing.T) {
	pets := []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "球球"}, {ID: "pet-3", Name: "豆豆"}}
	result := ResolvePets("旺仔和球球上次洗澡分别是什么时候", pets)
	if result.Status != PetResolveResolved || len(result.Resolved) != 2 || result.Resolved[0].ID != "pet-1" || result.Resolved[1].ID != "pet-2" {
		t.Fatalf("resolution = %+v", result)
	}
}

func TestResolvePetsReportsDuplicateName(t *testing.T) {
	pets := []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "旺仔"}, {ID: "pet-3", Name: "球球"}}
	result := ResolvePets("旺仔和球球上次洗澡是什么时候", pets)
	if result.Status != PetResolveAmbiguous || len(result.Resolved) != 1 || result.Resolved[0].ID != "pet-3" || len(result.Ambiguous) != 1 || len(result.Ambiguous[0].Pets) != 2 {
		t.Fatalf("resolution = %+v", result)
	}
}

func TestResolvePetsReturnsNoneWhenNameMissing(t *testing.T) {
	result := ResolvePets("上次洗澡是什么时候", []petapp.Pet{{ID: "pet-1", Name: "旺仔"}})
	if result.Status != PetResolveNone || len(result.Resolved) != 0 || len(result.Ambiguous) != 0 {
		t.Fatalf("resolution = %+v", result)
	}
}

func TestResolvePetsPrefersLongerName(t *testing.T) {
	pets := []petapp.Pet{{ID: "pet-1", Name: "球"}, {ID: "pet-2", Name: "球球"}}
	result := ResolvePets("球球上次洗澡是什么时候", pets)
	if result.Status != PetResolveResolved || len(result.Resolved) != 1 || result.Resolved[0].ID != "pet-2" {
		t.Fatalf("resolution = %+v", result)
	}
}

func TestResolvePetsKeepsExplicitShortName(t *testing.T) {
	pets := []petapp.Pet{{ID: "pet-1", Name: "球"}, {ID: "pet-2", Name: "球球"}}
	result := ResolvePets("球和球球上次洗澡是什么时候", pets)
	if result.Status != PetResolveResolved || len(result.Resolved) != 2 || result.Resolved[0].ID != "pet-1" || result.Resolved[1].ID != "pet-2" {
		t.Fatalf("resolution = %+v", result)
	}
}
