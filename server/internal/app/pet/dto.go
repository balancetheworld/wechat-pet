package pet

type PetDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreatePetRequest struct {
	Name string `json:"name" binding:"required,min=1,max=50"`
}

type UpdatePetRequest struct {
	Name string `json:"name" binding:"required,min=1,max=50"`
}
