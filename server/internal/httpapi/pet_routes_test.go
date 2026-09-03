package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
)

func TestPetRoutesCompleteCRUDFlow(t *testing.T) {
	familyRepository, db := newFamilyRouteRepository(t)
	familyService, err := familyapp.NewService(familyRepository)
	if err != nil {
		t.Fatal(err)
	}
	seedFamilyRouteUser(t, db, "owner-pet", "宠物家庭")
	family, err := familyService.Create(context.Background(), "owner-pet", familyapp.CreateFamilyRequest{Name: "宠物家庭"})
	if err != nil {
		t.Fatal(err)
	}
	petRepository, err := petapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	petService, err := petapp.NewService(petRepository)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewWithDependencies(Dependencies{FamilyRepository: familyRepository, FamilyService: familyService, PetRepository: petRepository, PetService: petService, TokenSigner: signer})

	create := petRouteRequest(t, router, signer, http.MethodPost, "/api/v1/pets", "owner-pet", `{"name":"小白"}`)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `"name":"小白"`) {
		t.Fatalf("create response: status=%d body=%s", create.Code, create.Body.String())
	}
	petID := extractPetID(create.Body.String())
	if petID == "" {
		t.Fatal("pet ID not returned")
	}

	list := petRouteRequest(t, router, signer, http.MethodGet, "/api/v1/pets", "owner-pet", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), petID) {
		t.Fatalf("list response: status=%d body=%s", list.Code, list.Body.String())
	}
	update := petRouteRequest(t, router, signer, http.MethodPatch, "/api/v1/pets/"+petID, "owner-pet", `{"name":"小黑"}`)
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"name":"小黑"`) {
		t.Fatalf("update response: status=%d body=%s", update.Code, update.Body.String())
	}
	remove := petRouteRequest(t, router, signer, http.MethodDelete, "/api/v1/pets/"+petID, "owner-pet", "")
	if remove.Code != http.StatusOK {
		t.Fatalf("delete response: status=%d body=%s", remove.Code, remove.Body.String())
	}
	list = petRouteRequest(t, router, signer, http.MethodGet, "/api/v1/pets", "owner-pet", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), petID) {
		t.Fatalf("list after delete response: status=%d body=%s", list.Code, list.Body.String())
	}

	guest := petRouteRequest(t, router, signer, http.MethodGet, "/api/v1/pets", "unknown-user", "")
	if guest.Code != http.StatusForbidden || !strings.Contains(guest.Body.String(), `"code":40301`) {
		t.Fatalf("guest response: status=%d body=%s", guest.Code, guest.Body.String())
	}
	_ = family
}

func petRouteRequest(t *testing.T, router http.Handler, signer *jwtpkg.Signer, method string, path string, userID string, body string) *httptest.ResponseRecorder {
	t.Helper()
	token, err := signer.Sign(userID)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func extractPetID(body string) string {
	const marker = `"id":"`
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.IndexByte(body[start:], '"')
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}
