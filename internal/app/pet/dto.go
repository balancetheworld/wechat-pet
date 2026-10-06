package pet

type PetDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreatePetRequest struct {
	Name          string `json:"name" binding:"required,min=1,max=50"`
	AvatarAssetID string `json:"avatar_asset_id"`
	Breed         string `json:"breed"`
	Species       string `json:"species"`
	Gender        string `json:"gender"`
	Sterilized    bool   `json:"sterilized"`
	Birthday      string `json:"birthday"`
	HomeDate      string `json:"home_date"`
}

type UpdatePetRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=50"`
	Breed      string `json:"breed"`
	Species    string `json:"species"`
	Gender     string `json:"gender"`
	Sterilized bool   `json:"sterilized"`
	Birthday   string `json:"birthday"`
	HomeDate   string `json:"home_date"`
}
