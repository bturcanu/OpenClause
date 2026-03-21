package console

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AuthSession struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	TenantID   string     `json:"tenant_id,omitempty"`
	Roles      []string   `json:"roles"`
	UserAgent  string     `json:"user_agent,omitempty"`
	ClientIP   string     `json:"client_ip,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RevokedBy  string     `json:"revoked_by,omitempty"`
}

type AuthSessionCreateInput struct {
	UserID    string
	Email     string
	Name      string
	TenantID  string
	Roles     []string
	UserAgent string
	ClientIP  string
	ExpiresAt time.Time
}

func (s *Store) CreateAuthSession(ctx context.Context, in AuthSessionCreateInput) (*AuthSession, error) {
	rolesJSON, err := json.Marshal(in.Roles)
	if err != nil {
		return nil, fmt.Errorf("console.CreateAuthSession marshal roles: %w", err)
	}

	session := &AuthSession{
		ID:        uuid.NewString(),
		UserID:    in.UserID,
		Email:     canonicalEmail(in.Email),
		Name:      in.Name,
		TenantID:  in.TenantID,
		Roles:     append([]string(nil), in.Roles...),
		UserAgent: in.UserAgent,
		ClientIP:  in.ClientIP,
		ExpiresAt: in.ExpiresAt.UTC(),
	}

	err = s.pool.QueryRow(ctx, `
		INSERT INTO auth_sessions (
			id, user_id, email, name, tenant_id, roles, user_agent, client_ip, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, last_seen_at`,
		session.ID, session.UserID, session.Email, session.Name, session.TenantID,
		rolesJSON, session.UserAgent, session.ClientIP, session.ExpiresAt,
	).Scan(&session.CreatedAt, &session.LastSeenAt)
	if err != nil {
		return nil, fmt.Errorf("console.CreateAuthSession: %w", err)
	}
	return session, nil
}

func (s *Store) TouchAuthSession(ctx context.Context, sessionID, userID string, seenAt time.Time) (bool, error) {
	if sessionID == "" || userID == "" {
		return false, nil
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE auth_sessions
		SET last_seen_at = CASE
			WHEN last_seen_at < $3 - INTERVAL '1 minute' THEN $3
			ELSE last_seen_at
		END
		WHERE id = $1
		  AND user_id = $2
		  AND revoked_at IS NULL
		  AND expires_at > $3`,
		sessionID, userID, seenAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("console.TouchAuthSession: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ListAuthSessions(ctx context.Context, tenantID, userID string, limit, offset int) ([]AuthSession, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	query := `
		SELECT id, user_id, email, name, tenant_id, roles, user_agent, client_ip,
		       created_at, last_seen_at, expires_at, revoked_at, revoked_by
		FROM auth_sessions
		WHERE revoked_at IS NULL
		  AND expires_at > NOW()`
	args := make([]any, 0, 4)
	argPos := 1
	if tenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		args = append(args, tenantID)
		argPos++
	}
	if userID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argPos)
		args = append(args, userID)
		argPos++
	}
	query += fmt.Sprintf(" ORDER BY last_seen_at DESC LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListAuthSessions: %w", err)
	}
	defer rows.Close()

	out := make([]AuthSession, 0)
	for rows.Next() {
		var session AuthSession
		var rolesJSON []byte
		if err := rows.Scan(
			&session.ID, &session.UserID, &session.Email, &session.Name,
			&session.TenantID, &rolesJSON, &session.UserAgent, &session.ClientIP,
			&session.CreatedAt, &session.LastSeenAt, &session.ExpiresAt,
			&session.RevokedAt, &session.RevokedBy,
		); err != nil {
			return nil, fmt.Errorf("console.ListAuthSessions scan: %w", err)
		}
		if len(rolesJSON) > 0 {
			if err := json.Unmarshal(rolesJSON, &session.Roles); err != nil {
				return nil, fmt.Errorf("console.ListAuthSessions unmarshal roles: %w", err)
			}
		}
		out = append(out, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListAuthSessions iteration: %w", err)
	}
	return out, nil
}

func (s *Store) ListActiveAuthSessionCounts(ctx context.Context, tenantID string) (map[string]int64, error) {
	query := `
		SELECT user_id, COUNT(*)
		FROM auth_sessions
		WHERE revoked_at IS NULL
		  AND expires_at > NOW()`
	args := make([]any, 0, 1)
	if tenantID != "" {
		query += ` AND tenant_id = $1`
		args = append(args, tenantID)
	}
	query += ` GROUP BY user_id`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("console.ListActiveAuthSessionCounts: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64)
	for rows.Next() {
		var userID string
		var count int64
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, fmt.Errorf("console.ListActiveAuthSessionCounts scan: %w", err)
		}
		out[userID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("console.ListActiveAuthSessionCounts iteration: %w", err)
	}
	return out, nil
}

func (s *Store) RevokeAuthSession(ctx context.Context, sessionID, tenantID, revokedBy string, now time.Time) (bool, error) {
	if sessionID == "" {
		return false, nil
	}

	query := `
		UPDATE auth_sessions
		SET revoked_at = $2, revoked_by = $3
		WHERE id = $1
		  AND revoked_at IS NULL
		  AND expires_at > $2`
	args := []any{sessionID, now.UTC(), revokedBy}
	if tenantID != "" {
		query += ` AND tenant_id = $4`
		args = append(args, tenantID)
	}

	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("console.RevokeAuthSession: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
