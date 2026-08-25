package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrNoFreePorts = errors.New("no free TCP ports in configured range")

type User struct {
	ID             int64
	Email          string
	Subdomain      string
	TokenHash      string
	Plan           string
	Active         bool
	TCPPort        int
	BandwidthUsed  int64
	BandwidthLimit int64
	StartsAt       time.Time
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (u *User) Expired(now time.Time) bool { return !now.Before(u.ExpiresAt) }
func (u *User) OverLimit() bool            { return u.BandwidthLimit > 0 && u.BandwidthUsed >= u.BandwidthLimit }

const timeFmt = time.RFC3339

func now() string { return time.Now().UTC().Format(timeFmt) }

const userCols = `id, email, subdomain, token_hash, plan, active, tcp_port,
bandwidth_used, bandwidth_limit, starts_at, expires_at, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var active int
	var starts, expires, created, updated string
	err := row.Scan(&u.ID, &u.Email, &u.Subdomain, &u.TokenHash, &u.Plan, &active,
		&u.TCPPort, &u.BandwidthUsed, &u.BandwidthLimit, &starts, &expires, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Active = active == 1
	u.StartsAt, _ = time.Parse(timeFmt, starts)
	u.ExpiresAt, _ = time.Parse(timeFmt, expires)
	u.CreatedAt, _ = time.Parse(timeFmt, created)
	u.UpdatedAt, _ = time.Parse(timeFmt, updated)
	return &u, nil
}

func (s *Store) CreateUser(email, subdomain, plan, tokenHash string, tcpPort int, limit int64, days int) (*User, error) {
	if err := ValidateSubdomain(subdomain); err != nil {
		return nil, err
	}
	if days <= 0 {
		return nil, errors.New("days must be positive")
	}
	n := now()
	exp := time.Now().UTC().AddDate(0, 0, days).Format(timeFmt)
	res, err := s.db.Exec(`INSERT INTO users
        (email, subdomain, token_hash, plan, active, tcp_port, bandwidth_used, bandwidth_limit,
         starts_at, expires_at, created_at, updated_at)
        VALUES (?, ?, ?, ?, 1, ?, 0, ?, ?, ?, ?, ?)`,
		email, subdomain, tokenHash, plan, tcpPort, limit, n, exp, n, n)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetUserByID(id)
}
func (s *Store) GetUserByID(id int64) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}
func (s *Store) GetUserByTokenHash(hash string) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE token_hash = ?`, hash))
}
func (s *Store) GetUserBySubdomain(sub string) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE subdomain = ?`, sub))
}
func (s *Store) SubdomainExists(sub string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM users WHERE subdomain = ?`, sub).Scan(&n)
	return n > 0, err
}
func (s *Store) ListUsers() ([]*User, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + ` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s *Store) DeleteUser(id int64) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}
func (s *Store) SetActive(id int64, active bool) error {
	a := 0
	if active {
		a = 1
	}
	_, err := s.db.Exec(`UPDATE users SET active = ?, updated_at = ? WHERE id = ?`, a, now(), id)
	return err
}
func (s *Store) Renew(id int64, days int, resetBandwidth bool) error {
	if days <= 0 {
		return errors.New("days must be positive")
	}
	exp := time.Now().UTC().AddDate(0, 0, days).Format(timeFmt)
	if resetBandwidth {
		_, err := s.db.Exec(`UPDATE users SET expires_at=?, active=1, bandwidth_used=0, updated_at=? WHERE id=?`, exp, now(), id)
		return err
	}
	_, err := s.db.Exec(`UPDATE users SET expires_at=?, active=1, updated_at=? WHERE id=?`, exp, now(), id)
	return err
}
func (s *Store) SetToken(id int64, tokenHash string) error {
	_, err := s.db.Exec(`UPDATE users SET token_hash=?, updated_at=? WHERE id=?`, tokenHash, now(), id)
	return err
}
func (s *Store) SetBandwidthLimit(id int64, limit int64) error {
	if limit < 0 {
		return errors.New("limit cannot be negative")
	}
	_, err := s.db.Exec(`UPDATE users SET bandwidth_limit=?, updated_at=? WHERE id=?`, limit, now(), id)
	return err
}
func (s *Store) ResetBandwidth(id int64) error {
	_, err := s.db.Exec(`UPDATE users SET bandwidth_used=0, updated_at=? WHERE id=?`, now(), id)
	return err
}
func (s *Store) AddBandwidth(id int64, n int64) error {
	_, err := s.db.Exec(`UPDATE users SET bandwidth_used=bandwidth_used+? WHERE id=?`, n, id)
	return err
}
func (s *Store) UsedTCPPorts() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT tcp_port FROM users WHERE tcp_port > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	used := map[int]bool{}
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		used[p] = true
	}
	return used, rows.Err()
}
func (s *Store) AllocateTCPPort(min, max int) (int, error) {
	used, err := s.UsedTCPPorts()
	if err != nil {
		return 0, err
	}
	for p := min; p <= max; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, ErrNoFreePorts
}
func (s *Store) Audit(actor, action, detail string) {
	_, _ = s.db.Exec(`INSERT INTO audit_log(ts,actor,action,detail) VALUES(?,?,?,?)`, now(), actor, action, detail)
}
func (s *Store) RecentAudit(limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT ts,actor,action,detail FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ts, actor, action, detail string
		if err := rows.Scan(&ts, &actor, &action, &detail); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%s  %-10s %-18s %s", ts, actor, action, detail))
	}
	return out, rows.Err()
}
