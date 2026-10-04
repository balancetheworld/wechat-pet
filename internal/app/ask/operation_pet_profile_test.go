package ask

import (
	"context"
	"testing"
)

type petProfileWrite struct {
	PetID    string
	Resource string
	Method   string
	Fields   map[string]any
}

type recordingPetProfileWriter struct {
	writes []petProfileWrite
}

func (w *recordingPetProfileWriter) Resource(_ context.Context, _, _, petID, resource, method string, payload map[string]any) (any, error) {
	w.writes = append(w.writes, petProfileWrite{PetID: petID, Resource: resource, Method: method, Fields: payload})
	return map[string]any{"id": petID}, nil
}

func confirmPetProfileOperation(t *testing.T, service *Service, familyID, userID, sessionID, runID string) Operation {
	t.Helper()
	operation, err := service.PrepareOperation(context.Background(), sessionID, runID, OperationPreviewInput{
		FamilyID: familyID,
		UserID:   userID,
		Target:   operationTargetPetProfileUpdate,
		Summary:  "修改宠物档案（pet-1）：breed=边牧",
		Payload:  map[string]any{"pet_id": "pet-1", "fields": map[string]any{"breed": "边牧"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.ConfirmOperation(context.Background(), familyID, userID, sessionID, operation.ID, operation.Version, operation.Preview, nil)
	if err != nil {
		t.Fatal(err)
	}
	return confirmed
}

func TestExecutePetProfileUpdateWithoutWriterFails(t *testing.T) {
	service, _ := newV2Service(t, &scriptedModel{}, &fakeBusinessRead{})
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "啾啾是边牧", "pet-writer-absent")
	if err != nil {
		t.Fatal(err)
	}
	confirmed := confirmPetProfileOperation(t, service, "family-1", "user-1", created.Session.ID, created.Run.ID)
	executed, err := service.ExecuteOperation(context.Background(), "family-1", "user-1", created.Session.ID, confirmed.ID, confirmed.Version)
	if err != nil {
		t.Fatal(err)
	}
	if executed.Status != OperationFailed || executed.Result != "operation_target_unavailable" {
		t.Fatalf("operation = %+v, want failed operation_target_unavailable", executed)
	}
}

func TestExecutePetProfileUpdateUsesInjectedWriter(t *testing.T) {
	service, _ := newV2Service(t, &scriptedModel{}, &fakeBusinessRead{})
	writer := &recordingPetProfileWriter{}
	service.SetPetProfileWriter(writer)
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "啾啾是边牧", "pet-writer-injected")
	if err != nil {
		t.Fatal(err)
	}
	confirmed := confirmPetProfileOperation(t, service, "family-1", "user-1", created.Session.ID, created.Run.ID)
	executed, err := service.ExecuteOperation(context.Background(), "family-1", "user-1", created.Session.ID, confirmed.ID, confirmed.Version)
	if err != nil {
		t.Fatal(err)
	}
	if executed.Status != OperationSucceeded {
		t.Fatalf("operation = %+v, want succeeded", executed)
	}
	if len(writer.writes) != 1 {
		t.Fatalf("writes = %+v, want 1", writer.writes)
	}
	write := writer.writes[0]
	if write.PetID != "pet-1" || write.Resource != "profile" || write.Method != "PATCH" || write.Fields["breed"] != "边牧" {
		t.Fatalf("write = %+v, want pet-1 profile PATCH breed=边牧", write)
	}
}
