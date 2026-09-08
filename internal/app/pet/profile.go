package pet

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

type PetProfile struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	AvatarAssetID    string  `json:"avatar_asset_id"`
	CoverAssetID     string  `json:"cover_asset_id"`
	Breed            string  `json:"breed"`
	Gender           string  `json:"gender"`
	Sterilized       bool    `json:"sterilized"`
	Birthday         *string `json:"birthday"`
	HomeDate         *string `json:"home_date"`
	Age              int     `json:"age"`
	CompanionDays    int     `json:"companion_days"`
	NextBirthdayDays *int    `json:"next_birthday_days"`
}

type PetHealth struct {
	Status             string `json:"status"`
	Allergies          string `json:"allergies"`
	LongTermMedication string `json:"long_term_medication"`
}

type ProfileRepository interface {
	GetProfile(context.Context, string, string) (PetProfile, error)
	Resource(context.Context, string, string, string, string, map[string]any) (any, error)
}

func (s *Service) Profile(ctx context.Context, familyID, petID string) (PetProfile, error) {
	r, ok := s.repository.(ProfileRepository)
	if !ok {
		return PetProfile{}, appErrors.Internal(errors.New("pet profile repository unavailable"))
	}
	v, err := r.GetProfile(ctx, familyID, petID)
	if err != nil {
		return PetProfile{}, mapError(err)
	}
	return v, nil
}

func (s *Service) Resource(ctx context.Context, familyID, userID, petID, resource, method string, payload map[string]any) (any, error) {
	r, ok := s.repository.(ProfileRepository)
	if !ok {
		return nil, appErrors.Internal(errors.New("pet profile repository unavailable"))
	}
	if method == "PATCH" && resource == "profile" {
		for _, key := range []string{"avatar_asset_id", "cover_asset_id"} {
			if value, exists := payload[key].(string); exists {
				if err := s.authorizeAsset(ctx, value, familyID); err != nil {
					return nil, err
				}
			}
		}
	}
	if (method == "POST" || method == "PATCH") && (resource == "birthday-media" || resource == "growth-media") {
		if value, exists := payload["asset_id"].(string); exists {
			if err := s.authorizeAsset(ctx, value, familyID); err != nil {
				return nil, err
			}
		}
	}
	if method == "POST" || method == "PATCH" {
		if resource == "birthday-blessings" {
			payload["user_id"] = userID
		}
		if resource == "growth-events" {
			payload["recorder"] = userID
		}
	}
	if err := validateResourcePayload(resource, method, payload); err != nil {
		return nil, err
	}
	v, err := r.Resource(ctx, familyID, petID, resource, method, payload)
	if err != nil {
		return nil, mapError(err)
	}
	return v, nil
}

func (r *SQLRepository) GetProfile(ctx context.Context, familyID, petID string) (PetProfile, error) {
	var p PetProfile
	var birthday, home sql.NullString
	err := r.db.QueryRowContext(ctx, r.query("SELECT id,name,COALESCE(avatar_asset_id,''),COALESCE(cover_asset_id,''),COALESCE(breed,''),COALESCE(gender,''),sterilized,birthday,home_date FROM pets WHERE id=? AND family_id=? AND deleted_at IS NULL"), petID, familyID).Scan(&p.ID, &p.Name, &p.AvatarAssetID, &p.CoverAssetID, &p.Breed, &p.Gender, &p.Sterilized, &birthday, &home)
	if err != nil {
		return p, err
	}
	if birthday.Valid {
		p.Birthday = &birthday.String
	}
	if home.Valid {
		p.HomeDate = &home.String
	}
	now := time.Now()
	if p.Birthday != nil {
		if d, e := time.Parse("2006-01-02", *p.Birthday); e == nil {
			p.Age = now.Year() - d.Year()
			if now.YearDay() < d.YearDay() {
				p.Age--
			}
			next := time.Date(now.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
			if !next.After(now) {
				next = next.AddDate(1, 0, 0)
			}
			days := int(next.Sub(now).Hours() / 24)
			p.NextBirthdayDays = &days
		}
	}
	if p.HomeDate != nil {
		if d, e := time.Parse("2006-01-02", *p.HomeDate); e == nil {
			p.CompanionDays = int(now.Sub(d).Hours() / 24)
			if p.CompanionDays < 0 {
				p.CompanionDays = 0
			}
		}
	}
	return p, nil
}

func (r *SQLRepository) GetHealth(ctx context.Context, familyID, petID string) (PetHealth, error) {
	var value PetHealth
	err := r.db.QueryRowContext(ctx, r.query("SELECT status, allergies, long_term_medication FROM pet_health WHERE pet_id = ? AND family_id = ?"), petID, familyID).Scan(&value.Status, &value.Allergies, &value.LongTermMedication)
	if errors.Is(err, sql.ErrNoRows) {
		return PetHealth{}, nil
	}
	return value, err
}

func (r *SQLRepository) Resource(ctx context.Context, familyID, petID, resource, method string, payload map[string]any) (any, error) {
	if _, err := r.Get(ctx, familyID, petID); err != nil {
		return nil, err
	}
	if resource == "profile" {
		if method != "PATCH" {
			return r.GetProfile(ctx, familyID, petID)
		}
		allowed := []string{"name", "avatar_asset_id", "cover_asset_id", "breed", "gender", "sterilized", "birthday", "home_date"}
		sets := []string{}
		args := []any{}
		for _, k := range allowed {
			if v, ok := payload[k]; ok {
				sets = append(sets, k+"=?")
				args = append(args, v)
			}
		}
		if len(sets) == 0 {
			return nil, appErrors.InvalidParam("没有可更新字段")
		}
		args = append(args, petID, familyID)
		result, e := r.db.ExecContext(ctx, r.query("UPDATE pets SET "+strings.Join(sets, ",")+",updated_at=CURRENT_TIMESTAMP WHERE id=? AND family_id=?"), args...)
		if e != nil {
			return nil, e
		}
		if affected, e := result.RowsAffected(); e != nil || affected == 0 {
			if e != nil {
				return nil, e
			}
			return nil, sql.ErrNoRows
		}
		return r.GetProfile(ctx, familyID, petID)
	}
	if resource == "dates" {
		p, e := r.GetProfile(ctx, familyID, petID)
		if e != nil {
			return nil, e
		}
		return map[string]any{"birthday": p.Birthday, "home_date": p.HomeDate, "age": p.Age, "companion_days": p.CompanionDays, "next_birthday_days": p.NextBirthdayDays}, nil
	}
	if resource == "health" {
		return r.health(ctx, familyID, petID, method, payload)
	}
	table, fields, err := resourceSpec(resource)
	if err != nil {
		return nil, err
	}
	if method == "GET" {
		rows, e := r.db.QueryContext(ctx, r.query("SELECT id,"+fields+" FROM "+table+" WHERE pet_id=? AND family_id=? ORDER BY created_at,id"), petID, familyID)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		out := []map[string]any{}
		parts := strings.Split(fields, ",")
		for rows.Next() {
			vals := make([]any, len(parts)+1)
			ptrs := make([]any, len(vals))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if e := rows.Scan(ptrs...); e != nil {
				return nil, e
			}
			m := map[string]any{"id": vals[0]}
			for i, k := range parts {
				m[k] = vals[i+1]
			}
			out = append(out, m)
		}
		return out, rows.Err()
	}
	if method == "POST" {
		id, _ := newID()
		cols := []string{"id", "pet_id", "family_id"}
		args := []any{id, petID, familyID}
		for _, f := range strings.Split(fields, ",") {
			v, ok := payload[f]
			if !ok {
				return nil, appErrors.InvalidParam("缺少字段: " + f)
			}
			cols = append(cols, f)
			args = append(args, v)
		}
		qs := make([]string, len(cols))
		for i := range qs {
			qs[i] = "?"
		}
		_, e := r.db.ExecContext(ctx, r.query("INSERT INTO "+table+" ("+strings.Join(cols, ",")+") VALUES ("+strings.Join(qs, ",")+")"), args...)
		if e != nil {
			return nil, e
		}
		payload["id"] = id
		return payload, nil
	}
	if method == "PATCH" || method == "DELETE" {
		id, ok := payload["id"].(string)
		if !ok || id == "" {
			return nil, appErrors.InvalidParam("档案 ID 不能为空")
		}
		if method == "DELETE" {
			result, e := r.db.ExecContext(ctx, r.query("DELETE FROM "+table+" WHERE id=? AND pet_id=? AND family_id=?"), id, petID, familyID)
			if e != nil {
				return nil, e
			}
			if affected, e := result.RowsAffected(); e != nil || affected == 0 {
				if e != nil {
					return nil, e
				}
				return nil, sql.ErrNoRows
			}
			return map[string]any{}, nil
		}
		sets := []string{}
		args := []any{}
		for _, f := range strings.Split(fields, ",") {
			if v, ok := payload[f]; ok {
				sets = append(sets, f+"=?")
				args = append(args, v)
			}
		}
		if len(sets) == 0 {
			return nil, appErrors.InvalidParam("没有可更新字段")
		}
		args = append(args, id, petID, familyID)
		result, e := r.db.ExecContext(ctx, r.query("UPDATE "+table+" SET "+strings.Join(sets, ",")+",updated_at=CURRENT_TIMESTAMP WHERE id=? AND pet_id=? AND family_id=?"), args...)
		if e != nil {
			return nil, e
		}
		if affected, e := result.RowsAffected(); e != nil || affected == 0 {
			if e != nil {
				return nil, e
			}
			return nil, sql.ErrNoRows
		}
		return payload, nil
	}
	return nil, appErrors.InvalidParam("不支持的操作")
}

func resourceSpec(resource string) (string, string, error) {
	switch resource {
	case "certificates":
		return "pet_certificates", "type,name,number", nil
	case "personality":
		return "pet_personality", "trait,value", nil
	case "questions":
		return "pet_questions", "question,answer", nil
	case "diseases":
		return "pet_diseases", "name,status,details", nil
	case "vaccines":
		return "pet_vaccines", "name,vaccinated_at,details", nil
	case "birthday-records":
		return "pet_birthday_records", "year,age,summary", nil
	case "birthday-media":
		return "pet_birthday_media", "record_id,type,asset_id", nil
	case "birthday-blessings":
		return "pet_birthday_blessings", "record_id,user_id,content", nil
	case "weights":
		return "pet_weights", "measured_at,weight", nil
	case "growth-events":
		return "pet_growth_events", "type,occurred_at,recorder,content", nil
	case "growth-media":
		return "pet_growth_media", "event_id,asset_id", nil
	}
	return "", "", appErrors.InvalidParam("未知档案资源")
}

func validateResourcePayload(resource, method string, payload map[string]any) error {
	if resource == "profile" {
		for _, field := range []string{"name", "avatar_asset_id", "cover_asset_id", "breed", "gender"} {
			if value, ok := payload[field]; ok {
				if _, ok := value.(string); !ok {
					return appErrors.InvalidParam(field + " 格式无效")
				}
			}
		}
		if value, ok := payload["sterilized"]; ok {
			if _, ok := value.(bool); !ok {
				return appErrors.InvalidParam("sterilized 格式无效")
			}
		}
		for _, field := range []string{"birthday", "home_date"} {
			if value, ok := payload[field]; ok && value != nil {
				date, ok := value.(string)
				if !ok {
					return appErrors.InvalidParam(field + " 格式无效")
				}
				if _, err := time.Parse("2006-01-02", date); err != nil {
					return appErrors.InvalidParam(field + " 格式应为 YYYY-MM-DD")
				}
			}
		}
		return nil
	}
	if resource == "health" {
		for _, field := range []string{"status", "allergies", "long_term_medication"} {
			if value, ok := payload[field]; ok {
				if _, ok := value.(string); !ok {
					return appErrors.InvalidParam(field + " 格式无效")
				}
			}
		}
		return nil
	}
	if method == "GET" || method == "DELETE" {
		return nil
	}
	for _, field := range []string{"record_id", "event_id", "asset_id", "user_id", "type", "name", "number", "trait", "value", "question", "answer", "status", "details", "summary", "content", "recorder"} {
		if value, ok := payload[field]; ok {
			if _, ok := value.(string); !ok {
				return appErrors.InvalidParam(field + " 格式无效")
			}
		}
	}
	for _, field := range []string{"vaccinated_at", "measured_at", "occurred_at"} {
		if value, ok := payload[field]; ok {
			date, ok := value.(string)
			if !ok {
				return appErrors.InvalidParam(field + " 格式无效")
			}
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return appErrors.InvalidParam(field + " 格式应为 YYYY-MM-DD")
			}
		}
	}
	for _, field := range []string{"year", "age", "weight"} {
		if value, ok := payload[field]; ok {
			if _, ok := value.(float64); !ok {
				return appErrors.InvalidParam(field + " 格式无效")
			}
		}
	}
	return nil
}

func (r *SQLRepository) health(ctx context.Context, familyID, petID, method string, p map[string]any) (any, error) {
	if method == "GET" {
		var status, allergy, med string
		e := r.db.QueryRowContext(ctx, r.query("SELECT status,allergies,long_term_medication FROM pet_health WHERE pet_id=? AND family_id=?"), petID, familyID).Scan(&status, &allergy, &med)
		if errors.Is(e, sql.ErrNoRows) {
			return map[string]any{"status": "", "allergies": "", "long_term_medication": ""}, nil
		}
		return map[string]any{"status": status, "allergies": allergy, "long_term_medication": med}, e
	}
	if method != "PUT" {
		return nil, appErrors.InvalidParam("不支持的操作")
	}
	status, _ := p["status"].(string)
	allergies, _ := p["allergies"].(string)
	med, _ := p["long_term_medication"].(string)
	_, e := r.db.ExecContext(ctx, r.query("INSERT INTO pet_health(pet_id,family_id,status,allergies,long_term_medication) VALUES(?,?,?,?,?) ON CONFLICT(pet_id) DO UPDATE SET status=excluded.status,allergies=excluded.allergies,long_term_medication=excluded.long_term_medication,updated_at=CURRENT_TIMESTAMP"), petID, familyID, status, allergies, med)
	return p, e
}

var _ ProfileRepository = (*SQLRepository)(nil)

func (r *SQLRepository) SetAvatar(ctx context.Context, familyID, petID, assetID string) error {
	_, err := r.db.ExecContext(ctx, r.query("UPDATE pets SET avatar_asset_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND family_id=?"), assetID, petID, familyID)
	return err
}
