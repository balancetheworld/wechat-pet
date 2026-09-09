package ask

import (
	"sort"
	"strings"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
)

type PetResolveStatus string

const (
	PetResolveNone      PetResolveStatus = "none"
	PetResolveResolved  PetResolveStatus = "resolved"
	PetResolveAmbiguous PetResolveStatus = "ambiguous"
)

type PetCandidate struct {
	Name string
	Pets []petapp.Pet
}

type PetResolution struct {
	Status    PetResolveStatus
	Resolved  []petapp.Pet
	Ambiguous []PetCandidate
	Input     string
}

func ResolvePets(input string, pets []petapp.Pet) PetResolution {
	input = strings.TrimSpace(input)
	result := PetResolution{Status: PetResolveNone, Input: input}
	if input == "" || len(pets) == 0 {
		return result
	}
	groups := make(map[string][]petapp.Pet)
	for _, pet := range pets {
		name := strings.TrimSpace(pet.Name)
		if name == "" || !strings.Contains(input, name) || containsLongerMatchedName(input, name, pets) {
			continue
		}
		groups[name] = append(groups[name], pet)
	}
	if len(groups) == 0 {
		return result
	}
	for name, values := range groups {
		if len(values) > 1 {
			result.Ambiguous = append(result.Ambiguous, PetCandidate{Name: name, Pets: values})
			continue
		}
		result.Resolved = append(result.Resolved, values[0])
	}
	sort.SliceStable(result.Resolved, func(i, j int) bool {
		return strings.Index(input, result.Resolved[i].Name) < strings.Index(input, result.Resolved[j].Name)
	})
	sort.SliceStable(result.Ambiguous, func(i, j int) bool {
		return strings.Index(input, result.Ambiguous[i].Name) < strings.Index(input, result.Ambiguous[j].Name)
	})
	if len(result.Ambiguous) > 0 {
		result.Status = PetResolveAmbiguous
	} else if len(result.Resolved) > 0 {
		result.Status = PetResolveResolved
	}
	return result
}

func containsLongerMatchedName(input, name string, pets []petapp.Pet) bool {
	shortPositions := matchPositions(input, name)
	if len(shortPositions) == 0 {
		return false
	}
	for _, pet := range pets {
		other := strings.TrimSpace(pet.Name)
		if other == "" || other == name || len([]rune(other)) <= len([]rune(name)) || !strings.Contains(other, name) {
			continue
		}
		for _, longPosition := range matchPositions(input, other) {
			longEnd := longPosition + len(other)
			covered := true
			for _, shortPosition := range shortPositions {
				if shortPosition < longPosition || shortPosition+len(name) > longEnd {
					covered = false
					break
				}
			}
			if covered {
				return true
			}
		}
	}
	return false
}

func matchPositions(input, value string) []int {
	result := make([]int, 0)
	for offset := 0; offset < len(input); {
		index := strings.Index(input[offset:], value)
		if index < 0 {
			break
		}
		position := offset + index
		result = append(result, position)
		offset = position + len(value)
	}
	return result
}
