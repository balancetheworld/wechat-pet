package family

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Summary struct {
	ID   string
	Name string
	Role string
}

type Repository interface {
	GetActiveFamilySummary(ctx context.Context, userID string) (*Summary, error)
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

func (r *SQLRepository) GetActiveFamilySummary(ctx context.Context, userID string) (*Summary, error) {
	query := "SELECT f.id, f.name, fm.role FROM family_members fm JOIN families f ON f.id = fm.family_id WHERE fm.user_id = ? AND fm.status = 'active' ORDER BY f.created_at LIMIT 1"
	if r.driver == "postgres" || r.driver == "postgresql" || r.driver == "pgx" {
		query = strings.Replace(query, "?", "$1", 1)
	}
	var result Summary
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&result.ID, &result.Name, &result.Role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}
