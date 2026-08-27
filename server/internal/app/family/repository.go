package family

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrNotFound             = errors.New("family resource not found")
	ErrAlreadyInFamily      = errors.New("user already has an active family")
	ErrApplicationProcessed = errors.New("join application has already been processed")
	ErrCannotRemoveSelf     = errors.New("owner cannot remove self")
)

type Summary struct {
	ID   string
	Name string
	Role string
}

type Detail struct {
	ID   string
	Name string
	Code string
	Role string
}

type Member struct {
	ID       string
	UserID   string
	Nickname string
	Avatar   string
	Role     string
	Status   string
}

type JoinApplication struct {
	ID         string
	FamilyID   string
	FamilyName string
	UserID     string
	Nickname   string
	Avatar     string
	Status     string
}

type ActiveFamilyRepository interface {
	GetActiveFamilySummary(ctx context.Context, userID string) (*Summary, error)
}

type Repository interface {
	ActiveFamilyRepository
	Create(ctx context.Context, userID string, name string) (Detail, error)
	GetCurrent(ctx context.Context, userID string) (*Detail, error)
	CreateJoinApplication(ctx context.Context, userID string, code string) (JoinApplication, error)
	GetMyJoinApplication(ctx context.Context, userID string) (*JoinApplication, error)
	ListMembers(ctx context.Context, familyID string) ([]Member, error)
	ListPendingApplications(ctx context.Context, familyID string) ([]JoinApplication, error)
	ApproveApplication(ctx context.Context, familyID string, applicationID string) error
	RejectApplication(ctx context.Context, familyID string, applicationID string) error
	RemoveMember(ctx context.Context, familyID string, memberID string, currentUserID string) error
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
	query := r.query("SELECT f.id, f.name, fm.role FROM family_members fm JOIN families f ON f.id = fm.family_id WHERE fm.user_id = ? AND fm.status = 'active' ORDER BY f.created_at LIMIT 1")
	var result Summary
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&result.ID, &result.Name, &result.Role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (r *SQLRepository) Create(ctx context.Context, userID string, name string) (Detail, error) {
	id, err := newID()
	if err != nil {
		return Detail{}, err
	}
	memberID, err := newID()
	if err != nil {
		return Detail{}, err
	}
	code, err := newCode()
	if err != nil {
		return Detail{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO families (id, name, code, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"), id, name, code); err != nil {
		return Detail{}, err
	}
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO family_members (id, family_id, user_id, role, status, created_at, updated_at) VALUES (?, ?, ?, 'owner', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"), memberID, id, userID); err != nil {
		return Detail{}, err
	}
	if err := tx.Commit(); err != nil {
		return Detail{}, err
	}
	return Detail{ID: id, Name: name, Code: code, Role: "owner"}, nil
}

func (r *SQLRepository) GetCurrent(ctx context.Context, userID string) (*Detail, error) {
	query := r.query("SELECT f.id, f.name, COALESCE(f.code, ''), fm.role FROM family_members fm JOIN families f ON f.id = fm.family_id WHERE fm.user_id = ? AND fm.status = 'active' ORDER BY f.created_at LIMIT 1")
	var result Detail
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&result.ID, &result.Name, &result.Code, &result.Role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (r *SQLRepository) CreateJoinApplication(ctx context.Context, userID string, code string) (JoinApplication, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return JoinApplication{}, err
	}
	defer tx.Rollback()
	var familyID, familyName string
	if err := tx.QueryRowContext(ctx, r.query("SELECT id, name FROM families WHERE code = ?"), code).Scan(&familyID, &familyName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JoinApplication{}, ErrNotFound
		}
		return JoinApplication{}, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, r.query("SELECT EXISTS(SELECT 1 FROM family_members WHERE user_id = ? AND status = 'active')"), userID).Scan(&exists); err != nil {
		return JoinApplication{}, err
	}
	if exists {
		return JoinApplication{}, ErrAlreadyInFamily
	}
	id, err := newID()
	if err != nil {
		return JoinApplication{}, err
	}
	query := "INSERT INTO family_join_applications (id, family_id, user_id, status, created_at, updated_at) VALUES (?, ?, ?, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) ON CONFLICT (family_id, user_id) DO UPDATE SET id = excluded.id, status = 'pending', updated_at = CURRENT_TIMESTAMP"
	if _, err := tx.ExecContext(ctx, r.query(query), id, familyID, userID); err != nil {
		return JoinApplication{}, err
	}
	if err := tx.Commit(); err != nil {
		return JoinApplication{}, err
	}
	return JoinApplication{ID: id, FamilyID: familyID, FamilyName: familyName, UserID: userID, Status: "pending"}, nil
}

func (r *SQLRepository) GetMyJoinApplication(ctx context.Context, userID string) (*JoinApplication, error) {
	query := r.query("SELECT a.id, a.family_id, f.name, a.user_id, a.status FROM family_join_applications a JOIN families f ON f.id = a.family_id WHERE a.user_id = ? ORDER BY a.updated_at DESC LIMIT 1")
	var result JoinApplication
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&result.ID, &result.FamilyID, &result.FamilyName, &result.UserID, &result.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (r *SQLRepository) ListMembers(ctx context.Context, familyID string) ([]Member, error) {
	query := r.query("SELECT fm.id, fm.user_id, COALESCE(u.nickname, ''), COALESCE(u.avatar_asset_id, ''), fm.role, fm.status FROM family_members fm JOIN users u ON u.id = fm.user_id WHERE fm.family_id = ? AND fm.status = 'active' ORDER BY fm.created_at")
	rows, err := r.db.QueryContext(ctx, query, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Member, 0)
	for rows.Next() {
		var value Member
		if err := rows.Scan(&value.ID, &value.UserID, &value.Nickname, &value.Avatar, &value.Role, &value.Status); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) ListPendingApplications(ctx context.Context, familyID string) ([]JoinApplication, error) {
	query := r.query("SELECT a.id, a.family_id, f.name, a.user_id, COALESCE(u.nickname, ''), COALESCE(u.avatar_asset_id, ''), a.status FROM family_join_applications a JOIN families f ON f.id = a.family_id JOIN users u ON u.id = a.user_id WHERE a.family_id = ? AND a.status = 'pending' ORDER BY a.created_at")
	rows, err := r.db.QueryContext(ctx, query, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]JoinApplication, 0)
	for rows.Next() {
		var value JoinApplication
		if err := rows.Scan(&value.ID, &value.FamilyID, &value.FamilyName, &value.UserID, &value.Nickname, &value.Avatar, &value.Status); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) ApproveApplication(ctx context.Context, familyID string, applicationID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := "SELECT user_id, status FROM family_join_applications WHERE id = ? AND family_id = ?"
	if r.isPostgres() {
		query += " FOR UPDATE"
	}
	var userID, status string
	if err := tx.QueryRowContext(ctx, r.query(query), applicationID, familyID).Scan(&userID, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != "pending" {
		return ErrApplicationProcessed
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, r.query("SELECT EXISTS(SELECT 1 FROM family_members WHERE user_id = ? AND status = 'active')"), userID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrAlreadyInFamily
	}
	memberID, err := newID()
	if err != nil {
		return err
	}
	query = "INSERT INTO family_members (id, family_id, user_id, role, status, created_at, updated_at) VALUES (?, ?, ?, 'member', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) ON CONFLICT (family_id, user_id) DO UPDATE SET role = excluded.role, status = excluded.status, updated_at = CURRENT_TIMESTAMP"
	if _, err := tx.ExecContext(ctx, r.query(query), memberID, familyID, userID); err != nil {
		return fmt.Errorf("create active family member: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE family_join_applications SET status = 'active', updated_at = CURRENT_TIMESTAMP WHERE id = ?"), applicationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRepository) RejectApplication(ctx context.Context, familyID string, applicationID string) error {
	result, err := r.db.ExecContext(ctx, r.query("UPDATE family_join_applications SET status = 'rejected', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND family_id = ? AND status = 'pending'"), applicationID, familyID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrApplicationProcessed
	}
	return nil
}

func (r *SQLRepository) RemoveMember(ctx context.Context, familyID string, memberID string, currentUserID string) error {
	var userID string
	if err := r.db.QueryRowContext(ctx, r.query("SELECT user_id FROM family_members WHERE id = ? AND family_id = ? AND status = 'active'"), memberID, familyID).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if userID == currentUserID {
		return ErrCannotRemoveSelf
	}
	_, err := r.db.ExecContext(ctx, r.query("UPDATE family_members SET status = 'inactive', updated_at = CURRENT_TIMESTAMP WHERE id = ?"), memberID)
	return err
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
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func newCode() (string, error) {
	var bytes [6]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(bytes[:])), nil
}
