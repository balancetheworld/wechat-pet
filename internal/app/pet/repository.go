package pet

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrNotFound = sql.ErrNoRows

type Pet struct {
	ID        string     `json:"id"`
	FamilyID  string     `json:"family_id"`
	Name      string     `json:"name"`
	CreatedBy string     `json:"created_by"`
	UpdatedBy string     `json:"updated_by"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type Repository interface {
	List(ctx context.Context, familyID string) ([]Pet, error)
	Get(ctx context.Context, familyID string, petID string) (Pet, error)
	Create(ctx context.Context, familyID string, userID string, name string) (Pet, error)
	Update(ctx context.Context, familyID string, petID string, userID string, request UpdatePetRequest) (Pet, error)
	Delete(ctx context.Context, familyID string, petID string, userID string) error
}

type SQLRepository struct {
	db     *sql.DB
	driver string
}

func NewRepository(db *sql.DB, driver string) (*SQLRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLRepository{db: db, driver: strings.ToLower(driver)}, nil
}

func (r *SQLRepository) List(ctx context.Context, familyID string) ([]Pet, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, family_id, name, created_by, updated_by, created_at, updated_at, deleted_at FROM pets WHERE family_id = ? AND deleted_at IS NULL ORDER BY created_at, id"), familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Pet, 0)
	for rows.Next() {
		value, err := scanPet(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) Get(ctx context.Context, familyID string, petID string) (Pet, error) {
	row := r.db.QueryRowContext(ctx, r.query("SELECT id, family_id, name, created_by, updated_by, created_at, updated_at, deleted_at FROM pets WHERE id = ? AND family_id = ? AND deleted_at IS NULL"), petID, familyID)
	return scanPet(row)
}

func (r *SQLRepository) Create(ctx context.Context, familyID string, userID string, name string) (Pet, error) {
	id, err := newID()
	if err != nil {
		return Pet{}, err
	}
	if _, err := r.db.ExecContext(ctx, r.query("INSERT INTO pets (id, family_id, name, created_by, updated_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"), id, familyID, name, userID, userID); err != nil {
		return Pet{}, err
	}
	return r.Get(ctx, familyID, id)
}

func (r *SQLRepository) Update(ctx context.Context, familyID string, petID string, userID string, request UpdatePetRequest) (Pet, error) {
	/* 日期为空字符串时写 NULL (数据库列为可空 DATE), 否则原样传入由 PG 解析 YYYY-MM-DD */
	birthday := nullableDate(request.Birthday)
	homeDate := nullableDate(request.HomeDate)
	result, err := r.db.ExecContext(ctx, r.query("UPDATE pets SET name = ?, breed = ?, gender = ?, sterilized = ?, birthday = ?, home_date = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND family_id = ? AND deleted_at IS NULL"), request.Name, request.Breed, request.Gender, request.Sterilized, birthday, homeDate, userID, petID, familyID)
	if err != nil {
		return Pet{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Pet{}, err
	}
	if affected == 0 {
		return Pet{}, ErrNotFound
	}
	return r.Get(ctx, familyID, petID)
}

/* nullableDate: 空串/空白 → nil (SQL NULL), 否则返回去除空白后的字符串 */
func nullableDate(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func (r *SQLRepository) Delete(ctx context.Context, familyID string, petID string, userID string) error {
	result, err := r.db.ExecContext(ctx, r.query("UPDATE pets SET deleted_at = CURRENT_TIMESTAMP, updated_by = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND family_id = ? AND deleted_at IS NULL"), userID, petID, familyID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPet(value scanner) (Pet, error) {
	var result Pet
	var deletedAt sql.NullTime
	if err := value.Scan(&result.ID, &result.FamilyID, &result.Name, &result.CreatedBy, &result.UpdatedBy, &result.CreatedAt, &result.UpdatedAt, &deletedAt); err != nil {
		return Pet{}, err
	}
	if deletedAt.Valid {
		result.DeletedAt = &deletedAt.Time
	}
	return result, nil
}

func (r *SQLRepository) query(value string) string {
	if !r.isPostgres() {
		return value
	}
	var result strings.Builder
	index := 0
	for _, char := range value {
		if char != '?' {
			result.WriteRune(char)
			continue
		}
		index++
		result.WriteByte('$')
		result.WriteString(strconv.Itoa(index))
	}
	return result.String()
}

func (r *SQLRepository) isPostgres() bool {
	return r.driver == "postgres" || r.driver == "postgresql" || r.driver == "pgx"
}

func newID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate pet id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
