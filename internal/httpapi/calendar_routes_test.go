package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
)

func TestCalendarRoutesRecordReminderFlow(t *testing.T) {
	familyRepository, db := newFamilyRouteRepository(t)
	if _, err := db.Exec("ALTER TABLE pets ADD COLUMN avatar_asset_id TEXT"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(calendarRouteSchema); err != nil {
		t.Fatal(err)
	}
	familyService, err := familyapp.NewService(familyRepository)
	if err != nil {
		t.Fatal(err)
	}
	seedFamilyRouteUser(t, db, "owner-calendar", "日历成员")
	family, err := familyService.Create(context.Background(), "owner-calendar", familyapp.CreateFamilyRequest{Name: "日历家庭"})
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
	pet, err := petService.Create(context.Background(), family.ID, "owner-calendar", petapp.CreatePetRequest{Name: "团子"})
	if err != nil {
		t.Fatal(err)
	}
	calendarRepository, err := calendarapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	calendarService, err := calendarapp.NewService(calendarRepository)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewWithDependencies(Dependencies{FamilyRepository: familyRepository, FamilyService: familyService, PetRepository: petRepository, PetService: petService, CalendarService: calendarService, TokenSigner: signer})

	create := petRouteRequest(t, router, signer, http.MethodPost, "/api/v1/calendar/records", "owner-calendar", `{"category":"medical","medical_type":"vaccine","pet_id":"`+pet.ID+`","content":"接种狂犬疫苗","occurred_at":"2026-09-03T15:30:00+08:00","reminder":{"reminder_date":"2026-09-20","repeat_type":"yearly","advance_days":3,"notification_channels":["in_app"]}}`)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `"category":"medical"`) || !strings.Contains(create.Body.String(), `"reminder_date":"2026-09-20"`) {
		t.Fatalf("create response: status=%d body=%s", create.Code, create.Body.String())
	}
	reminderID := extractCalendarReminderID(create.Body.String())
	if reminderID == "" {
		t.Fatalf("reminder ID not returned: %s", create.Body.String())
	}

	month := petRouteRequest(t, router, signer, http.MethodGet, "/api/v1/calendar/months/2026-09?pet_id="+pet.ID, "owner-calendar", "")
	if month.Code != http.StatusOK || !strings.Contains(month.Body.String(), `"date":"2026-09-03"`) || !strings.Contains(month.Body.String(), `"has_medical_record":true`) || !strings.Contains(month.Body.String(), `"date":"2026-09-20"`) || !strings.Contains(month.Body.String(), `"has_pending_reminder":true`) {
		t.Fatalf("month response: status=%d body=%s", month.Code, month.Body.String())
	}

	day := petRouteRequest(t, router, signer, http.MethodGet, "/api/v1/calendar/days/2026-09-03", "owner-calendar", "")
	if day.Code != http.StatusOK || !strings.Contains(day.Body.String(), `"record_count":1`) || !strings.Contains(day.Body.String(), `"nickname":"日历成员"`) {
		t.Fatalf("day response: status=%d body=%s", day.Code, day.Body.String())
	}

	complete := petRouteRequest(t, router, signer, http.MethodPost, "/api/v1/calendar/reminders/"+reminderID+"/complete", "owner-calendar", `{"completed_at":"2026-09-20T10:00:00+08:00","content":"完成年度疫苗接种"}`)
	if complete.Code != http.StatusOK || !strings.Contains(complete.Body.String(), `"status":"completed"`) || !strings.Contains(complete.Body.String(), `"reminder_date":"2027-09-20"`) {
		t.Fatalf("complete response: status=%d body=%s", complete.Code, complete.Body.String())
	}

	repeated := petRouteRequest(t, router, signer, http.MethodPost, "/api/v1/calendar/reminders/"+reminderID+"/complete", "owner-calendar", "")
	if repeated.Code != http.StatusConflict || !strings.Contains(repeated.Body.String(), `"code":40901`) {
		t.Fatalf("repeated completion response: status=%d body=%s", repeated.Code, repeated.Body.String())
	}

	multiple := petRouteRequest(t, router, signer, http.MethodPost, "/api/v1/calendar/records", "owner-calendar", `{"category":"medical","medical_type":"other","custom_medical_type":"过敏复查","pet_id":"`+pet.ID+`","content":"皮肤过敏复查","occurred_at":"2026-09-03T16:00:00+08:00","reminders":[{"reminder_date":"2026-09-22","repeat_type":"once","advance_days":3,"notification_channels":["in_app"]},{"reminder_date":"2026-10-22","repeat_type":"monthly","advance_days":3,"notification_channels":["in_app"]}]}`)
	if multiple.Code != http.StatusOK || !strings.Contains(multiple.Body.String(), `"custom_medical_type":"过敏复查"`) || !strings.Contains(multiple.Body.String(), `"reminders":[`) || !strings.Contains(multiple.Body.String(), `"reminder_date":"2026-09-22"`) || !strings.Contains(multiple.Body.String(), `"reminder_date":"2026-10-22"`) {
		t.Fatalf("multiple reminders response: status=%d body=%s", multiple.Code, multiple.Body.String())
	}

	emptyMedical := petRouteRequest(t, router, signer, http.MethodPost, "/api/v1/calendar/records", "owner-calendar", `{"category":"medical","medical_type":"checkup","pet_id":"`+pet.ID+`","occurred_at":"2026-09-03T17:00:00+08:00"}`)
	if emptyMedical.Code != http.StatusOK {
		t.Fatalf("empty medical response: status=%d body=%s", emptyMedical.Code, emptyMedical.Body.String())
	}

	emptyDaily := petRouteRequest(t, router, signer, http.MethodPost, "/api/v1/calendar/records", "owner-calendar", `{"category":"daily","pet_id":"`+pet.ID+`","occurred_at":"2026-09-03T18:00:00+08:00"}`)
	if emptyDaily.Code != http.StatusBadRequest || !strings.Contains(emptyDaily.Body.String(), "记录内容和图片至少填写一项") {
		t.Fatalf("empty daily response: status=%d body=%s", emptyDaily.Code, emptyDaily.Body.String())
	}

	multipleMonth := petRouteRequest(t, router, signer, http.MethodGet, "/api/v1/calendar/months/2026-10?pet_id="+pet.ID, "owner-calendar", "")
	if multipleMonth.Code != http.StatusOK || !strings.Contains(multipleMonth.Body.String(), `"date":"2026-10-22"`) || !strings.Contains(multipleMonth.Body.String(), `"has_pending_reminder":true`) {
		t.Fatalf("multiple reminders month response: status=%d body=%s", multipleMonth.Code, multipleMonth.Body.String())
	}
}

func extractCalendarReminderID(body string) string {
	const marker = `"reminder":{"id":"`
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

const calendarRouteSchema = `CREATE TABLE calendar_records (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, category TEXT NOT NULL, medical_type TEXT, custom_medical_type TEXT NOT NULL DEFAULT '', content TEXT NOT NULL, occurred_at TIMESTAMP NOT NULL, occurred_on DATE NOT NULL, created_by TEXT NOT NULL, updated_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP);
CREATE TABLE calendar_record_media (id TEXT PRIMARY KEY, record_id TEXT NOT NULL, family_id TEXT NOT NULL, asset_id TEXT NOT NULL, sort_order INTEGER NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE calendar_reminders (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, source_record_id TEXT NOT NULL, previous_reminder_id TEXT, reminder_date DATE NOT NULL, repeat_type TEXT NOT NULL, repeat_interval_days INTEGER, advance_days INTEGER NOT NULL, notification_channels TEXT NOT NULL, status TEXT NOT NULL, completed_at TIMESTAMP, completed_by TEXT, completed_record_id TEXT, created_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);`
