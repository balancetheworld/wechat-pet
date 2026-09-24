package ask

import (
	"context"
	"database/sql"
	"testing"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	_ "github.com/mattn/go-sqlite3"
)

// stubAgentModel 是最小 AgentModel 替身，仅用于 setter 注入测试。
type stubAgentModel struct{}

func (stubAgentModel) Step(context.Context, StepInput) (ModelStepResult, error) {
	return ModelStepResult{}, nil
}

func TestServiceSettersInjectV2Dependencies(t *testing.T) {
	var s Service
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	model := stubAgentModel{}
	business := &fakeBusinessRead{}

	s.SetAgentModel(model)
	s.SetToolCatalog(catalog)
	s.SetBusinessReadRepository(business)

	if s.agentModel == nil {
		t.Fatal("SetAgentModel did not inject the model")
	}
	if s.catalog != catalog {
		t.Fatal("SetToolCatalog did not inject the catalog")
	}
	if s.businessRead == nil {
		t.Fatal("SetBusinessReadRepository did not inject the repository")
	}
}

// TestServiceV2DependenciesNilByDefault 未注入时 v2 依赖为 nil，processRun 保持 v1 路径不受影响。
func TestServiceV2DependenciesNilByDefault(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}})
	if err != nil {
		t.Fatal(err)
	}
	if service.agentModel != nil {
		t.Fatal("agentModel should be nil by default")
	}
	if service.catalog != nil {
		t.Fatal("catalog should be nil by default")
	}
	if service.businessRead != nil {
		t.Fatal("businessRead should be nil by default")
	}
}
