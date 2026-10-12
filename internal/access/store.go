// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package access

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"devops-wrapper/internal/randid"
)

// User is a panel account kept in the database. The accounts in
// AUTH_USERS aren't: they're built-in admins (see Resolver).
type User struct {
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Disabled  bool      `json:"disabled"`
	Builtin   bool      `json:"builtin,omitempty"`
	Groups    []string  `json:"groups"` // group IDs
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	passwordHash string
}

// Group is a set of users that access is granted to together.
type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Members     []string  `json:"members"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Subject kinds a grant is given to.
const (
	SubjectUser  = "user"
	SubjectGroup = "group"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

var usernameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}$`)

// ValidUsername: 2-63 of lowercase letters, digits, dot, dash, underscore.
func ValidUsername(u string) bool { return usernameRE.MatchString(u) }

// MinPasswordLength is the shortest password the panel accepts.
const MinPasswordLength = 10

// CheckPassword says what's wrong with a new password, if anything.
func CheckPassword(pw, username string) error {
	if len(pw) < MinPasswordLength {
		return fmt.Errorf("the password must be at least %d characters", MinPasswordLength)
	}
	if len(pw) > 72 {
		return fmt.Errorf("the password must be at most 72 bytes") // bcrypt's limit
	}
	if strings.EqualFold(pw, username) {
		return fmt.Errorf("the password can't be the username")
	}
	return nil
}

// Store keeps users, groups and grants in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const userColumns = `username, password_hash, role, disabled, created_at, updated_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.Username, &u.passwordHash, &u.Role, &u.Disabled, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Groups = []string{}
	return &u, nil
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// Users is every database user, with the groups each is in.
func (s *Store) Users(ctx context.Context) ([]*User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userColumns+` FROM panel_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*User{}
	byName := map[string]*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
		byName[u.Username] = u
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := s.pool.Query(ctx, `SELECT username, group_id FROM panel_group_members ORDER BY group_id`)
	if err != nil {
		return nil, err
	}
	defer members.Close()
	for members.Next() {
		var name, group string
		if err := members.Scan(&name, &group); err != nil {
			return nil, err
		}
		if u := byName[name]; u != nil {
			u.Groups = append(u.Groups, group)
		}
	}
	return out, members.Err()
}

// User reads one database user, with its groups.
func (s *Store) User(ctx context.Context, username string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM panel_users WHERE username = $1`, username))
	if err != nil {
		return nil, err
	}
	u.Groups, err = s.groupsOf(ctx, username)
	return u, err
}

func (s *Store) groupsOf(ctx context.Context, username string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT group_id FROM panel_group_members WHERE username = $1 ORDER BY group_id`, username)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if ids == nil {
		ids = []string{}
	}
	return ids, err
}

// CreateUser adds a user; hash is its bcrypt password hash.
func (s *Store) CreateUser(ctx context.Context, username, hash, role string) (*User, error) {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `INSERT INTO panel_users (username, password_hash, role, disabled, created_at, updated_at)
		VALUES ($1, $2, $3, false, $4, $4)`, username, hash, role, now)
	if isUniqueViolation(err) {
		return nil, ErrExists
	}
	if err != nil {
		return nil, err
	}
	return s.User(ctx, username)
}

// UpdateUser changes a user's role and whether it's disabled.
func (s *Store) UpdateUser(ctx context.Context, username, role string, disabled bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE panel_users SET role = $2, disabled = $3, updated_at = $4 WHERE username = $1`,
		username, role, disabled, time.Now())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetPasswordHash replaces a user's password.
func (s *Store) SetPasswordHash(ctx context.Context, username, hash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE panel_users SET password_hash = $2, updated_at = $3 WHERE username = $1`,
		username, hash, time.Now())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteUser removes a user, its grants, group memberships and sessions.
func (s *Store) DeleteUser(ctx context.Context, username string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM panel_users WHERE username = $1`, username)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM panel_grants WHERE subject_kind = 'user' AND subject = $1`, username); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE username = $1`, username)
		return err
	})
}

// EndSessions signs a user out everywhere, except the session keep (if any).
func (s *Store) EndSessions(ctx context.Context, username, keep string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE username = $1 AND token <> $2`, username, keep)
	return err
}

// Groups is every group, with its members.
func (s *Store) Groups(ctx context.Context) ([]*Group, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, description, created_at FROM panel_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Group{}
	byID := map[string]*Group{}
	for rows.Next() {
		g := &Group{Members: []string{}}
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
		byID[g.ID] = g
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := s.pool.Query(ctx, `SELECT group_id, username FROM panel_group_members ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer members.Close()
	for members.Next() {
		var id, name string
		if err := members.Scan(&id, &name); err != nil {
			return nil, err
		}
		if g := byID[id]; g != nil {
			g.Members = append(g.Members, name)
		}
	}
	return out, members.Err()
}

// Group reads one group.
func (s *Store) Group(ctx context.Context, id string) (*Group, error) {
	all, err := s.Groups(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range all {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, ErrNotFound
}

// CreateGroup adds a group.
func (s *Store) CreateGroup(ctx context.Context, name, description string) (*Group, error) {
	id, err := randid.New(8)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO panel_groups (id, name, description, created_at) VALUES ($1, $2, $3, $4)`,
		id, name, description, time.Now())
	if isUniqueViolation(err) {
		return nil, ErrExists
	}
	if err != nil {
		return nil, err
	}
	return s.Group(ctx, id)
}

// UpdateGroup renames a group, changes its description, and sets its
// members to exactly members.
func (s *Store) UpdateGroup(ctx context.Context, id, name, description string, members []string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE panel_groups SET name = $2, description = $3 WHERE id = $1`, id, name, description)
		if isUniqueViolation(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM panel_group_members WHERE group_id = $1`, id); err != nil {
			return err
		}
		for _, m := range members {
			tag, err := tx.Exec(ctx, `INSERT INTO panel_group_members (group_id, username)
				SELECT $1, username FROM panel_users WHERE username = $2 ON CONFLICT DO NOTHING`, id, m)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("%w: user %q", ErrNotFound, m)
			}
		}
		return nil
	})
}

// SetUserGroups puts a user in exactly the groups given.
func (s *Store) SetUserGroups(ctx context.Context, username string, groups []string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM panel_group_members WHERE username = $1`, username); err != nil {
			return err
		}
		for _, g := range groups {
			tag, err := tx.Exec(ctx, `INSERT INTO panel_group_members (group_id, username)
				SELECT id, $2 FROM panel_groups WHERE id = $1 ON CONFLICT DO NOTHING`, g, username)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("%w: group %q", ErrNotFound, g)
			}
		}
		return nil
	})
}

// DeleteGroup removes a group and its grants; its members stay.
func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM panel_groups WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `DELETE FROM panel_grants WHERE subject_kind = 'group' AND subject = $1`, id)
		return err
	})
}

// Grants is what a user or group was given, per tab (tabs not given are
// None).
func (s *Store) Grants(ctx context.Context, kind, subject string) (map[string]Level, error) {
	rows, err := s.pool.Query(ctx, `SELECT tab, level FROM panel_grants WHERE subject_kind = $1 AND subject = $2`, kind, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Level{}
	for rows.Next() {
		var tab, level string
		if err := rows.Scan(&tab, &level); err != nil {
			return nil, err
		}
		if l, ok := ParseLevel(level); ok && l > None {
			out[tab] = l
		}
	}
	return out, rows.Err()
}

// SetGrants replaces what a user or group is given.
func (s *Store) SetGrants(ctx context.Context, kind, subject string, grants map[string]Level) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM panel_grants WHERE subject_kind = $1 AND subject = $2`, kind, subject); err != nil {
			return err
		}
		for tab, l := range grants {
			if l == None {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO panel_grants (subject_kind, subject, tab, level) VALUES ($1, $2, $3, $4)`,
				kind, subject, tab, l.String()); err != nil {
				return err
			}
		}
		return nil
	})
}
