package family

type FamilySummaryDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FamilyDetailDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	JoinCode string `json:"join_code"`
	Role     string `json:"role"`
}

type FamilyMemberDTO struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type JoinApplicationDTO struct {
	ID         string `json:"id"`
	FamilyID   string `json:"family_id"`
	FamilyName string `json:"family_name"`
	UserID     string `json:"user_id"`
	Nickname   string `json:"nickname"`
	Avatar     string `json:"avatar"`
	Status     string `json:"status"`
}

type CreateFamilyRequest struct {
	Name string `json:"name" binding:"required,min=1,max=50"`
}

type JoinFamilyRequest struct {
	Code string `json:"code" binding:"required,min=4,max=32"`
}
