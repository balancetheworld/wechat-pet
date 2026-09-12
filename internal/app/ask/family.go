package ask

import petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"

type FamilyPetItem struct {
	PetID   string `json:"pet_id"`
	PetName string `json:"pet_name"`
}

func buildFamilyPetsResult(pets []petapp.Pet) map[string]any {
	items := make([]FamilyPetItem, 0, len(pets))
	for _, pet := range pets {
		items = append(items, FamilyPetItem{PetID: pet.ID, PetName: pet.Name})
	}
	return map[string]any{"pets": items}
}
