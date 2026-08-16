package bootstrapadmin

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidRequest = errors.New("email and reason are required")
	ErrUserNotFound   = errors.New("verified active email user not found")
	ErrAdminExists    = errors.New("an initial admin already exists")
)

type Result struct {
	UserID       string
	AlreadyAdmin bool
}

// PromoteInitialAdmin performs the one permitted bootstrap transition. Once an
// active admin exists, normal audited admin APIs must perform privileged work.
func PromoteInitialAdmin(ctx context.Context, pool *pgxpool.Pool, email, reason string, now time.Time) (Result, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	reason = strings.TrimSpace(reason)
	if pool == nil || email == "" || reason == "" {
		return Result{}, ErrInvalidRequest
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result Result
	var role string
	err = tx.QueryRow(ctx, `SELECT users.id::text, users.role FROM users
		WHERE lower(users.email) = $1 AND users.status = 'active'
		AND EXISTS (SELECT 1 FROM auth_identities identity WHERE identity.user_id = users.id
			AND identity.provider = 'email' AND identity.verified_at IS NOT NULL)
		FOR UPDATE`, email).Scan(&result.UserID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrUserNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if role == "admin" {
		result.AlreadyAdmin = true
		return result, tx.Commit(ctx)
	}
	var existingAdmin bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE role = 'admin' AND status = 'active')`).Scan(&existingAdmin); err != nil {
		return Result{}, err
	}
	if existingAdmin {
		return Result{}, ErrAdminExists
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET role = 'admin', updated_at = $2 WHERE id = $1`, result.UserID, now.UTC()); err != nil {
		return Result{}, err
	}
	auditID, err := uuidv7.New()
	if err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs
		(id, actor_user_id, actor_kind, action, target_table, target_id, reason, metadata, created_at)
		VALUES ($1, $2, 'system', 'initial_admin_bootstrap', 'users', $2, $3,
			jsonb_build_object('previous_role', $4::text, 'email_login_verified', true), $5)`,
		auditID.String(), result.UserID, reason, role, now.UTC()); err != nil {
		return Result{}, err
	}
	return result, tx.Commit(ctx)
}
